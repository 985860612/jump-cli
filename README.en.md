# jump-cli

**[中文文档](README.md)** | English

A terminal SSH host selector. Reads `~/.ssh/config`, launches a fuzzy-search TUI powered by [bubbletea](https://github.com/charmbracelet/bubbletea), and `exec`s into `ssh <host>` on Enter.

![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat&logo=go)
![License](https://img.shields.io/badge/license-MIT-blue)

## Installation

### Pre-built binaries (recommended)

Download the matching binary from [Releases](https://github.com/985860612/jump-cli/releases):

| Platform | Arch | File |
|----------|------|------|
| macOS | Intel | `jump-*-darwin-amd64` |
| macOS | Apple Silicon | `jump-*-darwin-arm64` |
| Linux | x86_64 | `jump-*-linux-amd64` |
| Linux | ARM64 | `jump-*-linux-arm64` |
| Windows | x86_64 | `jump-*-windows-amd64.exe` |

```bash
# macOS / Linux
chmod +x jump-v0.1.0-darwin-arm64
sudo mv jump-v0.1.0-darwin-arm64 /usr/local/bin/jump
ln -s /usr/local/bin/jump /usr/local/bin/j  # optional short alias
```

### Build from source

```bash
git clone https://github.com/985860612/jump-cli.git
cd jump-cli
./build.sh
```

`build.sh` installs the binary to `~/bin/jump` and creates a symlink `~/bin/j`.

Make sure `~/bin` is in your PATH:

```bash
echo 'export PATH="$HOME/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

## Usage

```bash
jump          # launch the TUI, fuzzy-search SSH hosts
j             # same thing (short alias)
jump -v       # print version
jump -p 2222  # pass extra flags through to ssh
```

- Type to fuzzy-filter hosts
- `Enter` to select and connect
- `Esc` / `Ctrl+C` to quit

## SSH config comment conventions

On top of standard ssh config parsing, jump recognizes two kinds of comments and displays them in the list:

1. **`#` comment line immediately above a `Host` line** (no blank line in between)
2. **Trailing `# comment` at the end of a `Host` line**

Both may coexist and will be joined with ` · `. Comments are also included in fuzzy matching.

### Preset remote working directory `cwd=`

A `cwd=/path` or `cwd:/path` token in the comment causes jump to `cd` into that directory on the remote host upon connection:

```sshconfig
# cwd=/data/app  Aliyun Beijing  Customer A
Host s1
    HostName 1.2.3.4
    User root
```

Hosts without a `cwd=` token behave exactly like a plain `ssh <host>`.

## Cross-platform build

```bash
VERSION=v0.1.0 ./release.sh
```

Produces binaries for `darwin/amd64`, `darwin/arm64`, `linux/amd64`, `linux/arm64`, and `windows/amd64` in `dist/`.

## Dependencies

- [charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea)
- [charmbracelet/bubbles](https://github.com/charmbracelet/bubbles)
- [charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss)

## License

MIT
