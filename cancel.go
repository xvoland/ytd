/*
   Copyright (c) 2026, Vitalii Tereshchuk / xVoLAnD (https://dotoca.net). All rights reserved.
   Homepage: https://dotoca.net/ytd
*/

package main

import (
	"context"
	"sync"
	"time"

	"golang.org/x/term"
)

// cancelKey reports whether bytes read from the terminal mean "cancel".
// A standalone ESC cancels; escape sequences (arrows, function keys) do not.
func cancelKey(buf []byte, n int) bool {
	if n <= 0 {
		return false
	}
	switch buf[0] {
	case 0x1b: // ESC
		if n == 1 {
			return true
		}
		// ESC [ A (arrows), ESC O P (function keys) and similar sequences are
		// not a cancel request.
		if buf[1] == '[' || buf[1] == 'O' {
			return false
		}
		return true
	case 0x03: // Ctrl+C, in case raw mode bypasses signal handling
		return true
	default:
		return false
	}
}

// watchCancel reads the controlling terminal in raw mode and calls cancel when
// ESC (or Ctrl+C) is pressed. The returned function restores the terminal and
// must be called before exiting.
func watchCancel(ctx context.Context, cancel context.CancelFunc) func() {
	control, err := openControlTTY()
	if err != nil {
		return func() {}
	}
	fd := int(control.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		control.Close()
		return func() {}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 16)
		for {
			n, err := control.Read(buf)
			if err != nil || n == 0 {
				return
			}
			if cancelKey(buf, n) {
				cancel()
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			cancel() // stop the reader if it is still blocked in Read
			_ = term.Restore(fd, oldState)
			_ = control.Close()
			// Closing a fd does not always interrupt a blocked Read on every OS;
			// do not let cleanup hang if the reader is still finishing.
			select {
			case <-done:
			case <-time.After(100 * time.Millisecond):
			}
		})
	}
}
