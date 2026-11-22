/*
   Copyright (c) 2026, Vitalii Tereshchuk / xVoLAnD (https://dotoca.net). All rights reserved.
   Homepage: https://dotoca.net/ytd
*/

package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/mattn/go-isatty"
	"golang.org/x/term"
)

const (
	defaultCellWidth  = 10
	defaultCellHeight = 20

	// Fixed image id so every update replaces the same kitty image. 0x5944...
	// spells "YD" in ASCII and makes stray ids from other programs unlikely.
	kittyImageID uint32 = 0x59440001

	kittyChunkSize = 4096
)

type displayMode int

const (
	modeKitty displayMode = iota // kitty, Ghostty, WezTerm
	modeITerm                    // iTerm2 native inline images
	modeText                     // ANSI half-blocks
	modePlain                    // one-line text, for pipes and logs
)

// detectDisplayMode maps --mode to a renderer. The default ("auto") tries the
// Kitty graphics protocol first — including an active terminal query — and
// falls back to iTerm2's native protocol or ANSI text when it is unavailable.
func detectDisplayMode(force string) displayMode {
	switch strings.ToLower(strings.TrimSpace(force)) {
	case "kitty", "tgp":
		return modeKitty
	case "iterm", "iterm2":
		return modeITerm
	case "text", "ansi":
		return modeText
	case "plain", "none":
		return modePlain
	}

	fd := uintptr(os.Stdout.Fd())
	if !isatty.IsTerminal(fd) && !isatty.IsCygwinTerminal(fd) {
		return modePlain
	}

	// Known Kitty-protocol terminals need no probing.
	switch os.Getenv("TERM_PROGRAM") {
	case "Ghostty", "WezTerm":
		return modeKitty
	}
	if os.Getenv("TERM") == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != "" {
		return modeKitty
	}

	// iTerm2 supports both protocols; prefer Kitty's in-place image updates.
	if os.Getenv("TERM_PROGRAM") == "iTerm.app" {
		if probeKittyGraphics() {
			return modeKitty
		}
		return modeITerm
	}

	if probeKittyGraphics() {
		return modeKitty
	}
	return modeText
}

// probeKittyGraphics sends a Kitty graphics query action to /dev/tty and waits
// briefly for the terminal's acknowledgement. A query does not store the image
// or change what is on screen, so it is safe to use before drawing the card.
func probeKittyGraphics() bool {
	control, err := openControlTTY()
	if err != nil {
		return false
	}
	defer control.Close()

	fd := int(control.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return false
	}
	defer term.Restore(fd, oldState)

	var payload bytes.Buffer
	if err := png.Encode(&payload, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		return false
	}
	query := fmt.Sprintf("\x1b_Ga=q,f=100,t=d,i=%d;%s\x1b\\",
		kittyImageID, base64.StdEncoding.EncodeToString(payload.Bytes()))
	if _, err := os.Stdout.WriteString(query); err != nil {
		return false
	}

	response := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4096)
		n, _ := control.Read(buf)
		response <- buf[:n]
	}()

	select {
	case data := <-response:
		s := string(data)
		return strings.Contains(s, "\x1b_G") && strings.Contains(s, ";OK")
	case <-time.After(400 * time.Millisecond):
		return false
	}
}

type cardConfig struct {
	mode                  displayMode
	imageWidth            int
	cellWidth, cellHeight int
}

// card owns the reserved screen area and all frame drawing.
type card struct {
	out  io.Writer
	cfg  cardConfig
	base *image.NRGBA // pre-resized, full-colour thumbnail

	termW, termH int
	imgCols      int
	imgRows      int
	cardRows     int
	trueColor    bool
	hasImage     bool
	started      bool

	kittyID   uint32   // current kitty image id (double-buffered updates)
	prevLines []string // previously drawn text lines, for incremental redraw

	buf bytes.Buffer
}

