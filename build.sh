#!/usr/bin/env bash
# ytd build script.
#
# Copyright (c) 2026, Vitalii Tereshchuk / xVoLAnD (https://dotoca.net). All rights reserved.
# Homepage: https://dotoca.net/ytd

set -euo pipefail
cd "$(dirname "$0")"

DIST="${DIST:-dist}"
BIN="${BIN:-bin}"
VERSION="${VERSION:-$(git describe --tags --abbrev=0 2>/dev/null || echo x.x.x)}"
BUILD_SOURCE="${BUILD_SOURCE:-$(git describe --tags 2>/dev/null || echo local)}"
LDFLAGS="-X main.version=${VERSION} -X main.buildSource=${BUILD_SOURCE}"

# Pure Go: no CGO, no platform-specific system libraries. yt-dlp must still be
# installed separately (or passed with --ytdlp).
TARGETS=(
  "darwin/amd64"
  "darwin/arm64"
  "linux/amd64"
  "linux/arm64"
  "linux/arm"
  "windows/amd64"
  "windows/arm64"
  "freebsd/amd64"
  "freebsd/arm64"
  "openbsd/amd64"
)

usage() {
  cat <<EOF
Usage: ./build.sh [command]

Commands:
  local      Build ./bin/ytd for the current OS/arch (default)
  all        Cross-build all supported targets into ./dist/ + checksums
  current    Build the current OS/arch into ./dist/
  test       Run go test ./...
  bench      Run benchmarks (COUNT=10, output BENCH_OUT=bench.txt)
  platforms  Show OS/terminal graphics support
  clean      Remove ./bin and ./dist
  help       Show this help

Version is injected from git tags; override with VERSION and BUILD_SOURCE.
EOF
}

build_target() {
  local goos="$1" goarch="$2" ext="" name
  [[ "$goos" == "windows" ]] && ext=".exe"
  name="ytd-${goos}-${goarch}${ext}"
  printf '%-34s' "$name"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "$LDFLAGS" -o "$DIST/$name" .
  echo "ok"
}

build_local() {
  mkdir -p "$BIN"
  echo "Building $BIN/ytd (version ${VERSION})"
  go build -trimpath -ldflags "$LDFLAGS" -o "$BIN/ytd" .
}

build_all() {
  mkdir -p "$DIST"
  local built=()
  for target in "${TARGETS[@]}"; do
    goos="${target%/*}"
    goarch="${target#*/}"
    ext=""
    [[ "$goos" == "windows" ]] && ext=".exe"
    built+=("ytd-${goos}-${goarch}${ext}")
    build_target "$goos" "$goarch"
  done
  if command -v shasum >/dev/null 2>&1; then
    (cd "$DIST" && shasum -a 256 "${built[@]}" > checksums.txt)
    echo "Checksums: $DIST/checksums.txt"
  fi
}

build_current() {
  mkdir -p "$DIST"
  build_target "$(go env GOOS)" "$(go env GOARCH)"
}

show_platforms() {
  cat <<'EOF'
OS / architecture
  macOS   amd64, arm64      full support (pure Go)
  Linux   amd64, arm64, arm full support (pure Go)
  Windows amd64, arm64      full support (console input via CONIN$)
  FreeBSD amd64, arm64      full support (pure Go)
  OpenBSD amd64             full support (pure Go)

Terminal graphics (auto mode)
  Kitty protocol     kitty, Ghostty, WezTerm, iTerm2 (macOS) -> live coloured preview
  iTerm2 protocol    iTerm2 fallback when its Kitty support is unavailable
  ANSI text          any terminal with 24-bit/256 colours, incl. Windows Terminal,
                     Terminal.app, Konsole, GNOME Terminal, xterm, Alacritty
                     tmux/screen: enable passthrough for the graphics protocols
                     Sixel is not implemented -> ANSI preview on Sixel-only terminals
  Plain text         piped stdout / no TTY

Runtime requirements
  yt-dlp in PATH (or --ytdlp <path>), Python as required by yt-dlp
EOF
}

case "${1:-local}" in
  local)   build_local ;;
  all)     build_all ;;
  current) build_current ;;
  test)    go test ./... ;;
  bench)
    count="${COUNT:-10}"
    out="${BENCH_OUT:-bench.txt}"
    go test -run '^$' -bench=. -benchmem -count="$count" ./... | tee "$out"
    ;;
  platforms) show_platforms ;;
  clean)   rm -rf "$BIN" "$DIST" ;;
  help|-h|--help) usage ;;
  *)
    echo "Unknown command: $1" >&2
    usage >&2
    exit 2
    ;;
esac
