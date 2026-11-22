/*
   Copyright (c) 2026, Vitalii Tereshchuk / xVoLAnD (https://dotoca.net). All rights reserved.
   Homepage: https://dotoca.net/ytd
*/

package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"net/http"
	"time"

	"github.com/disintegration/imaging"
)

var previewClient = &http.Client{Timeout: 15 * time.Second}

// loadPreview downloads the video thumbnail advertised in yt-dlp metadata.
// The caller receives the full-size image; the card resizes it to the terminal.
func loadPreview(ctx context.Context, m meta) (image.Image, error) {
	url := m.thumbnailURL()
	if url == "" {
		return nil, fmt.Errorf("no thumbnail URL in metadata")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("thumbnail request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ytd/1.0)")
	req.Header.Set("Accept", "image/avif,image/webp,image/png,image/jpeg,image/*;q=0.8")

	resp, err := previewClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download thumbnail: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download thumbnail: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("read thumbnail: %w", err)
	}
	img, err := imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
	if err != nil {
		return nil, fmt.Errorf("decode thumbnail: %w", err)
	}
	return img, nil
}