func newCard(out io.Writer, base image.Image, cfg cardConfig) *card {
	var nrgba *image.NRGBA
	if base != nil {
		if converted, ok := base.(*image.NRGBA); ok {
			nrgba = converted
		} else {
			nrgba = imaging.Clone(base)
		}
	}
	seed := uint32(time.Now().UnixNano())
	if seed == 0 {
		seed = 1
	}
	return &card{
		out:     out,
		cfg:     cfg,
		base:    nrgba,
		kittyID: seed, // unique per run: the final image outlives the process
	}
}

// start reserves the card area on screen and remembers its origin with DECSC.
func (c *card) start() error {
	c.termW, c.termH = terminalSize()
	c.trueColor = supportsTrueColor()
	c.started = true

	if c.cfg.mode == modePlain {
		c.cardRows = 1
		return nil
	}

	if c.base != nil {
		bounds := c.base.Bounds()
		bw, bh := bounds.Dx(), bounds.Dy()
		if bw > 0 && bh > 0 {
			c.imgCols = c.preferredImageCols()
			rows := int(math.Round(float64(bh) * float64(c.imgCols) / float64(bw) / 2))
			c.imgRows = clampInt(rows, 5, 14)
			if maxRows := c.termH - 4; c.imgRows > maxRows {
				c.imgRows = maxInt(maxRows, 3)
			}
			pw := c.imgCols * c.cfg.cellWidth
			ph := c.imgRows * c.cfg.cellHeight
			c.base = imaging.Resize(c.base, pw, ph, imaging.Lanczos)
			c.hasImage = true
		}
	}

	c.cardRows = c.imgRows
	if c.cardRows < 6 {
		c.cardRows = 6
	}
	if maxRows := c.termH - 2; c.cardRows > maxRows {
		c.cardRows = maxInt(maxRows, 3)
	}

	c.buf.WriteString(strings.Repeat("\n", c.cardRows))
	fmt.Fprintf(&c.buf, "\x1b[%dA", c.cardRows)
	c.buf.WriteString("\x1b[s")    // save cursor at card origin
	c.buf.WriteString("\x1b[?25l") // hide cursor
	c.prevLines = nil
	return c.flush()
}

func (c *card) preferredImageCols() int {
	if c.cfg.imageWidth > 0 {
		return clampInt(c.cfg.imageWidth, 4, maxInt(c.termW-4, 4))
	}
	cols := c.termW / 3
	if cols < 20 {
		cols = 20
	}
	if cols > 34 {
		cols = 34
	}
	if c.termW < 60 {
		cols = maxInt(12, c.termW/2)
	}
	return clampInt(cols, 4, maxInt(c.termW-4, 4))
}

// update paints one frame: a more saturated thumbnail and refreshed text.
func (c *card) update(p progress, m meta) error {
	c.buf.Reset()

	pad := 0
	avail := c.termW - 1
	if c.hasImage {
		pad = c.imgCols + 2
		avail = c.termW - pad - 1
	}
	if avail < 12 {
		avail = 12
	}

	if c.cfg.mode == modePlain {
		pct := fmt.Sprintf("%5.1f%%", p.Percent)
		sizes := fmt.Sprintf("%s / %s", humanBytes(p.Downloaded), humanBytes(p.Total))
		c.buf.WriteString("\r\x1b[2K" + pct + "  " + sizes + "  " + paceLine(p))
		return c.flush()
	}

	lines := c.textLines(p, m, avail)

	// Kitty images are overlays on top of the cells, so the text can be
	// updated without touching the image area. Other modes must erase the
	// image cells before redrawing them.
	if c.cfg.mode != modeKitty {
		c.buf.WriteString("\x1b[u")
		c.clearCard()
	}
	c.buf.WriteString("\x1b[u")

	if c.hasImage {
		// The image itself is the progress bar: the coloured region grows
		// from the left as the download advances.
		img := applyProgressWipe(c.base, p.Percent)
		switch c.cfg.mode {
		case modeKitty:
			nextID := c.kittyID + 1
			if nextID == 0 {
				nextID = 1
			}
			if err := kittyTransmit(&c.buf, img, nextID, c.kittyID, c.imgCols, c.imgRows); err != nil {
				return err
			}
			c.kittyID = nextID
		case modeITerm:
			if err := itermPlace(&c.buf, img, c.imgCols, c.imgRows); err != nil {
				return err
			}
		case modeText:
			left := renderHalfBlocks(img, c.imgCols, c.imgRows, c.trueColor)
			for i := 0; i < c.cardRows; i++ {
				c.buf.WriteString("\x1b[u")
				if i > 0 {
					fmt.Fprintf(&c.buf, "\x1b[%dB", i)
				}
				if i < len(left) {
					c.buf.WriteString(left[i])
				}
				if i < len(lines) {
					c.buf.WriteString("\x1b[2C")
					c.writeTextLine(lines[i], i, avail)
				}
			}
			return c.flush()
		}
	}

	if c.cfg.mode == modeKitty {
		c.drawTextIncremental(lines, pad, avail)
		return c.flush()
	}

	c.buf.WriteString("\x1b[u")
	if pad > 0 {
		fmt.Fprintf(&c.buf, "\x1b[%dC", pad)
	}
	for i, line := range lines {
		c.writeTextLine(line, i, avail)
		if i < len(lines)-1 {
			c.buf.WriteString("\r\n")
			if pad > 0 {
				fmt.Fprintf(&c.buf, "\x1b[%dC", pad)
			}
		}
	}
	return c.flush()
}

