#!/usr/bin/env bash
# 构建 jump 二进制到 ~/bin/jump，并保证 ~/bin/j 软链存在。
set -euo pipefail

cd "$(dirname "$0")"

BIN_DIR="${BIN_DIR:-$HOME/bin}"
mkdir -p "$BIN_DIR"

echo ">> go build -o $BIN_DIR/jump ."
go build -ldflags="-s -w" -o "$BIN_DIR/jump" .

# 短命令软链 j -> jump
ln -sf jump "$BIN_DIR/j"

echo ">> done:"
ls -la "$BIN_DIR/jump" "$BIN_DIR/j"
