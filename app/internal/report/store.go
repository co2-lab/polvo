package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Store persists reports to the filesystem.
type Store struct {
	dir string
}

// NewStore creates a report store at the given directory.
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// Save persists a report to disk.
func (s *Store) Save(r *Report) error {
	// Create date-based subdirectory
	dateDir := filepath.Join(s.dir, r.Timestamp.Format("2006-01-02"))
	if err := os.MkdirAll(dateDir, 0755); err != nil {
		return fmt.Errorf("creating report directory: %w", err)
	}

	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling report: %w", err)
	}

	filename := r.ID + ".json"
	path := filepath.Join(dateDir, filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing report: %w", err)
	}

	return nil
}

// LoadByFile returns all reports for a given file, sorted by timestamp (newest first).
func (s *Store) LoadByFile(file string) ([]*Report, error) {
	var reports []*Report

	err := filepath.WalkDir(s.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".json" {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable files
		}

		var r Report
		if err := json.Unmarshal(data, &r); err != nil {
			return nil
		}

		if r.File == file {
			reports = append(reports, &r)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking report directory: %w", err)
	}

	sort.Slice(reports, func(i, j int) bool {
		return reports[i].Timestamp.After(reports[j].Timestamp)
	})

	return reports, nil
}

// LoadByAgent returns all reports from a given agent.
func (s *Store) LoadByAgent(agentName string) ([]*Report, error) {
	var reports []*Report

	err := filepath.WalkDir(s.dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".json" {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var r Report
		if err := json.Unmarshal(data, &r); err != nil {
			return nil
		}

		if r.Agent == agentName {
			reports = append(reports, &r)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking report directory: %w", err)
	}

	sort.Slice(reports, func(i, j int) bool {
		return reports[i].Timestamp.After(reports[j].Timestamp)
	})

	return reports, nil
}
