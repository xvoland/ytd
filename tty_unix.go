//go:build !windows

/*
   Copyright (c) 2026, Vitalii Tereshchuk / xVoLAnD (https://dotoca.net). All rights reserved.
   Homepage: https://dotoca.net/ytd
*/

package main

import "os"

// openControlTTY returns a file for reading terminal replies. On Unix the
// controlling terminal is used so the probe works even when stdin is piped.
func openControlTTY() (*os.File, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}
