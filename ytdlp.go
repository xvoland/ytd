/*
   Copyright (c) 2026, Vitalii Tereshchuk / xVoLAnD (https://dotoca.net). All rights reserved.
   Homepage: https://dotoca.net/ytd
*/

package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

const (
	statusFinished = "finished"

	progressPrefix = "YTD|"

	// yt-dlp progress fields are documented in yt_dlp/downloader/common.py.
	// The final %(info.filename)s may itself contain '|', so the parser uses
	// SplitN with a limit equal to the number of fields below.
	progressTemplate = "YTD|" +
		"%(info.id)s|" +
		"%(info.format_id)s|" +
		"%(progress.downloaded_bytes)s|" +
		"%(progress.total_bytes)s|" +
		"%(progress.total_bytes_estimate)s|" +
		"%(progress._percent)s|" +
		"%(progress.speed)s|" +
		"%(progress.eta)s|" +
		"%(progress.status)s|" +
		"%(info.filename)s"

	progressFieldCount = 11
)

// streamProgress is one progress update from a single yt-dlp download stream
// (a video and an audio stream are downloaded separately by yt-dlp).
type streamProgress struct {
	Key        string
	Downloaded int64
	Total      int64
	Estimate   int64
	Percent    float64
	Speed      float64
	ETA        float64
	Status     string
	Filename   string
}

// progress is the aggregated state displayed in the card.
type progress struct {
	Percent    float64
	Downloaded int64
	Total      int64
	Speed      float64
	ETA        float64
	Status     string
}

// progressTracker keeps the latest update per download stream.
type progressTracker struct {
	streams map[string]streamProgress
}

func (t *progressTracker) update(sp streamProgress) progress {
	if t.streams == nil {
		t.streams = make(map[string]streamProgress)
	}
	key := sp.Key
	if key == "" {
		key = "default"
	}
	t.streams[key] = sp

	p := progress{Status: sp.Status}
	knownTotal := false
	for _, s := range t.streams {
		p.Downloaded += s.Downloaded
		p.Speed += s.Speed
		total := s.Total
		if total <= 0 {
			total = s.Estimate
		}
		p.Total += total
		if total > 0 {
			knownTotal = true
		}
		if s.ETA > p.ETA {
			p.ETA = s.ETA
		}
		if s.Status == statusFinished {
			p.Status = statusFinished
		}
	}

	if knownTotal && p.Total > 0 {
		p.Percent = 100 * float64(p.Downloaded) / float64(p.Total)
	} else {
		for _, s := range t.streams {
			if s.Percent > p.Percent {
				p.Percent = s.Percent
			}
		}
	}
	if p.Percent > 100 {
		p.Percent = 100
	}
	if p.Percent < 0 {
		p.Percent = 0
	}
	if p.Status == statusFinished {
		p.Percent = 100
	}
	return p
}

// parseProgressLine parses one structured progress line. It returns ok=false
// for ordinary yt-dlp output.
func parseProgressLine(line string) (streamProgress, bool) {
	if !strings.HasPrefix(line, progressPrefix) {
		return streamProgress{}, false
	}
	fields := strings.SplitN(line, "|", progressFieldCount)
	if len(fields) < progressFieldCount {
		return streamProgress{}, false
	}
	return streamProgress{
		Key:        fields[1] + ":" + fields[2],
		Downloaded: parseInt64(fields[3]),
		Total:      parseInt64(fields[4]),
		Estimate:   parseInt64(fields[5]),
		Percent:    parseFloat(fields[6]),
		Speed:      parseFloat(fields[7]),
		ETA:        parseFloat(fields[8]),
		Status:     fields[9],
		Filename:   cleanTemplateValue(fields[10]),
	}, true
}

// cleanTemplateValue drops the literal "None" that yt-dlp substitutes for
// missing template fields.
func cleanTemplateValue(s string) string {
	s = strings.TrimSpace(s)
	if s == "None" || s == "NA" {
		return ""
	}
	return s
}

func parseInt64(s string) int64 {
	f := parseFloat(s)
	if f <= 0 {
		return 0
	}
	return int64(f)
}