// drawTextIncremental rewrites only the lines that actually changed. New text
// is padded to the previous line's width and written over it in one pass — no
// ESC[K erase step — so the terminal never paints a half-empty line. All
// changed rows are emitted in one left-to-right, top-to-bottom sweep instead
// of jumping back to the saved cursor for every line.
func (c *card) drawTextIncremental(lines []string, pad, avail int) {
	count := maxInt(len(lines), len(c.prevLines))
	if count == 0 {
		return
	}

	c.buf.WriteString("\x1b[u")
	if pad > 0 {
		fmt.Fprintf(&c.buf, "\x1b[%dC", pad)
	}
	for i := 0; i < count; i++ {
		current, previous := "", ""
		if i < len(lines) {
			current = lines[i]
		}
		if i < len(c.prevLines) {
			previous = c.prevLines[i]
		}
		if current != previous {
			visible := truncateRunes(current, avail)
			if width, oldWidth := len([]rune(visible)), len([]rune(previous)); width < oldWidth {
				visible += strings.Repeat(" ", oldWidth-width)
			}
			c.writeTextLine(visible, i, avail)
		}
		if i < count-1 {
			c.buf.WriteString("\r\n")
			if pad > 0 {
				fmt.Fprintf(&c.buf, "\x1b[%dC", pad)
			}
		}
	}
	c.prevLines = append([]string(nil), lines...)
}

func (c *card) writeTextLine(line string, index, avail int) {
	line = truncateRunes(line, avail)
	// Colour is applied after truncation so escape sequences cannot be cut.
	switch index {
	case 2: // percentage and byte counts
		line = "\x1b[1;32m" + line + "\x1b[0m"
	case 3: // progress bar
		line = "\x1b[32m" + line + "\x1b[0m"
	}
	c.buf.WriteString(line)
}

// textLines builds the plain-text card content. Colour is applied by
// writeTextLine once each line has been truncated to the available width.
func (c *card) textLines(p progress, m meta, avail int) []string {
	if avail < 8 {
		avail = 8
	}
	lines := wrapRunes(m.Title, avail, 2)
	if len(lines) == 0 {
		lines = []string{"yt-dlp"}
	}

	sub := m.sourceLabel()
	if d := formatDuration(m.Duration); d != "" {
		if sub != "" {
			sub += " · "
		}
		sub += d
	}
	if sub == "" {
		sub = "yt-dlp"
	}
	lines = append(lines, truncateRunes(sub, avail))

	pct := fmt.Sprintf("%5.1f%%", p.Percent)
	if p.Total > 0 {
		pct += fmt.Sprintf("  %s / %s", humanBytes(p.Downloaded), humanBytes(p.Total))
	} else if p.Downloaded > 0 {
		pct += fmt.Sprintf("  %s", humanBytes(p.Downloaded))
	}
	lines = append(lines, pct)

	barWidth := clampInt(avail-4, 8, 32)
	lines = append(lines, progressBar(p.Percent, barWidth))
	lines = append(lines, paceLine(p))
	return lines
}

