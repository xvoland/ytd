/*
   Copyright (c) 2026, Vitalii Tereshchuk / xVoLAnD (https://dotoca.net). All rights reserved.
   Homepage: https://dotoca.net/ytd
*/

package main

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"testing"
)

// benchImage builds a photo-like gradient so the colour conversions in the
// render path do not take shortcut paths on uniform pixels.
func benchImage(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x * 255 / w),
				G: uint8(y * 255 / h),
				B: uint8((x + y) * 255 / (w + h)),
				A: 255,
			})
		}
	}
	return img
}

func BenchmarkApplyProgressWipe(b *testing.B) {
	for _, width := range []int{120, 240, 480} {
		b.Run(fmt.Sprintf("w=%d", width), func(b *testing.B) {
			src := benchImage(width, width*3/8)
			for b.Loop() {
				applyProgressWipe(src, 42)
			}
		})
	}
}

func BenchmarkKittyTransmit(b *testing.B) {
	img := benchImage(240, 135)
	for b.Loop() {
		_ = kittyTransmit(io.Discard, img, 1000, 999, 24, 7)
	}
}

// BenchmarkKittyTransmitCached measures the real per-frame hot path: the
// thumbnail is encoded once per percent and reused on subsequent ticks.
func BenchmarkKittyTransmitCached(b *testing.B) {
	img := benchImage(240, 135)
	c := newCard(io.Discard, img, cardConfig{
		mode:       modeKitty,
		cellWidth:  defaultCellWidth,
		cellHeight: defaultCellHeight,
	})
	c.termW, c.termH = 100, 24
	if err := c.start(); err != nil {
		b.Fatal(err)
	}
	data, err := c.encodedFrame(42)
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		_ = kittyTransmitData(io.Discard, data, 1000, 999, 24, 7)
	}
}

func BenchmarkITermPlace(b *testing.B) {
	img := benchImage(240, 135)
	for b.Loop() {
		_ = itermPlace(io.Discard, img, 24, 7)
	}
}

func BenchmarkRenderHalfBlocks(b *testing.B) {
	img := benchImage(240, 135)
	for b.Loop() {
		renderHalfBlocks(img, 24, 7, true)
	}
}
