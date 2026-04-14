package repomap

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Indexer watches for file changes and maintains the chunk index.
type Indexer struct {
	root  string
	index *ChunkIndex
}

// NewIndexer creates a new Indexer for the given root directory and chunk index.
func NewIndexer(root string, index *ChunkIndex) *Indexer {
	return &Indexer{root: root, index: index}
}

// IndexAll performs a full scan of root, chunks all supported files, and upserts
// only the ones whose content has changed (incremental by file hash).
func (ix *Indexer) IndexAll(ctx context.Context) error {
	const maxFileSize = 500 * 1024

	err := filepath.WalkDir(ix.root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}

		// Respect context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			if skipIndexDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if !isSupportedSource(d.Name()) {
			return nil
		}

		info, err := d.Info()
		if err != nil || info.Size() > maxFileSize {
			return nil
		}

		return ix.indexFileAt(path)
	})
	return err
}

// IndexFile chunks a single file and upserts changed chunks into the index.
func (ix *Indexer) IndexFile(path string) error {
	return ix.indexFileAt(path)
}

// indexFileAt is the shared implementation used by IndexAll and IndexFile.
func (ix *Indexer) indexFileAt(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	fh := fileHash(content)

	// Check existing chunks for this path; if the file hash is unchanged, skip.
	existing, err := ix.index.GetByPath(path)
	if err != nil {
		return fmt.Errorf("getting existing chunks for %s: %w", path, err)
	}
	if len(existing) > 0 && existing[0].FileHash == fh {
		// File unchanged — nothing to do
		return nil
	}

	// File changed (or new): delete old chunks and index fresh ones.
	if len(existing) > 0 {
		if err := ix.index.DeleteByPath(path); err != nil {
			return fmt.Errorf("deleting stale chunks for %s: %w", path, err)
		}
	}

	chunks, err := ChunkFile(path, content)
	if err != nil {
		return fmt.Errorf("chunking %s: %w", path, err)
	}
	if len(chunks) == 0 {
		return nil
	}

	return ix.index.UpsertBatch(chunks)
}

// cleanPath returns a cleaned absolute path.
func cleanPath(path string) string {
	return filepath.Clean(strings.TrimSpace(path))
}
