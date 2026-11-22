# YTD — terminal video downloader with a live preview

`ytd` is a thin Golang wrapper around [yt-dlp](https://github.com/yt-dlp/yt-dlp).
It downloads videos from the terminal and shows a thumbnail that **gains colour
as the download progresses**: it starts fully grayscale and reaches full colour
at 100%. The thumbnail is the progress indicator.

The rendering code follows the approach used in
[pixu](https://github.com/xvoland/pixu), another terminal image tool by the same
author.

## How it works

1. `ytd` runs `yt-dlp -J --skip-download <your arguments>` to get the title,
   channel, duration, resolution and thumbnail URL — without downloading the
   video.
2. The thumbnail is fetched and resized to the reserved terminal area.
3. `yt-dlp` starts downloading with a custom
   `--progress-template`, so progress arrives as structured, machine-readable
   lines instead of a terminal progress bar.
4. **The image itself is the progress bar**: its coloured region grows from the
   left as the download advances, while the rest stays grayscale. In Kitty the new frame
   is transmitted with a fresh image ID and placed on top of the current one;
   only after that is the old ID deleted — no blank frame and no cursor
   movement. Text is updated line-by-line (`ESC[K` from the text column), so
   the image cells are never cleared. Kitty is used by default: `ytd` checks
   the terminal first and falls back to iTerm2 or ANSI text if the graphics
   protocol is not supported.
5. When the download finishes, a final full-colour frame is shown together with
   the output filename. **The image stays on screen** after `ytd` exits; only
   on a failed download is the card removed.

Everything that `ytd` does not recognise is passed to `yt-dlp` unchanged, so
all of yt-dlp's options, format selectors, cookies, proxies, etc. keep working.

## Requirements

- Go 1.25+ (to build from source)
- `yt-dlp` installed and available in `PATH` (or use `--ytdlp`)
- A terminal with image support is recommended; plain text works everywhere

## Installation

```bash
go install github.com/xvoland/ytd@latest
```

Or build locally:

```bash
make build-local     # produces ./bin/ytd
./bin/ytd --help
```

## Usage

```bash
# Simple download
ytd https://youtu.be/VIDEO_ID

# Pick a format with any yt-dlp option
ytd -f 'bestvideo[height<=1080]+bestaudio/best' https://youtu.be/VIDEO_ID

# Use cookies from your browser
ytd --cookies-from-browser firefox https://youtu.be/VIDEO_ID

# Force the Kitty graphics protocol and a wider thumbnail
ytd --mode kitty --width 36 https://youtu.be/VIDEO_ID

# Progress card without a thumbnail
ytd --no-preview https://youtu.be/VIDEO_ID

# Use a yt-dlp binary from a non-standard location
ytd --ytdlp /usr/local/bin/yt-dlp https://youtu.be/VIDEO_ID
```

Use `--` to stop `ytd` from interpreting further arguments:

```bash
ytd -- https://youtu.be/VIDEO_ID --some-future-yt-dlp-flag
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `--mode <name>` | `auto`, `kitty`, `iterm`, `text`, `plain` | `auto` |
| `--width <cells>` | Thumbnail width in terminal cells (env: `YTD_WIDTH`) | auto (≈1/3 of terminal) |
| `--cell-width <n>` | Terminal cell width in pixels | `10` |
| `--cell-height <n>` | Terminal cell height in pixels | `20` |
| `--no-preview` | Skip thumbnail download | off |
| `--ytdlp <path>` | Path to the yt-dlp binary | `yt-dlp` |
| `--debug` | Print diagnostic details | off |
| `--version`, `-V` | Show version and exit | — |
| `--help`, `-h` | Show help and exit | — |

### Environment variables

Environment variables provide defaults; explicit CLI flags win.

| Variable | Description |
|----------|-------------|
| `YTD_MODE` | Default display mode (`auto` probes for Kitty) |
| `YTD_WIDTH` | Default thumbnail width in terminal cells |
| `YTD_YTDLP` | Default yt-dlp binary path |
| `YTD_CELL_WIDTH` | Default cell width in pixels |
| `YTD_CELL_HEIGHT` | Default cell height in pixels |

## Display modes

| Mode | Terminals | Notes |
|------|-----------|-------|
| `kitty` | Kitty, Ghostty, WezTerm | Terminal Graphics Protocol. The image is transmitted with a fixed image ID and replaced in place. Cursor movement is disabled (`C=1`), so text is never disturbed. |
| `iterm` | iTerm2 | Native inline images (`OSC 1337; File=...`). The card area is cleared and the image re-emitted on each update. |
| `text` | any terminal | ANSI half-block characters with 24-bit or 256-colour output. |
| `plain` | pipes, logs | One updating text line, no image. Chosen automatically when stdout is not a TTY. |
| `auto` | detection (default) | Uses `kitty` whenever supported. Known terminals are detected from `TERM_PROGRAM`, `TERM` and `KITTY_WINDOW_ID`; otherwise `ytd` sends a graphics-protocol query to the terminal and uses the reply. Falls back to `iterm` or `text`. |

The Kitty implementation follows the official graphics protocol: a new image
for an existing ID requires deleting the old image first (`a=d,d=i`), then
transmitting PNG data (`a=T,f=100,t=d`) with placement size `c`/`r`. Terminal
responses are suppressed with `q=2` so they cannot leak into stdin.

## Troubleshooting

### `ERROR: unable to download video data: HTTP Error 403: Forbidden`

YouTube sometimes rejects the CDN URL for the selected format. This is a
`yt-dlp`/YouTube issue, not a rendering problem. Retry, or give yt-dlp a fresh
player client or your browser cookies:

```bash
ytd --extractor-args 'youtube:player_client=web' https://youtu.be/VIDEO_ID
ytd --cookies-from-browser firefox https://youtu.be/VIDEO_ID
```

## Development

```bash
go build ./...
go test ./...
gofmt -w *.go
```

`make build-local` embeds the git tag into `ytd --version`; a plain `go build`
reports `x.x.x (local)`.

## Copyright

```
(c) 2026, Vitalii Tereshchuk / xVoLAnD (https://dotoca.net). All rights reserved.
```

Homepage: https://dotoca.net/ytd
