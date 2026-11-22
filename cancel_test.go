/*
   Copyright (c) 2026, Vitalii Tereshchuk / xVoLAnD (https://dotoca.net). All rights reserved.
   Homepage: https://dotoca.net/ytd
*/

package main

import "testing"

func TestCancelKey(t *testing.T) {
	cases := []struct {
		name string
		buf  []byte
		want bool
	}{
		{"standalone ESC", []byte{0x1b}, true},
		{"ESC followed by arrow sequence", []byte{0x1b, '[', 'A'}, false},
		{"ESC followed by function key", []byte{0x1b, 'O', 'P'}, false},
		{"ESC followed by text", []byte{0x1b, 'x'}, true},
		{"Ctrl+C", []byte{0x03}, true},
		{"regular key", []byte{'q'}, false},
		{"empty read", []byte{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cancelKey(tc.buf, len(tc.buf)); got != tc.want {
				t.Fatalf("cancelKey(%v) = %v, want %v", tc.buf, got, tc.want)
			}
		})
	}
}
