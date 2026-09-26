package drive

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"
)

func TestBlobPath(t *testing.T) {
	sum := sha256.Sum256([]byte("hello"))
	want := filepath.Join("/data", "blobs", "2c", "f2", "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")
	if got := blobPath("/data", sum[:]); got != want {
		t.Errorf("blobPath() = %q, want %q", got, want)
	}
}

func TestNewBlobStore(t *testing.T) {
	dir := t.TempDir()
	tmp := filepath.Join(dir, "tmp")
	if err := os.MkdirAll(tmp, dirPerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "upload-1"), []byte("cut off"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newBlobStore(dir); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(tmp); err != nil || len(entries) != 0 {
		t.Errorf("tmp after newBlobStore = %d entries, %v, want the unfinished upload removed", len(entries), err)
	}
}