func parseFloat(s string) float64 {
	switch strings.TrimSpace(s) {
	case "", "None", "NA", "N/A", "unknown":
		return 0
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

// fetchMetadata asks yt-dlp for the video metadata without downloading it.
func fetchMetadata(ctx context.Context, bin string, passthrough []string, debug bool) (meta, error) {
	args := filterArgs(passthrough, metadataStrip)
	args = append(args, "-J", "--skip-download")
	if debug {
		log.Printf("metadata: %s %s", bin, strings.Join(args, " "))
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(errOut.String())
		if detail == "" {
			detail = err.Error()
		}
		return meta{}, fmt.Errorf("cannot read video metadata: %s", detail)
	}

	m, err := parseMetadata(out.Bytes())
	if err != nil {
		return meta{}, err
	}
	return m, nil
}

type downloadResult struct {
	Filename string
}

// download runs yt-dlp, feeding structured progress updates to onProgress.
// Regular output is collected so it can be shown after the card is closed.
func download(ctx context.Context, bin string, passthrough []string, onProgress func(streamProgress), logLines *[]string, debug bool) (downloadResult, error) {
	args := filterArgs(passthrough, downloadStrip)
	args = append(args,
		"--progress",
		"--newline",
		"--color", "never",
		"--progress-template", progressTemplate,
	)
	if debug {
		log.Printf("download: %s %s", bin, strings.Join(args, " "))
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return downloadResult{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return downloadResult{}, err
	}
	if err := cmd.Start(); err != nil {
		return downloadResult{}, err
	}

	var (
		mu     sync.Mutex
		result downloadResult
	)
	collect := func(line string, fromStdout bool) {
		if fromStdout {
			if sp, ok := parseProgressLine(line); ok {
				if sp.Status == statusFinished && sp.Filename != "" {
					result.Filename = sp.Filename
				}
				if onProgress != nil {
					onProgress(sp)
				}
				return
			}
		}
		line = strings.TrimRight(line, "\r")
		if line == "" {
			return
		}
		mu.Lock()
		if len(*logLines) < 200 {
			*logLines = append(*logLines, line)
		}
		if name := filenameFromLog(line); name != "" && result.Filename == "" {
			result.Filename = name
		}
		mu.Unlock()
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		scanLines(stdout, func(line string) { collect(line, true) })
	}()
	go func() {
		defer wg.Done()
		scanLines(stderr, func(line string) { collect(line, false) })
	}()

	waitErr := cmd.Wait()
	wg.Wait()

	if waitErr != nil {
		// The caller prints the collected log lines once; including them here
		// would show the same error twice.
		return result, fmt.Errorf("yt-dlp failed: %w", waitErr)
	}
	return result, nil
}

func scanLines(r io.Reader, fn func(string)) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		fn(scanner.Text())
	}
}

// shouldRetry403 reports whether a one-off YouTube 403 is worth retrying with
// an explicit player client. A user-supplied client or cookies already handle
// the problem, so we do not override those choices.
func shouldRetry403(logLines []string, passthrough []string) bool {
	joined := strings.Join(logLines, "\n")
	if !strings.Contains(joined, "HTTP Error 403") ||
		!strings.Contains(strings.ToLower(joined), "youtube") {
		return false
	}
	for _, a := range passthrough {
		if strings.HasPrefix(a, "--extractor-args") || strings.HasPrefix(a, "--cookies") {
			return false
		}
	}
	return true
}

func tailLines(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

func filenameFromLog(line string) string {
	for _, prefix := range []string{
		"[download] Destination: ",
		"[Merger] Merging formats into \"",
	} {
		if strings.HasPrefix(line, prefix) {
			name := strings.TrimPrefix(line, prefix)
			name = strings.TrimSuffix(name, "\"")
			return strings.TrimSpace(name)
		}
	}
	return ""
}

// Flags whose output would interfere with the JSON metadata or with our own
// progress card. Each value lists the number of following arguments to skip.
type stripSpec struct {
	flag   string
	values int
}

var metadataStrip = []stripSpec{
	{"-q", 0}, {"--quiet", 0}, {"--silent", 0}, {"--no-warnings", 0},
	{"-s", 0}, {"--simulate", 0}, {"--skip-download", 0}, {"--flat-playlist", 0},
	{"-J", 0}, {"--dump-json", 0}, {"--dump-single-json", 0},
	{"--print", 1}, {"--print-to-file", 2},
	{"--progress", 0}, {"--no-progress", 0}, {"--newline", 0},
	{"--console-title", 0}, {"--progress-template", 1}, {"--progress-delta", 1},
	{"--color", 1}, {"--no-colors", 0},
	{"-o", 1}, {"--output", 1},
	{"--write-thumbnail", 0}, {"--write-info-json", 0},
}

var downloadStrip = []stripSpec{
	{"-s", 0}, {"--simulate", 0}, {"--skip-download", 0}, {"--flat-playlist", 0},
	{"-J", 0}, {"--dump-json", 0}, {"--dump-single-json", 0},
	{"--print", 1}, {"--print-to-file", 2},
}

// filterArgs removes flags (and, when applicable, their values) from the
// passthrough arguments. Both "--flag value" and "--flag=value" are handled.
func filterArgs(args []string, strip []stripSpec) []string {
	byName := make(map[string]int, len(strip))
	for _, s := range strip {
		byName[s.flag] = s.values
	}

	out := make([]string, 0, len(args)+8)
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, hasInline := a, false
		if j := strings.IndexByte(a, '='); j > 0 && strings.HasPrefix(a, "-") {
			name, hasInline = a[:j], true
		}
		values, removed := byName[name]
		if !removed {
			out = append(out, a)
			continue
		}
		if hasInline {
			continue
		}
		for k := 0; k < values && i+1 < len(args); k++ {
			i++
		}
	}
	return out
}
