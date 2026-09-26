package drive

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

const (
	sniffLen = 512
	dirPerm  = 0o750
)

// blobStore keeps file contents on disk under dir, each in a file named by its SHA-256. Callers
// hold mu while they keep content or remove content no file refers to, so that a removal never
// deletes the same content that was just uploaded again.
type blobStore struct {
	dir string
	mu  sync.Mutex
}

// stagedBlob is content written to a temporary file and not yet kept.
type stagedBlob struct {
	path string
	sum  []byte
	size int64
	head []byte
}

// newBlobStore removes the uploads that an earlier run of the server left unfinished, so only one
// process may use dir.
func newBlobStore(dir string) (*blobStore, error) {
	if err := os.RemoveAll(filepath.Join(dir, "tmp")); err != nil {
		return nil, fmt.Errorf("clear unfinished uploads: %w", err)
	}
	for _, d := range []string{filepath.Join(dir, "blobs"), filepath.Join(dir, "tmp")} {
		if err := os.MkdirAll(d, dirPerm); err != nil {
			return nil, fmt.Errorf("create %s: %w", d, err)
		}
	}
	return &blobStore{dir: dir}, nil
}

// blobPath is where the content with sum is kept under dir.
func blobPath(dir string, sum []byte) string {
	h := hex.EncodeToString(sum)
	return filepath.Join(dir, "blobs", h[:2], h[2:4], h)
}

// stage copies r into a temporary file while hashing it.
func (b *blobStore) stage(r io.Reader) (stagedBlob, error) {
	f, err := os.CreateTemp(filepath.Join(b.dir, "tmp"), "upload-*")
	if err != nil {
		return stagedBlob{}, fmt.Errorf("create temp file: %w", err)
	}
	staged, err := writeHashed(f, r)
	if err = errors.Join(err, f.Close()); err != nil {
		return stagedBlob{}, errors.Join(err, os.Remove(f.Name()))
	}
	staged.path = f.Name()
	return staged, nil
}

func writeHashed(f *os.File, r io.Reader) (stagedBlob, error) {
	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(f, h), r)
	if err != nil {
		return stagedBlob{}, fmt.Errorf("write temp file: %w", err)
	}
	if err := f.Sync(); err != nil {
		return stagedBlob{}, fmt.Errorf("sync temp file: %w", err)
	}
	head := make([]byte, sniffLen)
	n, err := f.ReadAt(head, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return stagedBlob{}, fmt.Errorf("read temp file: %w", err)
	}
	return stagedBlob{sum: h.Sum(nil), size: size, head: head[:n]}, nil
}

// keep moves staged content into place and reports whether it was new. When the same content
// is already kept, it only removes the temporary file. Callers hold b.mu.
func (b *blobStore) keep(s stagedBlob) (bool, error) {
	dst := blobPath(b.dir, s.sum)
	_, err := os.Stat(dst)
	if err == nil {
		return false, b.discard(s)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("stat blob: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), dirPerm); err != nil {
		return false, fmt.Errorf("create blob directory: %w", err)
	}
	if err := os.Rename(s.path, dst); err != nil {
		return false, fmt.Errorf("move blob: %w", err)
	}
	return true, nil
}

// discard removes the temporary file of s. A file that is already gone is not an error.
func (b *blobStore) discard(s stagedBlob) error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove temp file: %w", err)
	}
	return nil
}

func (b *blobStore) open(sum []byte) (*os.File, error) {
	f, err := os.Open(blobPath(b.dir, sum))
	if err != nil {
		return nil, fmt.Errorf("open blob: %w", err)
	}
	return f, nil
}

// remove deletes the content with sum. Content that is already gone is not an error. Callers hold b.mu.
func (b *blobStore) remove(sum []byte) error {
	if err := os.Remove(blobPath(b.dir, sum)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove blob: %w", err)
	}
	return nil
}
