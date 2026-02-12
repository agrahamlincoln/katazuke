package main

import "testing"

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{name: "zero bytes", bytes: 0, want: "0 B"},
		{name: "one byte", bytes: 1, want: "1 B"},
		{name: "bytes below KB", bytes: 1023, want: "1023 B"},
		{name: "exactly 1 KB", bytes: 1024, want: "1.0 KB"},
		{name: "fractional KB", bytes: 1536, want: "1.5 KB"},
		{name: "large KB", bytes: 500 * 1024, want: "500.0 KB"},
		{name: "exactly 1 MB", bytes: 1024 * 1024, want: "1.0 MB"},
		{name: "fractional MB", bytes: 1536 * 1024, want: "1.5 MB"},
		{name: "exactly 1 GB", bytes: 1024 * 1024 * 1024, want: "1.0 GB"},
		{name: "fractional GB", bytes: 1536 * 1024 * 1024, want: "1.5 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatSize(tt.bytes)
			if got != tt.want {
				t.Errorf("formatSize(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}
