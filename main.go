/*
   Copyright (c) 2026, Vitalii Tereshchuk / xVoLAnD (https://dotoca.net). All rights reserved.
   Homepage: https://dotoca.net/ytd
*/

// Command ytd is a terminal download wrapper around yt-dlp. It shows a preview
// of the video thumbnail which gains colour as the download progresses.
package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "golang.org/x/image/webp"
	_ "image/gif"
	_ "image/jpeg"
)

const defaultVersion = "x.x.x"

// version is injected at build time via -ldflags "-X main.version=...".
var version = defaultVersion

// buildSource is injected at build time and shown only for non-release builds.
var buildSource = "local"

type options struct {
	ytdlpBin    string
	mode        string
	imageWidth  int
	cellWidth   int
	cellHeight  int
	noPreview   bool
	debug       bool
	showHelp    bool
	showVersion bool
	passthrough []string
}

func main() {
	log.SetFlags(0)

	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "ytd: %v\n", err)
		fmt.Fprintln(os.Stderr, "Try 'ytd --help' for more information.")
		os.Exit(2)
	}
	if opts.showHelp {
		printHelp()
		return
	}
	if opts.showVersion {
		printBanner()
		return
	}
	if err := run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "ytd: %v\n", err)
		os.Exit(1)
	}
}

// parseArgs separates ytd's own flags from the arguments that must be passed
// to yt-dlp. Everything that is not a ytd flag is forwarded verbatim, in the
// original order, so any yt-dlp option keeps working.
func parseArgs(args []string) (options, error) {
	o := options{
		ytdlpBin:   "yt-dlp",
		mode:       "auto",
		cellWidth:  defaultCellWidth,
		cellHeight: defaultCellHeight,
	}

	// Priority: explicit CLI flag > environment variable > default.
	if v := os.Getenv("YTD_YTDLP"); v != "" {
		o.ytdlpBin = v
	}
	if v := os.Getenv("YTD_MODE"); v != "" {
		o.mode = v
	}
	if v := os.Getenv("YTD_WIDTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			o.imageWidth = n
		}
	}
	if v := os.Getenv("YTD_CELL_WIDTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			o.cellWidth = n
		}
	}
	if v := os.Getenv("YTD_CELL_HEIGHT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			o.cellHeight = n
		}
	}

	passthrough := make([]string, 0, len(args))

	valueFlags := map[string]func(string) error{
		"--ytdlp": func(v string) error { o.ytdlpBin = v; return nil },
		"--mode":  func(v string) error { o.mode = v; return nil },
		"--width": func(v string) error {
			n, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("invalid --width %q: expected a number", v)
			}
			o.imageWidth = n
			return nil
		},
		"--cell-width": func(v string) error {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				return fmt.Errorf("invalid --cell-width %q: expected a positive number", v)
			}
			o.cellWidth = n
			return nil
		},
		"--cell-height": func(v string) error {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				return fmt.Errorf("invalid --cell-height %q: expected a positive number", v)
			}
			o.cellHeight = n
			return nil
		},
	}

	for i := 0; i < len(args); i++ {
		a := args[i]

		switch a {
		case "--":
			passthrough = append(passthrough, args[i+1:]...)
			i = len(args)
			continue
		case "--help", "-h":
			o.showHelp = true
			continue
		case "--version", "-V":
			o.showVersion = true
			continue
		case "--no-preview":
			o.noPreview = true
			continue
		case "--debug":
			o.debug = true
			continue
		}

		handled := false
		for name, setter := range valueFlags {
			if a == name {
				if i+1 >= len(args) {
					return o, fmt.Errorf("%s requires a value", name)
				}
				i++
				if err := setter(args[i]); err != nil {
					return o, err
				}
				handled = true
				break
			}
			if strings.HasPrefix(a, name+"=") {
				if err := setter(strings.TrimPrefix(a, name+"=")); err != nil {
					return o, err
				}
				handled = true
				break
			}
		}
		if handled {
			continue
		}

		passthrough = append(passthrough, a)
	}

	o.passthrough = passthrough
	return o, nil
}