// close clears the card, prints the final summary and restores the cursor.
func (c *card) close(finalName string) error {
	if !c.started {
		return nil
	}
	c.buf.Reset()
	if c.cfg.mode == modePlain {
		if finalName != "" {
			fmt.Fprintf(&c.buf, "\nDownloaded: %s\n", finalName)
		}
		c.started = false
		return c.flush()
	}

	// Keep the finished, fully coloured preview on screen. Only the summary
	// is printed, on the first row below the image, and the kitty image is
	// deliberately NOT deleted.
	summaryRow := c.imgRows
	if summaryRow >= c.cardRows {
		summaryRow = c.cardRows
	}
	c.buf.WriteString("\x1b[u")
	if summaryRow > 0 {
		fmt.Fprintf(&c.buf, "\x1b[%dB", summaryRow)
	}
	if finalName != "" {
		fmt.Fprintf(&c.buf, "\x1b[1;32m✓\x1b[0m Downloaded: %s\n", truncateRunes(finalName, c.termW-2))
	}
	c.buf.WriteString("\x1b[?25h")
	c.started = false
	return c.flush()
}

// abort removes the card after a failed or interrupted download.
func (c *card) abort() error {
	if !c.started {
		return nil
	}
	c.buf.Reset()
	if c.cfg.mode == modePlain {
		c.started = false
		return c.flush()
	}
	c.buf.WriteString("\x1b[u")
	c.clearCard()
	c.buf.WriteString("\x1b[u")
	c.buf.WriteString("\x1b[1;31m✗\x1b[0m Download failed or interrupted\n")
	// Leave the cursor just after the summary line instead of below the
	// reserved area, so no blank lines appear before the next output.
	c.buf.WriteString("\x1b[?25h")
	if c.cfg.mode == modeKitty {
		kittyDelete(&c.buf, c.kittyID)
	}
	c.started = false
	return c.flush()
}

func (c *card) clearCard() {
	for i := 0; i < c.cardRows; i++ {
		c.buf.WriteString("\x1b[2K")
		if i < c.cardRows-1 {
			c.buf.WriteString("\x1b[B")
		}
	}
}

func (c *card) flush() error {
	if c.buf.Len() == 0 {
		return nil
	}
	_, err := c.out.Write(c.buf.Bytes())
	c.buf.Reset()
	return err
}

// --- graphics protocol helpers ------------------------------------------------

func kittyDelete(w io.Writer, id uint32) {
	fmt.Fprintf(w, "\x1b_Ga=d,d=i,i=%d,q=2\x1b\\", id)
}

// kittyTransmit places a new image at the current cursor position and removes
// the previous one. A fresh (higher) image id is used on every frame: kitty
// draws the higher id on top, so the new image is already visible when the old
// one is deleted. This avoids the one-frame blank gap of delete-then-retransmit
// and does not move the cursor (C=1).
func kittyTransmit(w io.Writer, img image.Image, id, oldID uint32, cols, rows int) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return fmt.Errorf("encode preview PNG: %w", err)
	}
	data := base64.StdEncoding.EncodeToString(buf.Bytes())

	for i := 0; i < len(data); i += kittyChunkSize {
		end := i + kittyChunkSize
		if end > len(data) {
			end = len(data)
		}
		more := 0
		if end < len(data) {
			more = 1
		}
		if i == 0 {
			fmt.Fprintf(w, "\x1b_Ga=T,f=100,t=d,q=2,C=1,i=%d,p=1,c=%d,r=%d,m=%d;%s\x1b\\",
				id, cols, rows, more, data[i:end])
		} else {
			fmt.Fprintf(w, "\x1b_Gm=%d;%s\x1b\\", more, data[i:end])
		}
	}
	if oldID != 0 && oldID != id {
		kittyDelete(w, oldID)
	}
	return nil
}

