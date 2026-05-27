package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

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

	hosts, err := parseSSHConfig(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse ssh config:", err)
		os.Exit(1)
	}
	if len(hosts) == 0 {
		fmt.Fprintln(os.Stderr, "no Host entries found in", configPath)
		os.Exit(1)
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

	sshBin, err := exec.LookPath("ssh")
	if err != nil {
		fmt.Fprintln(os.Stderr, "ssh not found in PATH")
		os.Exit(1)
	}
	args := []string{"ssh"}
	args = append(args, os.Args[1:]...) // 透传给 ssh 的额外选项
	switch {
	case final.selected.cmd != "":
		// 远端跑指定命令；如同时配了 cwd 就先 cd。完事 ssh 退出，本进程一起退。
		remote := final.selected.cmd
		if final.selected.cwd != "" {
			remote = "cd " + shellQuote(final.selected.cwd) + " && " + final.selected.cmd
		}
		args = append(args, "-t", final.selected.name, remote)
	case final.selected.cwd != "":
		// 远端命令：cd 到预设目录，再起一个登录 shell 接管
		args = append(args, "-t", final.selected.name,
			"cd "+shellQuote(final.selected.cwd)+" && exec ${SHELL:-/bin/sh} -l")
	default:
		args = append(args, final.selected.name)
	}
	if err := syscall.Exec(sshBin, args, os.Environ()); err != nil {
		fmt.Fprintln(os.Stderr, "exec ssh:", err)
		os.Exit(1)
	}
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
