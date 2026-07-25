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

### 注释里的 token（会从展示文本里悄悄抽走）

| token | 形式 | 行为 |
| --- | --- | --- |
| `cwd=` / `cwd:` | 单 token | 选中后 `ssh -t <host> 'cd <cwd> && exec ${SHELL:-/bin/sh} -l'` |
| `tag=` / `tag:` | 单 token | 列表 title 前缀显示 `[tag]`，并参与 fuzzy 过滤 |
| `svc=` / `svc:` | 单 token（多服务逗号分隔，别留空格） | 列表 description 里显示 `⚙ <svc>`，标注这台机器部署了什么服务；参与 fuzzy 过滤，`--list` 补全描述里也带上 |
| `cmd=` | **贪婪到行尾**，必须放最后 | 选中后 `ssh -t <host> '<cmd>'`，跑完即退；若同时配 `cwd=` 会先 cd |

例：

```sshconfig
# cwd=/data/app tag=prod svc=nginx,redis,pg  阿里云北京 客户A
Host s1
    HostName 1.2.3.4
    User root

# tag=ops cmd=df -h | head -5
Host metrics
    HostName 5.6.7.8
```

未配任何 token 的 host 行为完全等同 `ssh <host>`，不会强制 `-t`。

## 入口流程（main 函数）

1. 模式开关：`--list` / `--completion` / `--themes` 走特殊路径，直接 print 完退出
2. 读 `$SSH_CONFIG` 或 `~/.ssh/config`
3. `parseSSHConfig` 递归处理 `Include` 指令、跳过通配符 host、按 name 首次出现去重；`Match` 块整体忽略且会 flush 上一个 Host bucket，防止字段污染；除 HostName/User 外还解析 Port / ProxyJump
4. 按 `~/.local/state/jump/history` 里的最近选中时间排序（MRU），没用过的按 ssh config 原序落在后面
5. CLI 第一个非 flag 的位置参数当成 **预填过滤词**：精确匹配 host 名时直接跳过 TUI 连上去；否则 TUI 列表只显示命中的子集。剩余参数原样透传给 ssh
6. bubbles list 渲染，过滤态 enter = accept filter；非过滤态 enter = 选中退出
7. 选中后追加历史，`syscall.Exec` 成 ssh（不是子进程，直接替换当前 PID，退出 ssh 即退出工具）

## TUI 键位（非过滤态）

| 键 | 行为 |
| --- | --- |
| enter | 选中 → ssh |
| q / esc / ctrl+c | 取消退出 |
| d | 把当前 host 从 MRU 历史里抹掉（不删 host，下次启动它会回落到原序） |
| `/` | 进过滤态 |

## MRU 历史

- 文件：`~/.local/state/jump/history`
- 格式：每行 `<unix_ts>\t<host_name>`，追加写
- 读取时按 host name 取最大时间戳；失败/不存在静默忽略，不阻断 ssh
- **自动 prune**：写入后若文件 > 100KB，触发压缩，保留 `(每个 host 最新一行) ∪ (全局最近 200 行)`，临时文件 + rename 原子写

## CLI 用例

```bash
jump                    # 全列表 TUI
jump prod               # 子串过滤 'prod' 的子列表 TUI；若 'prod' 恰好是 host 名则直连
jump s1                 # host 名命中 → 直接 ssh s1
jump s1 -p 2222         # 直连 + ssh 透传 -p 2222
jump -p 2222            # 全列表 TUI，选中后透传 -p 2222
jump --theme ocean      # 指定配色开 TUI（旗标会被剥掉，不透传给 ssh）
jump --list             # 打印 'name\tdesc' 一行一个，给补全脚本用
jump --themes           # 打印全部配色方案（终端里带色块预览）
jump --completion       # 打印 zsh 补全脚本
```

## 配色方案

- 选择方式：`--theme <name>` 旗标 或 `JUMP_THEME` 环境变量（旗标优先），都不给用 `default`
- 方案定义在 main.go 的 `themeList`：标题条前景/背景 + accent（选中项 / 过滤提示符 / fuzzy 匹配高亮），加方案 = 加一行
- `applyTheme` 只换颜色，不动默认 delegate 的 padding / 边框形状
- 解析时机在直连路径**之后**：`jump s1` 直连不受 `JUMP_THEME` 笔误影响；进 TUI 前才校验，未知名字报错并列出全部可选，exit 1

## zsh 补全安装

一行扔到 `~/.zshrc` 末尾即可：

```zsh
source <(jump --completion)
```

之后 `jump <TAB>` / `j <TAB>` 会列出所有 host（带 tag/备注），顺序与 TUI 一致（MRU）。`--theme` 后面补全配色方案名，`--` 开头补全 jump 自己的选项。

## 依赖

- `github.com/charmbracelet/bubbletea`
- `github.com/charmbracelet/bubbles/list`
- `github.com/charmbracelet/lipgloss`

只读 stdlib + 上面三个包，跨 macOS / Linux。
