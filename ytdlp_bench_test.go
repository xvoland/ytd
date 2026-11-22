/*
   Copyright (c) 2026, Vitalii Tereshchuk / xVoLAnD (https://dotoca.net). All rights reserved.
   Homepage: https://dotoca.net/ytd
*/

package main

import "testing"

func BenchmarkParseProgressLine(b *testing.B) {
	line := "YTD|aqz-KE-bpKQ|401|277191153|712445280|NA|38.90700953201627|4231958.14144953|102|downloading|/tmp/video.mp4"
	for b.Loop() {
		parseProgressLine(line)
	}
}

func BenchmarkFilterArgs(b *testing.B) {
	args := []string{
		"https://youtu.be/aqz-KE-bpKQ", "-f", "bestvideo+bestaudio",
		"-q", "--print", "title", "-o", "out.mp4",
		"--progress-template", "x", "--newline",
	}
	for b.Loop() {
		filterArgs(args, metadataStrip)
	}
}

func BenchmarkProgressTrackerUpdate(b *testing.B) {
	var tracker progressTracker
	sp := streamProgress{
		Key: "401", Downloaded: 277191153, Total: 712445280,
		Percent: 38.9, Speed: 4231958, ETA: 102, Status: "downloading",
	}
	for b.Loop() {
		tracker.update(sp)
	}
}
