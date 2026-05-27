package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// version is set via -ldflags at build time.
var version = "dev"

type hostEntry struct {
	name      string
	hostName  string
	user      string
	port      string // ssh config 的 Port，非 22 时显示
	proxyJump string // ssh config 的 ProxyJump，跳板链
	comment   string
	cwd       string // 预设远端工作目录，来自注释里的 cwd=xxx 或 cwd:xxx
	tag       string // 分组标签，来自注释里的 tag=xxx 或 tag:xxx
	cmd       string // 远端命令，来自注释里 cmd=（贪婪，取到行尾）
}

func (h hostEntry) Title() string {
	head := h.name
	if h.tag != "" {
		head = "[" + h.tag + "] " + head
	}
	if h.comment != "" {
		return head + "  ·  " + h.comment
	}
	return head
}

func (h hostEntry) Description() string {
	hp := h.hostName
	if h.port != "" && h.port != "22" {
		if hp == "" {
			hp = "?"
		}
		hp += ":" + h.port
	}
	var base string
	switch {
	case h.user != "" && hp != "":
		base = h.user + "@" + hp
	case hp != "":
		base = hp
	case h.user != "":
		base = h.user + "@?"
	default:
		base = "(no HostName)"
	}
	if h.proxyJump != "" {
		base += "  via " + h.proxyJump
	}
	if h.cwd != "" {
		base += "  cwd=" + h.cwd
	}
	if h.cmd != "" {
		base += "  cmd=" + h.cmd
	}
	return base
}

func (h hostEntry) FilterValue() string {
	return h.name + " " + h.hostName + " " + h.user + " " + h.comment + " " + h.tag
}

type model struct {
	list     list.Model
	selected *hostEntry
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height-2)
	case tea.KeyMsg:
		if m.list.FilterState() != list.Filtering {
			switch msg.String() {
			case "q", "ctrl+c", "esc":
				return m, tea.Quit
			case "enter":
				if it, ok := m.list.SelectedItem().(hostEntry); ok {
					m.selected = &it
				}
				return m, tea.Quit
			case "d":
				// 把当前 host 从 MRU 历史里抹掉（不删 host，只复位它的最近使用时间）
				if it, ok := m.list.SelectedItem().(hostEntry); ok {
					deleteHistoryForName(it.name)
					return m, m.list.NewStatusMessage("removed from MRU: " + it.name)
				}
			}
		} else if msg.String() == "enter" {
			// 过滤中按 enter：先 accept filter 再让用户再按 enter 选中
			// bubbles list 本身就是这个行为，无需特殊处理
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m model) View() string { return m.list.View() }

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "-v" || arg == "--version" || arg == "version" {
			fmt.Println("jump", version)
			return
		}
	}

	configPath := filepath.Join(os.Getenv("HOME"), ".ssh", "config")
	if p := os.Getenv("SSH_CONFIG"); p != "" {
		configPath = p
	}

	// 模式开关：--list / --completion 走完即退，不进 TUI
	for _, a := range os.Args[1:] {
		switch a {
		case "--list":
			runList(configPath)
			return
		case "--completion":
			// 默认输出 zsh 脚本（目前只支持 zsh）
			runCompletion()
			return
		}
	}

	hosts, err := parseSSHConfig(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse ssh config:", err)
		os.Exit(1)
	}
	if len(hosts) == 0 {
		fmt.Fprintln(os.Stderr, "no Host entries found in", configPath)
		os.Exit(1)
	}

	// CLI 透传 & 预填过滤：第一个非 flag 的位置参数作为初始过滤词，剩余原样给 ssh
	var initialFilter string
	sshExtraArgs := os.Args[1:]
	if len(sshExtraArgs) > 0 && !strings.HasPrefix(sshExtraArgs[0], "-") {
		initialFilter = sshExtraArgs[0]
		sshExtraArgs = sshExtraArgs[1:]
	}

	// MRU：按上次选中时间倒序排（没用过的按 ssh config 原序在后）
	hist := loadHistory()
	sort.SliceStable(hosts, func(i, j int) bool {
		return hist[hosts[i].name] > hist[hosts[j].name]
	})

	// 命中预填：先按名字精确匹配（不区分大小写），命中唯一就跳过 TUI 直接连
	if initialFilter != "" {
		if h := findExactName(hosts, initialFilter); h != nil {
			execSSH(*h, sshExtraArgs)
			return
		}
		needle := strings.ToLower(initialFilter)
		narrowed := hosts[:0]
		for _, h := range hosts {
			if strings.Contains(strings.ToLower(h.FilterValue()), needle) {
				narrowed = append(narrowed, h)
			}
		}
		if len(narrowed) == 0 {
			fmt.Fprintln(os.Stderr, "no host matches", initialFilter)
			os.Exit(1)
		}
		hosts = narrowed
	}

	items := make([]list.Item, 0, len(hosts))
	for _, h := range hosts {
		items = append(items, h)
	}

	l := list.New(items, list.NewDefaultDelegate(), 80, 20)
	l.Title = fmt.Sprintf("ssh hosts  (%d entries from %s)", len(hosts), prettyPath(configPath))
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.Styles.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 1)

	p := tea.NewProgram(model{list: l}, tea.WithAltScreen())
	finalM, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "tui:", err)
		os.Exit(1)
	}

	final := finalM.(model)
	if final.selected == nil {
		os.Exit(130) // 取消
	}

	execSSH(*final.selected, sshExtraArgs)
}

