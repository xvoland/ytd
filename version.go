/*
   Copyright (c) 2026, Vitalii Tereshchuk / xVoLAnD (https://dotoca.net). All rights reserved.
   Homepage: https://dotoca.net/ytd
*/

package main

import (
	"fmt"
	"strings"
)

const copyrightLine = "(c) 2026, Vitalii Tereshchuk / xVoLAnD (https://dotoca.net). All rights reserved."

// printBanner prints the project banner used by --version.
func printBanner() {
	v := displayVersion()
	label := "v" + v
	if strings.HasPrefix(v, "v") || strings.HasPrefix(v, defaultVersion) {
		label = v
	}
	fmt.Printf("YTD, %s | https://dotoca.net/ytd\n", label)
	fmt.Println(copyrightLine)
}

// displayVersion keeps a clean release tag clean and adds the build source for
// dev/dirty builds, e.g. "v1.0.0" vs "x.x.x (local)".
func displayVersion() string {
	if version == defaultVersion || buildSource == "" || buildSource == "local" {
		if version == defaultVersion {
			return version + " (" + buildSource + ")"
		}
		return version
	}
	if strings.Contains(version, buildSource) {
		return version
	}
	return version + " (" + buildSource + ")"
}

const helpText = `YTD — download videos from the terminal with a live preview.

The thumbnail is shown before the download starts and gains colour as the
download progresses. Kitty graphics are used by default: ytd checks the
terminal and falls back to iTerm2 or ANSI text when Kitty is unavailable.
ytd is a thin wrapper: all unrecognised options go to yt-dlp unchanged.

Usage:
  ytd [ytd options] <URL> [yt-dlp options...]

ytd options:
  --mode <name>        Display mode: auto, kitty, iterm, text, plain
                       (default: auto — Kitty if supported, then fallback)
  --width <cells>      Thumbnail width in terminal cells (default: ≈1/3 of
                       the terminal; also settable with YTD_WIDTH)
  --cell-width <n>     Terminal cell width in pixels (default: 10)
  --cell-height <n>    Terminal cell height in pixels (default: 20)
  --no-preview         Do not show the thumbnail, only the progress card
  --ytdlp <path>       Path to the yt-dlp binary (default: yt-dlp)
  --debug              Print diagnostic details
  --version, -V        Show version and exit
  --help, -h           Show this help and exit
  --                   Treat all following arguments as yt-dlp arguments

Environment:
  YTD_MODE             Default display mode
  YTD_WIDTH            Default thumbnail width in terminal cells
  YTD_YTDLP            Default yt-dlp binary path
  YTD_CELL_WIDTH       Default cell width in pixels
  YTD_CELL_HEIGHT      Default cell height in pixels

Examples:
  ytd https://youtu.be/VIDEO_ID
  ytd -f 'bestvideo[height<=1080]+bestaudio/best' https://youtu.be/VIDEO_ID
  ytd --mode kitty --width 36 https://youtu.be/VIDEO_ID
  ytd --no-preview https://youtu.be/VIDEO_ID

Copyright:
  ` + copyrightLine + `
`

func printHelp() {
	fmt.Print(helpText)
}
