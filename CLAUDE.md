# jump

终端 SSH host 选择器。读 `~/.ssh/config`，bubbletea + bubbles/list 跑一个模糊搜索 TUI，回车后 `syscall.Exec` 接管成 `ssh <host>`。

## 文件

- `main.go` — 全部逻辑：ssh config 解析 + TUI + exec ssh
- `build.sh` — `go build -ldflags="-s -w" -o ~/bin/jump .`，并维护 `~/bin/j -> jump` 软链
- `go.mod` — module name `jump`

## 构建 / 安装

```bash
./build.sh
```

部署位置：
- `~/bin/jump` — 主二进制
- `~/bin/j` — 软链，等价短命令

`~/bin` 已经在 `~/.zshrc` 里加到 PATH。

## SSH config 备注约定（关键，自创）

工具在标准 ssh config 解析之外，识别两种「备注」并在列表里展示：

1. **Host 行上方紧贴的 `#` 行**（中间不能有空行，否则会被重置）
2. **Host 行行尾 `# 备注`**

两者可同时存在，会用 ` · ` 拼接。备注也参与 fuzzy 过滤。

### 预设远端工作目录 `cwd=`

在备注里写 `cwd=/path` 或 `cwd:/path` token，会从备注文本里被悄悄抽走，并在选中后改用：

```bash
ssh -t <host> 'cd '\''<cwd>'\'' && exec ${SHELL:-/bin/sh} -l'
```

例：

```sshconfig
# cwd=/data/app  阿里云北京 客户A
Host s1
    HostName 1.2.3.4
    User root
```

未配 `cwd=` 的 host 行为完全等同 `ssh <host>`，不会强制 `-t`。

## 入口流程（main 函数）

1. 读 `$SSH_CONFIG` 或 `~/.ssh/config`
2. `parseSSHConfig` 递归处理 `Include` 指令、跳过通配符 host、按 name 首次出现去重
3. bubbles list 渲染，过滤态 enter = accept filter；非过滤态 enter = 选中退出
4. 选中后 `syscall.Exec` 成 ssh（不是子进程，直接替换当前 PID，退出 ssh 即退出工具）
5. CLI 透传：`jump -p 2222` → `ssh -p 2222 <host>`（host 前的所有参数都给 ssh）

## 依赖

- `github.com/charmbracelet/bubbletea`
- `github.com/charmbracelet/bubbles/list`
- `github.com/charmbracelet/lipgloss`

只读 stdlib + 上面三个包，跨 macOS / Linux。
