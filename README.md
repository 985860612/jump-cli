# jump-cli

**[English](README.en.md)** | 中文

终端 SSH host 选择器。读 `~/.ssh/config`，用 [bubbletea](https://github.com/charmbracelet/bubbletea) 跑一个模糊搜索 TUI，回车后直接 `exec ssh <host>`。

![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/license-MIT-blue)

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
jump          # 打开 TUI，模糊搜索 SSH host
j             # 同上（短命令）
jump -p 2222  # 透传参数给 ssh
```

- 输入关键字模糊过滤 host
- `Enter` 选中并连接
- `Esc` / `Ctrl+C` 退出

## SSH config 备注约定

工具在标准 ssh config 之外，识别两种备注并显示在列表中：

1. **Host 行上方紧贴的 `#` 注释行**（中间不能有空行）
2. **Host 行行尾 `# 备注`**

两者可同时存在，用 ` · ` 拼接，备注也参与 fuzzy 过滤。

### 预设远端工作目录 `cwd=`

在备注里写 `cwd=/path` 或 `cwd:/path`，选中后会自动 cd 到该目录：

```sshconfig
# cwd=/data/app  阿里云北京 客户A
Host s1
    HostName 1.2.3.4
    User root
```

未配 `cwd=` 的 host 行为等同 `ssh <host>`。

## 依赖

- [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea)
- [charmbracelet/bubbles](https://github.com/charmbracelet/bubbles)
- [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss)

## License

MIT
