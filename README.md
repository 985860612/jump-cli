# jump-cli

**[English](README.en.md)** | 中文

终端 SSH host 选择器。读 `~/.ssh/config`，用 [bubbletea](https://github.com/charmbracelet/bubbletea) 跑一个模糊搜索 TUI，回车后直接 `exec ssh <host>`。

![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/license-MIT-blue)

![host 列表](docs/tui-list.png)

## 特性

- **模糊搜索**：输入即过滤，host 名、备注、tag、svc 全部参与匹配
- **MRU 排序**：最近用过的 host 排最前；`d` 键可从历史里抹掉单条
- **CLI 预填**：`jump prod` 先过滤再进 TUI；`jump s1` 命中 host 名直接连
- **备注约定**：ssh config 里的 `#` 注释直接变成列表描述，还能塞 `cwd=` / `tag=` / `svc=` / `cmd=` 行为 token
- **8 套配色**：`--theme` / `JUMP_THEME` / TUI 里按 `t` 实时预览并持久化
- **zsh 补全**：`jump <TAB>` 列出全部 host（MRU 序，带备注）
- **零配置**：没有任何 token 的 host 行为等同裸 `ssh <host>`；选中后 `exec` 替换进程，不套娃

## 安装

### 下载预编译二进制（推荐）

前往 [Releases](https://github.com/985860612/jump-cli/releases) 下载对应平台的文件：

| 平台 | 架构 | 文件 |
|------|------|------|
| macOS | Intel | `jump-*-darwin-amd64` |
| macOS | Apple Silicon | `jump-*-darwin-arm64` |
| Linux | x86_64 | `jump-*-linux-amd64` |
| Linux | ARM64 | `jump-*-linux-arm64` |
| Windows | x86_64 | `jump-*-windows-amd64.exe` |

```bash
# macOS / Linux
chmod +x jump-v0.1.0-darwin-arm64
sudo mv jump-v0.1.0-darwin-arm64 /usr/local/bin/jump
ln -s /usr/local/bin/jump /usr/local/bin/j  # 可选：短命令
```

### 从源码构建

```bash
git clone https://github.com/985860612/jump-cli.git
cd jump-cli
./build.sh
```

`build.sh` 会把二进制放到 `~/bin/jump`，并创建短命令软链 `~/bin/j`。

确保 `~/bin` 在你的 PATH 里：

```bash
echo 'export PATH="$HOME/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

## 使用

```bash
jump                    # 全列表 TUI
jump prod               # 先按 'prod' 过滤再进 TUI；若恰好是 host 名则直连
jump s1                 # host 名命中 → 直接 ssh s1
jump s1 -p 2222         # 直连 + 透传 -p 2222 给 ssh
jump -p 2222            # 全列表 TUI，选中后透传 -p 2222
jump --theme ocean      # 指定配色开 TUI
jump --themes           # 打印全部配色方案（带色块预览）
jump --list             # 打印 'name\tdesc'，给补全脚本用
jump --completion       # 打印 zsh 补全脚本
jump -v                 # 打印版本
```

### TUI 键位

| 键 | 行为 |
| --- | --- |
| `Enter` | 选中 → ssh（过滤态下 = 确认过滤词） |
| `/` | 进入过滤态 |
| `t` | 打开设置面板（配色选择器，↑↓ 实时预览，`Enter` 保存，`Esc` 回滚） |
| `d` | 把当前 host 从 MRU 历史里抹掉（不删 host，下次启动回落到原序） |
| `q` / `Esc` / `Ctrl+C` | 退出 |

## SSH config 备注约定

工具在标准 ssh config 之外，识别两种备注并显示在列表中：

1. **Host 行上方紧贴的 `#` 注释行**（中间不能有空行）
2. **Host 行行尾 `# 备注`**

两者可同时存在，用 ` · ` 拼接，备注也参与 fuzzy 过滤。

### 注释里的行为 token

token 会从展示文本里悄悄抽走，只触发行为：

| token | 形式 | 行为 |
| --- | --- | --- |
| `cwd=` / `cwd:` | 单 token | 选中后 `ssh -t <host> 'cd <cwd> && exec ${SHELL:-/bin/sh} -l'` |
| `tag=` / `tag:` | 单 token | 列表 title 前缀显示 `[tag]`，参与 fuzzy 过滤 |
| `svc=` / `svc:` | 单 token（多服务逗号分隔，别留空格） | 列表 description 显示 `⚙ <svc>`，标注这台机器部署了什么服务 |
| `cmd=` | **贪婪到行尾**，必须放最后 | 选中后 `ssh -t <host> '<cmd>'`，跑完即退；若同时配 `cwd=` 会先 cd |

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

## 配色方案

![设置面板](docs/settings-theme.png)
![实时预览](docs/settings-preview.png)

- **TUI 里按 `t`** 打开配色选择器：↑↓ 移动实时预览，`Enter` 持久化，`Esc` 回滚
- 持久化到 `$XDG_CONFIG_HOME/jump/config`（默认 `~/.config/jump/config`），`key=value` 每行一个
- 优先级：`--theme` 旗标 > `JUMP_THEME` 环境变量 > 配置文件 > `default`
- 内置 8 套：`default`（经典紫）、`ocean`（海蓝）、`forest`（森绿）、`amber`（琥珀）、`rose`（玫红）、`dracula`（德古拉紫粉）、`nord`（北欧冷灰蓝）、`mono`（黑白极简），`jump --themes` 看全量带色块预览

## zsh 补全

一行扔到 `~/.zshrc` 末尾：

```zsh
source <(jump --completion)
```

之后 `jump <TAB>` / `j <TAB>` 列出所有 host（MRU 序，带 tag/备注），`--theme` 后面补全配色方案名。

## MRU 历史

- 文件：`~/.local/state/jump/history`，每行 `<unix_ts>\t<host_name>`，追加写
- 读取失败/不存在静默忽略，不阻断 ssh
- 文件超过 100KB 自动 prune：保留每个 host 最新一行 ∪ 全局最近 200 行，原子写

## 跨平台构建

```bash
VERSION=v0.1.0 ./release.sh
```

产出 `darwin/amd64`、`darwin/arm64`、`linux/amd64`、`linux/arm64`、`windows/amd64` 到 `dist/`。

## 依赖

- [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea)
- [charmbracelet/bubbles](https://github.com/charmbracelet/bubbles)
- [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss)

## License

MIT
