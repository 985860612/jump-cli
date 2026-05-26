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

type hostEntry struct {
	name     string
	hostName string
	user     string
	comment  string
	cwd      string // 预设远端工作目录，来自注释里的 cwd=xxx 或 cwd:xxx
}

func (h hostEntry) Title() string {
	if h.comment != "" {
		return h.name + "  ·  " + h.comment
	}
	return h.name
}

func (h hostEntry) Description() string {
	var base string
	switch {
	case h.user != "" && h.hostName != "":
		base = h.user + "@" + h.hostName
	case h.hostName != "":
		base = h.hostName
	case h.user != "":
		base = h.user + "@?"
	default:
		base = "(no HostName)"
	}
	if h.cwd != "" {
		base += "  cwd=" + h.cwd
	}
	return base
}

func (h hostEntry) FilterValue() string {
	return h.name + " " + h.hostName + " " + h.user + " " + h.comment
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
	if final.selected.cwd != "" {
		// 远端命令：cd 到预设目录，再起一个登录 shell 接管
		args = append(args, "-t", final.selected.name,
			"cd "+shellQuote(final.selected.cwd)+" && exec ${SHELL:-/bin/sh} -l")
	} else {
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
		names    []string
		hostName string
		user     string
		comment  string
		cwd      string
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
				name:     n,
				hostName: current.hostName,
				user:     current.user,
				comment:  current.comment,
				cwd:      current.cwd,
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
			cleanedPending, cwdFromPending := extractCwdFromMany(pendingComments)
			cleanedTrail, cwdFromTrail := extractCwdFromOne(trailComment)
			cwd := cwdFromPending
			if cwd == "" {
				cwd = cwdFromTrail
			}
			current = &bucket{
				names:   strings.Fields(val),
				comment: joinComments(cleanedPending, cleanedTrail),
				cwd:     cwd,
			}
		case "hostname":
			if current != nil {
				current.hostName = val
			}
		case "user":
			if current != nil {
				current.user = val
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

// extractCwdFromOne 在一段注释里找形如 `cwd=/path` 或 `cwd:/path` 的 token，
// 返回剥掉该 token 后的文本和 cwd 值。找不到 cwd 返回 "".
func extractCwdFromOne(text string) (rest, cwd string) {
	if text == "" {
		return "", ""
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
		keep = append(keep, t)
	}
	return strings.Join(keep, " "), cwd
}

func extractCwdFromMany(comments []string) (cleaned []string, cwd string) {
	for _, c := range comments {
		rest, v := extractCwdFromOne(c)
		if cwd == "" && v != "" {
			cwd = v
		}
		if rest != "" {
			cleaned = append(cleaned, rest)
		}
	}
	return cleaned, cwd
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