// execSSH 记录历史 + 拼远端命令 + syscall.Exec 替换成 ssh。正常情况下不返回。
func execSSH(h hostEntry, extraArgs []string) {
	recordHistory(h.name)

	sshBin, err := exec.LookPath("ssh")
	if err != nil {
		fmt.Fprintln(os.Stderr, "ssh not found in PATH")
		os.Exit(1)
	}
	args := []string{"ssh"}
	args = append(args, extraArgs...)

	switch {
	case h.cmd != "":
		// 远端跑指定命令；如同时配了 cwd 就先 cd。完事 ssh 退出，本进程一起退。
		remote := h.cmd
		if h.cwd != "" {
			remote = "cd " + shellQuote(h.cwd) + " && " + h.cmd
		}
		args = append(args, "-t", h.name, remote)
	case h.cwd != "":
		args = append(args, "-t", h.name,
			"cd "+shellQuote(h.cwd)+" && exec ${SHELL:-/bin/sh} -l")
	default:
		args = append(args, h.name)
	}
	if err := syscall.Exec(sshBin, args, os.Environ()); err != nil {
		fmt.Fprintln(os.Stderr, "exec ssh:", err)
		os.Exit(1)
	}
}

// runList 打印 "name\tdesc" 一行一个 host，供 shell 补全脚本读取
func runList(configPath string) {
	hosts, err := parseSSHConfig(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse ssh config:", err)
		os.Exit(1)
	}
	// 用 MRU 排序，补全候选顺序也跟 TUI 一致
	hist := loadHistory()
	sort.SliceStable(hosts, func(i, j int) bool {
		return hist[hosts[i].name] > hist[hosts[j].name]
	})
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	for _, h := range hosts {
		desc := h.comment
		if h.tag != "" {
			if desc == "" {
				desc = "[" + h.tag + "]"
			} else {
				desc = "[" + h.tag + "] " + desc
			}
		}
		// 描述里不能带 \n / \t（会破坏 _describe 解析）
		desc = strings.ReplaceAll(desc, "\t", " ")
		desc = strings.ReplaceAll(desc, "\n", " ")
		fmt.Fprintf(w, "%s\t%s\n", h.name, desc)
	}
}

// runCompletion 输出 zsh 补全脚本到 stdout。用户：source <(jump --completion)
func runCompletion() {
	fmt.Print(`#compdef jump j

_jump() {
    local -a hosts
    local name desc
    while IFS=$'\t' read -r name desc; do
        if [[ -n "$desc" ]]; then
            hosts+=("${name}:${desc}")
        else
            hosts+=("${name}")
        fi
    done < <(jump --list 2>/dev/null)
    _describe -t hosts 'ssh host' hosts
}

compdef _jump jump j
`)
}

func findExactName(hosts []hostEntry, name string) *hostEntry {
	target := strings.ToLower(name)
	for i := range hosts {
		if strings.ToLower(hosts[i].name) == target {
			return &hosts[i]
		}
	}
	return nil
}

// ---------------- MRU history ----------------

func historyPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "jump", "history")
}

// loadHistory 读历史文件，每行 "<unix_ts>\t<name>"，返回 name → 最近一次时间戳
func loadHistory() map[string]int64 {
	f, err := os.Open(historyPath())
	if err != nil {
		return map[string]int64{}
	}
	defer f.Close()
	result := map[string]int64{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		i := strings.IndexByte(line, '\t')
		if i <= 0 || i+1 >= len(line) {
			continue
		}
		ts, err := strconv.ParseInt(line[:i], 10, 64)
		if err != nil {
			continue
		}
		name := line[i+1:]
		if ts > result[name] {
			result[name] = ts
		}
	}
	return result
}

