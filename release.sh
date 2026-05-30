#!/usr/bin/env bash
# Cross-compile jump-cli for multiple platforms.
set -euo pipefail

cd "$(dirname "$0")"

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
DIST="dist"
rm -rf "$DIST"
mkdir -p "$DIST"

PLATFORMS=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
  "windows/amd64"
)

echo ">> Building jump-cli $VERSION"
echo

for platform in "${PLATFORMS[@]}"; do
  IFS='/' read -r GOOS GOARCH <<< "$platform"
  output="$DIST/jump-${VERSION}-${GOOS}-${GOARCH}"
  [[ "$GOOS" == "windows" ]] && output+=".exe"

  printf "  %-30s" "${GOOS}/${GOARCH}"
  CGO_ENABLED=0 GOOS=$GOOS GOARCH=$GOARCH go build \
    -ldflags="-s -w -X main.version=$VERSION" \
    -o "$output" . 2>&1 && echo "✓" || echo "✗"
done

echo
echo ">> Built:"
ls -lh "$DIST"/
