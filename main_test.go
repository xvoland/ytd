/*
   Copyright (c) 2026, Vitalii Tereshchuk / xVoLAnD (https://dotoca.net). All rights reserved.
   Homepage: https://dotoca.net/ytd
*/

package main

import (
	"os"
	"testing"

	"github.com/mattn/go-isatty"
)

func TestParseArgsWidthFromEnv(t *testing.T) {
	t.Setenv("YTD_WIDTH", "42")
	opts, err := parseArgs([]string{"URL"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.imageWidth != 42 {
		t.Fatalf("YTD_WIDTH ignored: got %d", opts.imageWidth)
	}

	// Explicit CLI flag wins over the environment.
	opts, err = parseArgs([]string{"--width", "18", "URL"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.imageWidth != 18 {
		t.Fatalf("--width must override YTD_WIDTH: got %d", opts.imageWidth)
	}
}

func TestParseArgsModeFromEnv(t *testing.T) {
	t.Setenv("YTD_MODE", "text")
	opts, err := parseArgs([]string{"URL"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.mode != "text" {
		t.Fatalf("YTD_MODE ignored: got %q", opts.mode)
	}
}

func TestParseArgsForwardsUnknownFlags(t *testing.T) {
	opts, err := parseArgs([]string{
		"URL", "-f", "bestvideo+bestaudio",
		"--cookies-from-browser", "firefox", "--no-playlist",
		"--width", "24",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"URL", "-f", "bestvideo+bestaudio", "--cookies-from-browser", "firefox", "--no-playlist"}
	if len(opts.passthrough) != len(want) {
		t.Fatalf("passthrough: got %q, want %q", opts.passthrough, want)
	}
	for i := range want {
		if opts.passthrough[i] != want[i] {
			t.Fatalf("passthrough: got %q, want %q", opts.passthrough, want)
		}
	}
	if opts.imageWidth != 24 {
		t.Fatalf("--width not parsed: %d", opts.imageWidth)
	}
}

func TestDetectDisplayModeAutoWithoutTTY(t *testing.T) {
	fd := uintptr(os.Stdout.Fd())
	if isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd) {
		t.Skip("stdout is a terminal in this test run")
	}
	if got := detectDisplayMode("auto"); got != modePlain {
		t.Fatalf("non-TTY auto mode = %v, want modePlain", got)
	}
}