// itermPlace uses iTerm2's native inline image protocol with explicit cell
// dimensions. The image is re-emitted on every frame because iTerm2 has no
// image-id update command; the card area is cleared first.
func itermPlace(w io.Writer, img image.Image, cols, rows int) error {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return fmt.Errorf("encode preview PNG: %w", err)
	}
	name := base64.StdEncoding.EncodeToString([]byte("ytd-preview.png"))
	data := base64.StdEncoding.EncodeToString(buf.Bytes())
	fmt.Fprintf(w, "\x1b]1337;File=name=%s;width=%d;height=%d;preserveAspectRatio=1;inline=1:%s\a",
		name, cols, rows, data)
	return nil
}

// applyProgressWipe turns the preview into the progress indicator: pixels to
// the left of the fill boundary keep their original colour, pixels to the
// right are converted to grayscale. A short feathered edge avoids a hard seam,
// and a thin white line marks the boundary itself.
func applyProgressWipe(src *image.NRGBA, pct float64) *image.NRGBA {
	if src == nil {
		return nil
	}
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	out := image.NewNRGBA(bounds)
	copy(out.Pix, src.Pix)

	fillX := int(float64(w) * clamp01(pct/100))
	if fillX >= w || w == 0 {
		return out // 100%: the whole image is coloured
	}

	const feather = 6
	for y := 0; y < h; y++ {
		row := y * out.Stride
		for x := 0; x < w; x++ {
			weight := 1.0
			if x > fillX-feather {
				weight = float64(fillX-x) / feather
				if weight > 1 {
					weight = 1
				} else if weight < 0 {
					weight = 0
				}
			}
			if weight >= 1 {
				continue
			}
			i := row + x*4
			r, g, b := int(src.Pix[i]), int(src.Pix[i+1]), int(src.Pix[i+2])
			gray := (77*r + 150*g + 29*b) >> 8
			out.Pix[i] = uint8(float64(gray) + weight*float64(r-gray))
			out.Pix[i+1] = uint8(float64(gray) + weight*float64(g-gray))
			out.Pix[i+2] = uint8(float64(gray) + weight*float64(b-gray))
		}
	}

	// A thin white divider makes the progress boundary visible even when the
	// thumbnail itself is light. It is part of the image, not a terminal overlay.
	if fillX > 0 && fillX < w {
		lineWidth := clampInt(w/120, 1, 3)
		const opacity = 0.25 // 25% of white: visible, but not a harsh stripe
		for y := 0; y < h; y++ {
			row := y * out.Stride
			for x := fillX - lineWidth; x < fillX; x++ {
				if x < 0 || x >= w {
					continue
				}
				i := row + x*4
				for channel := 0; channel < 3; channel++ {
					v := float64(out.Pix[i+channel])
					out.Pix[i+channel] = uint8(v + (255-v)*opacity)
				}
			}
		}
	}
	return out
}

// --- text fallback ------------------------------------------------------------

// rgbTo256 maps an RGB colour to the nearest xterm-256 colour. Integer maths
// mirrors the implementation used by the pixu project.
func rgbTo256(r, g, b uint8) int {
	ri, gi, bi := int(r), int(g), int(b)
	sum := ri + gi + bi
	maxDiff3 := abs3(3*ri - sum)
	if d := abs3(3*gi - sum); d > maxDiff3 {
		maxDiff3 = d
	}
	if d := abs3(3*bi - sum); d > maxDiff3 {
		maxDiff3 = d
	}
	if maxDiff3 <= 30 {
		gray := (46*sum + 765) / 1530
		if gray > 23 {
			gray = 23
		}
		return 232 + gray
	}
	r6 := minInt((ri*10+255)/510, 5)
	g6 := minInt((gi*10+255)/510, 5)
	b6 := minInt((bi*10+255)/510, 5)
	return 16 + 36*r6 + 6*g6 + b6
}

