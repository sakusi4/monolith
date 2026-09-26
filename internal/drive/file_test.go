package drive

import "testing"

func TestContentType(t *testing.T) {
	tests := []struct {
		name string
		file string
		head []byte
		want string
	}{
		{"from the extension", "photo.png", nil, "image/png"},
		{"from the content without an extension", "scan", []byte("\x89PNG\r\n\x1a\n"), "image/png"},
		{"unknown binary", "data", []byte{0x00, 0x01, 0x02}, "application/octet-stream"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contentType(tt.file, tt.head); got != tt.want {
				t.Errorf("contentType(%q) = %q, want %q", tt.file, got, tt.want)
			}
		})
	}
}

func TestKindOf(t *testing.T) {
	tests := []struct {
		contentType string
		want        Kind
	}{
		{"image/png", KindImage},
		{"video/mp4", KindVideo},
		{"audio/mpeg", KindAudio},
		{"application/pdf", KindPDF},
		{"text/plain; charset=utf-8", KindText},
		{"application/json", KindText},
		{"application/zip", KindOther},
		{"image/png; x", KindOther},
	}
	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			if got := kindOf(tt.contentType); got != tt.want {
				t.Errorf("kindOf(%q) = %q, want %q", tt.contentType, got, tt.want)
			}
		})
	}
}

func TestMustDownload(t *testing.T) {
	tests := []struct {
		contentType string
		want        bool
	}{
		{"text/html; charset=utf-8", true},
		{"application/rss+xml", true},
		{"image/png", false},
		{"", true},
	}
	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			if got := mustDownload(tt.contentType); got != tt.want {
				t.Errorf("mustDownload(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		size int64
		want string
	}{
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{5 << 30, "5.0 GB"},
	}
	for _, tt := range tests {
		if got := formatSize(tt.size); got != tt.want {
			t.Errorf("formatSize(%d) = %q, want %q", tt.size, got, tt.want)
		}
	}
}
