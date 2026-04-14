package filelock

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
)

// FileVersion is a snapshot of a file's state used for optimistic CAS.
type FileVersion struct {
	Path    string
	Hash    string // SHA-256 hex of content at read time
	Content []byte
}

// ErrConflict is returned when a file was modified by another agent since it was read.
type ErrConflict struct {
	Path           string
	ExpectedHash   string
	CurrentHash    string
	CurrentContent []byte
}

func (e *ErrConflict) Error() string {
	return fmt.Sprintf("conflict on %s: expected hash %s but current hash is %s",
		e.Path, e.ExpectedHash, e.CurrentHash)
}

// hashContent returns the SHA-256 hex digest of data.
func hashContent(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ReadVersioned reads the file at path and returns a FileVersion with its SHA-256 hash.
func ReadVersioned(path string) (FileVersion, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return FileVersion{}, fmt.Errorf("reading %s: %w", path, err)
	}
	return FileVersion{
		Path:    path,
		Hash:    hashContent(data),
		Content: data,
	}, nil
}

// WriteIfUnchanged writes newContent to expected.Path only if the file's current
// hash matches expected.Hash. Returns *ErrConflict if another agent modified the
// file in the meantime.
//
// The caller must hold the write lock for the file before calling this function.
func WriteIfUnchanged(path string, expected FileVersion, newContent []byte) error {
	current, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading current content of %s: %w", path, err)
	}

	var currentHash string
	if err == nil {
		currentHash = hashContent(current)
	}
	// If the file doesn't exist yet and expected.Hash is empty, that's a match
	// (both represent "no file").

	if currentHash != expected.Hash {
		return &ErrConflict{
			Path:           path,
			ExpectedHash:   expected.Hash,
			CurrentHash:    currentHash,
			CurrentContent: current,
		}
	}

	if err := os.WriteFile(path, newContent, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}