// recordHistory 追加一行到历史文件；失败静默（不能阻断 ssh）。
// 顺手在文件超过阈值时压缩一次。
func recordHistory(name string) {
	p := historyPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	fmt.Fprintf(f, "%d\t%s\n", time.Now().Unix(), name)
	if info, err := f.Stat(); err == nil && info.Size() > 100<<10 {
		f.Close()
		pruneHistory()
		return
	}
	f.Close()
}

// deleteHistoryForName 从历史文件里删掉所有 host name 匹配的行
func deleteHistoryForName(name string) {
	p := historyPath()
	data, err := os.ReadFile(p)
	if err != nil {
		return
	}
	suffix := "\t" + name
	var b strings.Builder
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasSuffix(line, suffix) {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	writeAtomic(p, b.String())
}

// pruneHistory 压缩历史文件：保留 (每个 host 最新一行) ∪ (全局最近 200 行)
func pruneHistory() {
	p := historyPath()
	data, err := os.ReadFile(p)
	if err != nil {
		return
	}
	type entry struct {
		ts   int64
		line string
		name string
	}
	var entries []entry
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		i := strings.IndexByte(line, '\t')
		if i <= 0 || i+1 >= len(line) {
			continue
		}
		ts, err := strconv.ParseInt(line[:i], 10, 64)
		if err != nil {
			continue
		}
		entries = append(entries, entry{ts, line, line[i+1:]})
	}
	// 按 ts 降序排，方便挑 top 200 和每个 name 的最新
	sort.Slice(entries, func(i, j int) bool { return entries[i].ts > entries[j].ts })
	keep := make([]bool, len(entries))
	seenName := map[string]bool{}
	for i, e := range entries {
		if i < 200 {
			keep[i] = true
		}
		if !seenName[e.name] {
			keep[i] = true
			seenName[e.name] = true
		}
	}
	// 写回时再按 ts 升序排，文件保持时间顺序
	type idxEntry struct {
		i int
		e entry
	}
	var kept []idxEntry
	for i, e := range entries {
		if keep[i] {
			kept = append(kept, idxEntry{i, e})
		}
	}
	sort.Slice(kept, func(i, j int) bool { return kept[i].e.ts < kept[j].e.ts })
	var b strings.Builder
	for _, k := range kept {
		b.WriteString(k.e.line)
		b.WriteByte('\n')
	}
	writeAtomic(p, b.String())
}

// writeAtomic 写临时文件再 rename，避免崩在中途留半截文件
func writeAtomic(path, content string) {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// ---------------- ssh config parser ----------------

func parseSSHConfig(path string) ([]hostEntry, error) {
	seen := map[string]bool{}
	raw, err := parseFile(path, seen)
	if err != nil {
		return nil, err
	}
	// 按 name 去重，保留首次出现（符合 ssh "first match wins" 直觉）
	seenName := map[string]bool{}
	out := raw[:0]
	for _, h := range raw {
		if seenName[h.name] {
			continue
		}
		seenName[h.name] = true
		out = append(out, h)
	}
	return out, nil
}

func parseFile(path string, seen map[string]bool) ([]hostEntry, error) {
	abs, err := filepath.Abs(expandHome(path))
	if err != nil {
		return nil, err
	}
	if seen[abs] {
		return nil, nil
	}
	seen[abs] = true

	f, err := os.Open(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	type bucket struct {
		names     []string
		hostName  string
		user      string
		port      string
		proxyJump string
		comment   string
		cwd       string
		tag       string
		cmd       string
	}
	var out []hostEntry
	var current *bucket
	flush := func() {
		if current == nil {
			return
		}
		for _, n := range current.names {
			if strings.ContainsAny(n, "*?!") {
				continue
			}
			out = append(out, hostEntry{
				name:      n,
				hostName:  current.hostName,
				user:      current.user,
				port:      current.port,
				proxyJump: current.proxyJump,
				comment:   current.comment,
				cwd:       current.cwd,
				tag:       current.tag,
				cmd:       current.cmd,
			})
		}
		current = nil
	}

	var pendingComments []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			pendingComments = nil
			continue
		}
		if strings.HasPrefix(line, "#") {
			if text := strings.TrimSpace(strings.TrimLeft(line, "#")); text != "" {
				pendingComments = append(pendingComments, text)
			}
			continue
		}
		var trailComment string
		if idx := findCommentStart(line); idx > 0 {
			trailComment = strings.TrimSpace(strings.TrimLeft(line[idx:], "#"))
			line = strings.TrimSpace(line[:idx])
		}
		key, val := splitKV(line)
		if key == "" {
			pendingComments = nil
			continue
		}
		switch strings.ToLower(key) {
		case "host":
			flush()
			cleanedPending, cwdP, tagP, cmdP := extractTokensFromMany(pendingComments)
			cleanedTrail, cwdT, tagT, cmdT := extractTokens(trailComment)
			current = &bucket{
				names:   strings.Fields(val),
				comment: joinComments(cleanedPending, cleanedTrail),
				cwd:     firstNonEmpty(cwdP, cwdT),
				tag:     firstNonEmpty(tagP, tagT),
				cmd:     firstNonEmpty(cmdP, cmdT),
			}
		case "match":
			// Match 块跟 Host 不是同一种东西：里面的 HostName/User 不应该回流到上一个 Host bucket。
			// 直接 flush 上一个 bucket，让后续字段无处可去（current == nil 时 hostname/user 分支都是 no-op）。
			flush()
		case "hostname":
			if current != nil {
				current.hostName = val
			}
		case "user":
			if current != nil {
				current.user = val
			}
		case "port":
			if current != nil {
				current.port = val
			}
		case "proxyjump":
			if current != nil {
				current.proxyJump = val
			}
		case "include":
			flush()
			for _, p := range strings.Fields(val) {
				p = expandHome(p)
				if !filepath.IsAbs(p) {
					p = filepath.Join(filepath.Dir(abs), p)
				}
				matches, _ := filepath.Glob(p)
				for _, m := range matches {
					sub, err := parseFile(m, seen)
					if err == nil {
						out = append(out, sub...)
					}
				}
			}
		}
		pendingComments = nil
	}
	flush()
	return out, sc.Err()
}

