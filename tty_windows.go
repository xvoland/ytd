//go:build windows

/*
   Copyright (c) 2026, Vitalii Tereshchuk / xVoLAnD (https://dotoca.net). All rights reserved.
   Homepage: https://dotoca.net/ytd
*/

package main

import "os"

// openControlTTY returns the Windows console input handle. The graphics query
// itself is written to stdout; replies are read from this handle.
func openControlTTY() (*os.File, error) {
	return os.OpenFile("CONIN$", os.O_RDONLY, 0)
}