func run(o options) error {
	if len(o.passthrough) == 0 {
		// Running without arguments shows the version and copyright banner,
		// like other tools by the same author.
		printBanner()
		fmt.Println()
		return errors.New("no URL given (see 'ytd --help')")
	}
	if _, err := exec.LookPath(o.ytdlpBin); err != nil {
		return fmt.Errorf("%s not found in PATH: %w", o.ytdlpBin, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	started := time.Now()

	meta, err := fetchMetadata(ctx, o.ytdlpBin, o.passthrough, o.debug)
	if err != nil {
		return err
	}

	var base image.Image
	if !o.noPreview {
		var previewErr error
		base, previewErr = loadPreview(ctx, meta)
		if previewErr != nil && o.debug {
			log.Printf("preview: %v", previewErr)
		}
	}

	card := newCard(os.Stdout, base, cardConfig{
		mode:       detectDisplayMode(o.mode),
		imageWidth: o.imageWidth,
		cellWidth:  o.cellWidth,
		cellHeight: o.cellHeight,
	})
	if err := card.start(); err != nil {
		return err
	}
	defer card.close("")

	var (
		tracker      progressTracker
		lastDraw     time.Time
		lastPct      = -1.0
		lastProgress progress
		drawErr      error
		logLines     []string
	)

	onProgress := func(sp streamProgress) {
		p := tracker.update(sp)
		lastProgress = p
		if drawErr != nil {
			return
		}
		now := time.Now()
		pctChanged := p.Percent-lastPct >= 0.5 || p.Percent < lastPct
		timePassed := now.Sub(lastDraw) >= 120*time.Millisecond
		finished := p.Status == statusFinished || p.Percent >= 100
		if !(pctChanged || timePassed) && !finished {
			return
		}
		lastPct, lastDraw = p.Percent, now
		drawErr = card.update(p, meta)
	}

	result, err := download(ctx, o.ytdlpBin, o.passthrough, onProgress, &logLines, o.debug)
	if err != nil && shouldRetry403(logLines, o.passthrough) {
		// YouTube can reject a CDN URL once and accept it on the next attempt.
		// Reset the indicator and retry once with an explicit player client.
		tracker = progressTracker{}
		lastPct, lastDraw, lastProgress, drawErr = -1, time.Time{}, progress{}, nil
		fmt.Fprintln(os.Stderr, "ytd: YouTube returned 403; retrying with the web player client...")
		retryArgs := append(append([]string(nil), o.passthrough...),
			"--extractor-args", "youtube:player_client=web")
		retryLogs := make([]string, 0)
		result, err = download(ctx, o.ytdlpBin, retryArgs, onProgress, &retryLogs, o.debug)
		logLines = append(logLines, retryLogs...)
	}
	if err != nil {
		_ = card.abort()
		if len(logLines) > 0 {
			fmt.Fprintln(os.Stderr, strings.Join(tailLines(logLines, 8), "\n"))
		}
		if strings.Contains(strings.Join(logLines, "\n"), "HTTP Error 403") {
			fmt.Fprintln(os.Stderr, "Hint: YouTube can require cookies: --cookies-from-browser <browser>")
		}
		return err
	}

	// Always paint a final, fully saturated frame before the summary.
	if lastProgress.Percent < 100 {
		lastProgress.Percent = 100
		lastProgress.Status = statusFinished
		_ = card.update(lastProgress, meta)
	}

	finalName := result.Filename
	if finalName == "" {
		finalName = suggestedFilename(meta)
	}
	_ = card.close(finalName)

	if len(logLines) > 0 && o.debug {
		fmt.Println(strings.Join(logLines, "\n"))
	}
	if o.debug {
		log.Printf("finished in %s", time.Since(started).Round(time.Millisecond))
	}
	return nil
}

// suggestedFilename builds a readable fallback name when yt-dlp did not report
// the final path (for example for formats without a destination line).
func suggestedFilename(m meta) string {
	name := m.Title
	if name == "" {
		name = m.ID
	}
	if name == "" {
		return "video"
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.', r == ' ':
			return r
		default:
			return '_'
		}
	}, name)
	name = strings.TrimSpace(name)
	if len(name) > 80 {
		name = name[:80]
	}
	if m.Ext != "" {
		return name + "." + m.Ext
	}
	return name
}
