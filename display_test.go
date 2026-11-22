/*
   Copyright (c) 2026, Vitalii Tereshchuk / xVoLAnD (https://dotoca.net). All rights reserved.
   Homepage: https://dotoca.net/ytd
*/

package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestDetectDisplayModeForced(t *testing.T) {
	cases := map[string]displayMode{
		"kitty": modeKitty,
		"tgp":   modeKitty,
		"iterm": modeITerm,
		"text":  modeText,
		"plain": modePlain,
	}
	for in, want := range cases {
		if got := detectDisplayMode(in); got != want {
			t.Errorf("detectDisplayMode(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestProgressBar(t *testing.T) {
	if got := progressBar(0, 10); got != strings.Repeat("░", 10) {
		t.Fatalf("empty bar: %q", got)
	}
	if got := progressBar(50, 10); got != strings.Repeat("█", 5)+strings.Repeat("░", 5) {
		t.Fatalf("half bar: %q", got)
	}
	if got := progressBar(150, 10); got != strings.Repeat("█", 10) {
		t.Fatalf("clamped bar: %q", got)
	}
}

func TestTruncateAndWrap(t *testing.T) {
	if got := truncateRunes("abcdef", 3); got != "ab…" {
		t.Fatalf("truncateRunes: %q", got)
	}
	lines := wrapRunes("Big Buck Bunny 60fps 4K Official Blender Foundation", 12, 2)
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %q", lines)
	}
	for _, line := range lines {
		if len([]rune(line)) > 12 {
			t.Fatalf("line too long: %q", line)
		}
	}
}

func TestHumanReadableValues(t *testing.T) {
	if got := humanBytes(37_888_000); !strings.HasSuffix(got, "MiB") {
		t.Fatalf("humanBytes: %q", got)
	}
	if got := humanETA(19); got != "19s" {
		t.Fatalf("humanETA: %q", got)
	}
	if got := humanETA(125); got != "2m05s" {
		t.Fatalf("humanETA: %q", got)
	}
}

func TestRGBTo256(t *testing.T) {
	if got := rgbTo256(0, 0, 0); got != 232 {
		t.Fatalf("black: %d", got)
	}
	if got := rgbTo256(255, 255, 255); got != 255 {
		t.Fatalf("white: %d", got)
	}
	if got := rgbTo256(255, 0, 0); got != 196 {
		t.Fatalf("red: %d", got)
	}
}

func TestTextLinesContainMetadata(t *testing.T) {
	c := &card{termW: 100, termH: 24}
	m := meta{
		Title:   "Big Buck Bunny 60fps 4K - Official Blender Foundation",
		Channel: "Blender", Height: 1080, FPS: 60, Duration: 634,
	}
	p := progress{Percent: 13.4, Downloaded: 37_888_000, Total: 286_000_000, Speed: 3_400_000, ETA: 19}
	lines := c.textLines(p, m, 50)
	if len(lines) < 5 {
		t.Fatalf("too few lines: %q", lines)
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"Big Buck Bunny", "Blender", "1080p60", "10:34", "13.4%", "MiB"} {
		if !strings.Contains(joined, want) {
			t.Errorf("card text missing %q:\n%s", want, joined)
		}
	}
}

func TestRenderHalfBlocks(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 6), G: uint8(y * 12), B: 128, A: 255})
		}
	}
	lines := renderHalfBlocks(img, 10, 5, true)
	if len(lines) != 5 {
		t.Fatalf("want 5 rendered rows, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "\x1b[38;2;") {
		t.Fatalf("expected truecolor escape: %q", lines[0])
	}
}

func TestKittyAndITermEscapeSequences(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	const oldID = kittyImageID
	const newID = kittyImageID + 1
	if err := kittyTransmit(&out, img, newID, oldID, 4, 2); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.HasPrefix(got, "\x1b_Ga=T,f=100,t=d,q=2,C=1") {
		t.Fatalf("new image must be placed first: %q", got[:60])
	}
	if !strings.Contains(got, "c=4,r=2") {
		t.Fatalf("missing placement size: %q", got[:120])
	}
	transmitAt := strings.Index(got, "a=T,")
	deleteAt := strings.Index(got, "a=d,d=i")
	if transmitAt < 0 || deleteAt < 0 || transmitAt > deleteAt {
		t.Fatalf("old image must be deleted only after the new one is visible: %q", got[:160])
	}
	if !strings.HasSuffix(got, "\x1b\\") {
		t.Fatalf("missing APC terminator")
	}

	out.Reset()
	if err := itermPlace(&out, img, 4, 2); err != nil {
		t.Fatal(err)
	}
	got = out.String()
	if !strings.HasPrefix(got, "\x1b]1337;File=name=") || !strings.Contains(got, "inline=1:") {
		t.Fatalf("unexpected iTerm2 sequence: %q", got[:80])
	}
}

func TestApplyProgressWipe(t *testing.T) {
	// A source with a hard colour split down the middle: left half red,
	// right half cyan. Grayscaling both produces the same grey value.
	src := image.NewNRGBA(image.Rect(0, 0, 200, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 200; x++ {
			c := color.NRGBA{R: 200, G: 40, B: 40, A: 255}
			if x >= 100 {
				c = color.NRGBA{R: 40, G: 200, B: 200, A: 255}
			}
			src.SetNRGBA(x, y, c)
		}
	}

	empty := applyProgressWipe(src, 0)
	for _, p := range []image.Point{{10, 10}, {190, 10}} {
		c := empty.NRGBAAt(p.X, p.Y)
		if c.R != c.G || c.G != c.B {
			t.Fatalf("0%% must be fully grayscale, got %+v", c)
		}
	}

	half := applyProgressWipe(src, 50)
	left := half.NRGBAAt(20, 10)
	right := half.NRGBAAt(180, 10)
	if left.R <= left.G+50 {
		t.Fatalf("left part must keep its colour: %+v", left)
	}
	if right.R != right.G || right.G != right.B {
		t.Fatalf("right part must be grayscale: %+v", right)
	}
	divider := half.NRGBAAt(99, 10) // fillX is 100, line is one pixel to the left
	nearby := half.NRGBAAt(90, 10)
	if divider.G <= nearby.G {
		t.Fatalf("boundary must be lighter than its surroundings: %+v vs %+v", divider, nearby)
	}
	if divider.G >= 200 {
		t.Fatalf("divider must be subtle (25%% brightness), got %+v", divider)
	}

	full := applyProgressWipe(src, 100)
	if got := full.NRGBAAt(180, 10); got.G <= got.R+50 {
		t.Fatalf("100%% must be fully coloured, got %+v", got)
	}
}

func TestIncrementalTextRedraw(t *testing.T) {
	var out bytes.Buffer
	c := newCard(&out, nil, cardConfig{mode: modeKitty})
	c.termW, c.termH = 100, 24
	c.cardRows = 6
	if err := c.start(); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	if err := c.update(progress{Percent: 10, Total: 100}, meta{Title: "Title"}); err != nil {
		t.Fatal(err)
	}
	first := out.String()
	if !strings.Contains(first, "Title") {
		t.Fatalf("first frame must draw text: %q", first)
	}
	if strings.Contains(first, "\x1b[K") {
		t.Fatalf("text must be overwritten, not erased: %q", first)
	}

	out.Reset()
	if err := c.update(progress{Percent: 11, Total: 100}, meta{Title: "Title"}); err != nil {
		t.Fatal(err)
	}
	second := out.String()
	// No erase step at all: changed rows are overwritten in a single sweep.
	if strings.Contains(second, "\x1b[2K") || strings.Contains(second, "\x1b[K") {
		t.Fatalf("incremental redraw must not erase cells: %q", second)
	}
	if !strings.Contains(second, "11.0%") {
		t.Fatalf("new percentage missing: %q", second)
	}
}

func TestCloseKeepsKittyImageOnScreen(t *testing.T) {
	var out bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	c := newCard(&out, img, cardConfig{
		mode:       modeKitty,
		cellWidth:  defaultCellWidth,
		cellHeight: defaultCellHeight,
	})
	c.termW, c.termH = 100, 24
	if err := c.start(); err != nil {
		t.Fatal(err)
	}
	if err := c.update(progress{Percent: 100, Total: 100}, meta{Title: "Test"}); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	if err := c.close("video.mp4"); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Contains(got, "a=d,d=i") {
		t.Fatalf("finished preview must stay on screen: %q", got)
	}
	if strings.Contains(got, "\x1b[2K") {
		t.Fatalf("close must not clear the preview rows: %q", got)
	}
	if !strings.Contains(got, "Downloaded: video.mp4") {
		t.Fatalf("summary missing: %q", got)
	}
}

func TestCardStartUpdateClosePlain(t *testing.T) {
	var out bytes.Buffer
	c := newCard(&out, nil, cardConfig{mode: modePlain})
	if err := c.start(); err != nil {
		t.Fatal(err)
	}
	if err := c.update(progress{Percent: 42, Downloaded: 42, Total: 100}, meta{Title: "Test"}); err != nil {
		t.Fatal(err)
	}
	if err := c.close("video.mp4"); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "42.0%") || !strings.Contains(got, "Downloaded: video.mp4") {
		t.Fatalf("unexpected plain output: %q", got)
	}
}
