/*
   Copyright (c) 2026, Vitalii Tereshchuk / xVoLAnD (https://dotoca.net). All rights reserved.
   Homepage: https://dotoca.net/ytd
*/

package main

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// meta is the subset of yt-dlp's JSON metadata that the progress card needs.
type meta struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	Channel    string  `json:"channel"`
	Uploader   string  `json:"uploader"`
	Duration   float64 `json:"duration"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	FPS        float64 `json:"fps"`
	Resolution string  `json:"resolution"`
	Thumbnail  string  `json:"thumbnail"`
	Thumbnails []struct {
		URL    string `json:"url"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	} `json:"thumbnails"`
	Ext     string `json:"ext"`
	Type    string `json:"_type"`
	Entries []meta `json:"entries"`
}

// parseMetadata decodes yt-dlp --dump-single-json output. Playlists expose an
// "entries" array; the first playable entry is used for the preview card.
func parseMetadata(data []byte) (meta, error) {
	var m meta
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parse yt-dlp metadata: %w", err)
	}
	if m.Type == "playlist" && len(m.Entries) > 0 {
		m = m.Entries[0]
	}
	if m.Title == "" {
		m.Title = m.ID
	}
	return m, nil
}

// thumbnailURL picks the largest thumbnail advertised by yt-dlp. Some
// extractors only provide the single "thumbnail" field.
func (m meta) thumbnailURL() string {
	best := m.Thumbnail
	bestArea := 0
	for _, t := range m.Thumbnails {
		if t.URL == "" {
			continue
		}
		area := t.Width * t.Height
		if area > bestArea {
			best, bestArea = t.URL, area
		}
	}
	return best
}

// formatLabel returns a short quality label for the card, e.g. "1080p60".
// yt-dlp usually reports resolution as "3840x2160", which is shortened to the
// height-based form used by video players.
func (m meta) formatLabel() string {
	if m.Resolution != "" {
		parts := strings.Split(m.Resolution, "x")
		if len(parts) == 2 {
			if h, err := strconv.Atoi(parts[1]); err == nil && h > 0 {
				return heightLabel(h, m.FPS)
			}
		}
		return m.Resolution
	}
	if m.Height <= 0 {
		return ""
	}
	return heightLabel(m.Height, m.FPS)
}

func heightLabel(height int, fps float64) string {
	label := fmt.Sprintf("%dp", height)
	if fps >= 50 {
		label += strconv.Itoa(int(math.Round(fps)))
	}
	return label
}

// sourceLabel builds the "YouTube · Blender · 1080p60" style subtitle.
func (m meta) sourceLabel() string {
	parts := make([]string, 0, 4)
	if m.Channel != "" {
		parts = append(parts, m.Channel)
	} else if m.Uploader != "" {
		parts = append(parts, m.Uploader)
	}
	if label := m.formatLabel(); label != "" {
		parts = append(parts, label)
	}
	return strings.Join(parts, " · ")
}

// formatDuration renders seconds as m:ss or h:mm:ss.
func formatDuration(seconds float64) string {
	if seconds <= 0 {
		return ""
	}
	total := int(math.Round(seconds))
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}
