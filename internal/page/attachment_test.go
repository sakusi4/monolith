package page

import (
	"slices"
	"strings"
	"testing"

	"github.com/sakusi4/monolith/internal/drive"
)

func TestFolderName(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"the title as it is", "SDN", "SDN (#7)"},
		{"slashes become dashes", "A/B test", "A-B test (#7)"},
		{"a long title is cut", strings.Repeat("가", 250), strings.Repeat("가", 200) + " (#7)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := folderName(tt.title, 7); got != tt.want {
				t.Errorf("folderName(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestUsedUploads(t *testing.T) {
	uploads := []drive.Upload{{Name: "pasted-1.png"}, {Name: "removed.png"}}
	used, unused := usedUploads("Before ![pasted-1.png](pasted-1.png) after removed.png", uploads)
	if len(used) != 1 || used[0].Name != "pasted-1.png" || len(unused) != 1 || unused[0].Name != "removed.png" {
		t.Errorf("usedUploads() = %v, %v, want pasted-1.png used and removed.png unused", used, unused)
	}
}

func TestLinkFiles(t *testing.T) {
	body := "![shot.png](shot.png) and [offer.pdf](offer.pdf)"
	want := "![shot.png](/drive/files/3/content) and [offer.pdf](/drive/files/4/content)"
	if got := linkFiles(body, map[string]int64{"shot.png": 3, "offer.pdf": 4}); got != want {
		t.Errorf("linkFiles() = %q, want %q", got, want)
	}
}

func TestDroppedFiles(t *testing.T) {
	before := "![a](/drive/files/1/content) ![b](/drive/files/2/content) [c](/drive/files/3/content)"
	after := "![b](/drive/files/2/content)"
	if got := droppedFiles(before, after); !slices.Equal(got, []int64{1, 3}) {
		t.Errorf("droppedFiles() = %v, want [1 3]", got)
	}
}