func abs3(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// renderHalfBlocks renders an image using the upper-half-block character so
// that one terminal row carries two source pixel rows.
func renderHalfBlocks(img image.Image, cols, rows int, trueColor bool) []string {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	resized := imaging.Resize(img, cols, rows*2, imaging.Lanczos)
	// imaging.Resize returns *image.NRGBA, so pixels can be read directly.
	rgba := resized

	lines := make([]string, 0, rows)
	for y := 0; y < rows*2; y += 2 {
		var sb strings.Builder
		sb.Grow(cols * 30)
		for x := 0; x < cols; x++ {
			i := y*rgba.Stride + x*4
			tr, tg, tb := rgba.Pix[i], rgba.Pix[i+1], rgba.Pix[i+2]
			j := (y+1)*rgba.Stride + x*4
			br, bg, bb := tr, tg, tb
			if y+1 < rows*2 {
				br, bg, bb = rgba.Pix[j], rgba.Pix[j+1], rgba.Pix[j+2]
			}
			if trueColor {
				fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm▀", tr, tg, tb, br, bg, bb)
			} else {
				fmt.Fprintf(&sb, "\x1b[38;5;%d;48;5;%dm▀", rgbTo256(tr, tg, tb), rgbTo256(br, bg, bb))
			}
		}
		sb.WriteString("\x1b[0m")
		lines = append(lines, sb.String())
	}
	return lines
}

// --- small helpers ------------------------------------------------------------

func terminalSize() (int, int) {
	fd := int(os.Stdout.Fd())
	w, h, err := term.GetSize(fd)
	if err != nil {
		if tty, e := os.Open("/dev/tty"); e == nil {
			w, h, err = term.GetSize(int(tty.Fd()))
			tty.Close()
		}
	}
	if err != nil || w <= 0 || h <= 0 {
		return 80, 24
	}
	return w, h
}

func supportsTrueColor() bool {
	ct := strings.ToLower(os.Getenv("COLORTERM"))
	return strings.Contains(ct, "truecolor") || strings.Contains(ct, "24bit")
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		hi = lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// truncateRunes truncates to n visible characters. Callers must not pass
// strings containing ANSI codes; styling is applied after truncation.
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n == 1 {
		return string(runes[0])
	}
	return string(runes[:n-1]) + "…"
}

// wrapRunes wraps s into at most maxLines lines of width cells.
func wrapRunes(s string, width, maxLines int) []string {
	if strings.TrimSpace(s) == "" || width <= 0 || maxLines <= 0 {
		return nil
	}
	var lines []string
	current := ""
	for _, word := range strings.Fields(s) {
		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		if len([]rune(candidate)) <= width {
			current = candidate
			continue
		}
		if current != "" {
			lines = append(lines, current)
			current = ""
		}
		if len([]rune(word)) > width {
			word = truncateRunes(word, width)
		}
		current = word
	}
	if current != "" {
		lines = append(lines, current)
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		last := lines[maxLines-1]
		if len([]rune(last)) >= width && width > 1 {
			lines[maxLines-1] = truncateRunes(last, width)
		} else {
			lines[maxLines-1] += "…"
		}
	}
	return lines
}

func progressBar(pct float64, width int) string {
	pct = clamp01(pct / 100)
	filled := int(pct * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

// paceLine renders speed and remaining time, e.g. "3.2 MiB/s · 19s left".
func paceLine(p progress) string {
	parts := make([]string, 0, 2)
	if p.Speed > 0 {
		parts = append(parts, humanBytes(int64(p.Speed))+"/s")
	}
	if p.ETA > 0 && p.Status != statusFinished {
		parts = append(parts, humanETA(p.ETA)+" left")
	}
	if p.Status == statusFinished {
		parts = append(parts, "done")
	}
	return strings.Join(parts, " · ")
}

// humanBytes renders bytes using binary units.
func humanBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	units := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	i := -1
	for value >= unit && i < len(units)-1 {
		value /= unit
		i++
	}
	return fmt.Sprintf("%.1f %s", value, units[i])
}

// humanETA renders seconds as a compact time estimate.
func humanETA(seconds float64) string {
	if seconds <= 0 {
		return "0s"
	}
	if seconds < 60 {
		return strconv.Itoa(int(math.Round(seconds))) + "s"
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm%02ds", int(seconds)/60, int(seconds)%60)
	}
	return fmt.Sprintf("%dh%02dm", int(seconds)/3600, (int(seconds)%3600)/60)
}
