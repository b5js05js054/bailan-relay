//go:build windows

package main

import "testing"

func TestMinTrackSizeIsEightyPercentOfDefault(t *testing.T) {
	w, h := minTrackSize(preferredWinWidth, preferredWinHeight)
	if w != 1024 || h != 656 {
		t.Fatalf("min track = %d x %d, want 1024 x 656", w, h)
	}
}

func TestMinTrackSizeNeverCollapsesToZero(t *testing.T) {
	w, h := minTrackSize(0, 0)
	if w != 1 || h != 1 {
		t.Fatalf("min track = %d x %d, want 1 x 1", w, h)
	}
}
