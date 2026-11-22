/*
   Copyright (c) 2026, Vitalii Tereshchuk / xVoLAnD (https://dotoca.net). All rights reserved.
   Homepage: https://dotoca.net/ytd
*/

package main

import (
	"reflect"
	"testing"
)

func TestParseProgressLine(t *testing.T) {
	line := "YTD|abc|best|1048576|10485760|None|10.0|524288|18|downloading|/tmp/video.mp4"
	sp, ok := parseProgressLine(line)
	if !ok {
		t.Fatal("progress line was not recognised")
	}
	if sp.Downloaded != 1048576 || sp.Total != 10485760 {
		t.Fatalf("unexpected sizes: %+v", sp)
	}
	if sp.Percent != 10 || sp.Speed != 524288 || sp.ETA != 18 {
		t.Fatalf("unexpected progress values: %+v", sp)
	}
	if sp.Status != "downloading" || sp.Filename != "/tmp/video.mp4" {
		t.Fatalf("unexpected status/filename: %+v", sp)
	}
	if _, ok := parseProgressLine("[download] 10%"); ok {
		t.Fatal("ordinary yt-dlp output must not be parsed as progress")
	}
}

func TestParseProgressLineWithPipesInFilename(t *testing.T) {
	line := "YTD|abc|0|1|2||50|0|0|finished|/tmp/a|b.mp4"
	sp, ok := parseProgressLine(line)
	if !ok {
		t.Fatal("line not parsed")
	}
	if sp.Filename != "/tmp/a|b.mp4" {
		t.Fatalf("filename was cut: %q", sp.Filename)
	}
}

func TestProgressTrackerAggregatesStreams(t *testing.T) {
	var tracker progressTracker
	tracker.update(streamProgress{
		Key: "video", Downloaded: 50, Total: 100, Percent: 50, Speed: 100, ETA: 10,
	})
	p := tracker.update(streamProgress{
		Key: "audio", Downloaded: 25, Total: 50, Percent: 50, Speed: 50, ETA: 20,
	})
	if p.Downloaded != 75 || p.Total != 150 {
		t.Fatalf("unexpected aggregate sizes: %+v", p)
	}
	if p.Percent != 50 {
		t.Fatalf("unexpected percent: %v", p.Percent)
	}
	if p.Speed != 150 || p.ETA != 20 {
		t.Fatalf("unexpected speed/eta: %+v", p)
	}
}

func TestProgressTrackerUsesEstimateAndClamps(t *testing.T) {
	var tracker progressTracker
	p := tracker.update(streamProgress{
		Key: "video", Downloaded: 150, Total: 0, Estimate: 100, Percent: 150,
	})
	if p.Percent != 100 {
		t.Fatalf("percent must be clamped to 100, got %v", p.Percent)
	}
}

func TestProgressTrackerFinishOverridesPercent(t *testing.T) {
	var tracker progressTracker
	p := tracker.update(streamProgress{Key: "video", Percent: 42, Status: statusFinished})
	if p.Percent != 100 || p.Status != statusFinished {
		t.Fatalf("finished frame must be fully saturated: %+v", p)
	}
}

func TestShouldRetry403(t *testing.T) {
	logs := []string{"[youtube] id: error", "ERROR: unable to download video data: HTTP Error 403: Forbidden"}
	if !shouldRetry403(logs, []string{"URL"}) {
		t.Fatal("must retry a plain YouTube 403")
	}
	if shouldRetry403([]string{"ERROR: HTTP Error 403"}, []string{"URL"}) {
		t.Fatal("must not retry a non-YouTube 403")
	}
	if shouldRetry403(logs, []string{"URL", "--extractor-args", "youtube:player_client=android"}) {
		t.Fatal("must not override an explicit player client")
	}
	if shouldRetry403(logs, []string{"URL", "--cookies-from-browser", "firefox"}) {
		t.Fatal("must not override explicit cookies")
	}
}

func TestFilterArgs(t *testing.T) {
	args := []string{
		"https://example.com/v", "-f", "best",
		"-q", "--print", "title", "-o", "out.mp4",
		"--progress-template", "x", "--newline",
	}
	got := filterArgs(args, metadataStrip)
	want := []string{"https://example.com/v", "-f", "best"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterArgs:\n got %q\nwant %q", got, want)
	}
}

func TestFilterArgsInlineValues(t *testing.T) {
	args := []string{"URL", "--output=out.mp4", "--color=never", "-f", "bestvideo"}
	got := filterArgs(args, metadataStrip)
	want := []string{"URL", "-f", "bestvideo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filterArgs:\n got %q\nwant %q", got, want)
	}
}

func TestDownloadStripKeepsFormat(t *testing.T) {
	args := []string{"URL", "-J", "--simulate", "-f", "best"}
	got := filterArgs(args, downloadStrip)
	want := []string{"URL", "-f", "best"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("download args:\n got %q\nwant %q", got, want)
	}
}

func TestParseMetadataPlaylistUsesFirstEntry(t *testing.T) {
	data := []byte(`{
		"_type": "playlist",
		"entries": [{
			"id": "abc", "title": "First video", "channel": "Channel",
			"duration": 634, "width": 1920, "height": 1080, "fps": 60,
			"thumbnail": "http://example.com/thumb.jpg", "ext": "mp4"
		}]
	}`)
	m, err := parseMetadata(data)
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "First video" || m.thumbnailURL() != "http://example.com/thumb.jpg" {
		t.Fatalf("unexpected metadata: %+v", m)
	}
	if m.formatLabel() != "1080p60" {
		t.Fatalf("unexpected format label: %q", m.formatLabel())
	}
}

func TestFormatLabelFromResolution(t *testing.T) {
	m := meta{Resolution: "3840x2160", Height: 2160, FPS: 60}
	if got := m.formatLabel(); got != "2160p60" {
		t.Fatalf("unexpected format label: %q", got)
	}
	m = meta{Resolution: "1920x1080", Height: 1080, FPS: 30}
	if got := m.formatLabel(); got != "1080p" {
		t.Fatalf("unexpected format label: %q", got)
	}
	m = meta{Resolution: "video", Height: 720, FPS: 30}
	if got := m.formatLabel(); got != "video" {
		t.Fatalf("unexpected format label: %q", got)
	}
}

func TestThumbnailURLSelectsLargest(t *testing.T) {
	data := []byte(`{
		"thumbnail": "http://example.com/small.jpg",
		"thumbnails": [
			{"url": "http://example.com/small.jpg", "width": 120, "height": 90},
			{"url": "http://example.com/large.jpg", "width": 1280, "height": 720}
		]
	}`)
	m, err := parseMetadata(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.thumbnailURL(); got != "http://example.com/large.jpg" {
		t.Fatalf("unexpected thumbnail: %q", got)
	}
}

func TestFormatDuration(t *testing.T) {
	cases := map[float64]string{
		0:    "",
		634:  "10:34",
		3725: "1:02:05",
		59.6: "1:00",
	}
	for in, want := range cases {
		if got := formatDuration(in); got != want {
			t.Errorf("formatDuration(%v) = %q, want %q", in, got, want)
		}
	}
}