func splitKV(line string) (string, string) {
	// 支持 "Key Value" 和 "Key=Value"
	i := strings.IndexAny(line, "= \t")
	if i <= 0 {
		return "", ""
	}
	k := line[:i]
	v := strings.TrimLeft(line[i:], "= \t")
	return k, v
}

// findCommentStart 找到行尾注释的 `#` 位置（要求前面是空白），找不到返回 -1
func findCommentStart(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '#' && (i == 0 || s[i-1] == ' ' || s[i-1] == '\t') {
			return i
		}
	}
	return -1
}

// extractTokens 从一段注释里抽出 cwd=/cwd: tag=/tag: cmd= 三种 token。
// cmd= 是贪婪的：取到行尾全部内容作为远端命令；其他 token 不再被识别。
// 返回剥掉 token 后的剩余文本以及抽到的值。
func extractTokens(text string) (rest, cwd, tag, cmd string) {
	if text == "" {
		return "", "", "", ""
	}
	// 先处理 cmd=：必须在 word 边界，整段取到行尾
	if idx := findTokenStart(text, "cmd="); idx >= 0 {
		cmd = strings.TrimSpace(text[idx+len("cmd="):])
		text = strings.TrimSpace(text[:idx])
	}
	tokens := strings.Fields(text)
	keep := tokens[:0]
	for _, t := range tokens {
		if cwd == "" {
			if v, ok := strings.CutPrefix(t, "cwd="); ok {
				cwd = v
				continue
			}
			if v, ok := strings.CutPrefix(t, "cwd:"); ok {
				cwd = v
				continue
			}
		}
		if tag == "" {
			if v, ok := strings.CutPrefix(t, "tag="); ok {
				tag = v
				continue
			}
			if v, ok := strings.CutPrefix(t, "tag:"); ok {
				tag = v
				continue
			}
		}
		keep = append(keep, t)
	}
	return strings.Join(keep, " "), cwd, tag, cmd
}

func extractTokensFromMany(comments []string) (cleaned []string, cwd, tag, cmd string) {
	for _, c := range comments {
		rest, vCwd, vTag, vCmd := extractTokens(c)
		if cwd == "" {
			cwd = vCwd
		}
		if tag == "" {
			tag = vTag
		}
		if cmd == "" {
			cmd = vCmd
		}
		if rest != "" {
			cleaned = append(cleaned, rest)
		}
	}
	return cleaned, cwd, tag, cmd
}

// findTokenStart 找到 prefix 在 s 中第一个处于 word 边界的位置（前面是 BOS 或空白），找不到返回 -1
func findTokenStart(s, prefix string) int {
	for from := 0; from < len(s); {
		i := strings.Index(s[from:], prefix)
		if i < 0 {
			return -1
		}
		abs := from + i
		if abs == 0 || s[abs-1] == ' ' || s[abs-1] == '\t' {
			return abs
		}
		from = abs + 1
	}
	return -1
}

func joinComments(pending []string, trailing string) string {
	parts := make([]string, 0, len(pending)+1)
	for _, c := range pending {
		if c != "" {
			parts = append(parts, c)
		}
	}
	if trailing != "" {
		parts = append(parts, trailing)
	}
	return strings.Join(parts, " · ")
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

func prettyPath(p string) string {
	if home := os.Getenv("HOME"); home != "" && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
