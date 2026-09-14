package main

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// FileSnapshot represents a single file's metadata and checksums
type FileSnapshot struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	IsDir   bool   `json:"is_dir"`
	MD5     string `json:"md5"`
	SHA256  string `json:"sha256"`
	IsLink  bool   `json:"is_link"`
	LinkDst string `json:"link_dst,omitempty"`
}

// DirectorySnapshot represents all files in a directory tree
type DirectorySnapshot struct {
	Root  string         `json:"root"`
	Files []FileSnapshot `json:"files"`
	Count int            `json:"count"`
}

// SnapshotDirectory walks a directory and creates a snapshot of all files
func SnapshotDirectory(rootPath string) (*DirectorySnapshot, error) {
	var files []FileSnapshot

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}

		fs := FileSnapshot{
			Path:  relPath,
			Size:  info.Size(),
			Mode:  info.Mode().String(),
			IsDir: info.IsDir(),
		}

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			fs.IsLink = true
			target, err := os.Readlink(path)
			if err == nil {
				fs.LinkDst = target
			}
		}

		// Compute checksums for regular files
		if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			md5Hash, sha256Hash, err := computeFileHashes(path)
			if err != nil {
				return err
			}
			fs.MD5 = md5Hash
			fs.SHA256 = sha256Hash
		}

		files = append(files, fs)
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort by path for consistent comparison
	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	return &DirectorySnapshot{
		Root:  rootPath,
		Files: files,
		Count: len(files),
	}, nil
}

// computeFileHashes computes MD5 and SHA256 hashes for a file
func computeFileHashes(filePath string) (string, string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", "", err
	}
	defer func() {
		_ = f.Close()
	}()

	md5h := md5.New()
	sha256h := sha256.New()
	mw := io.MultiWriter(md5h, sha256h)

	if _, err := io.Copy(mw, f); err != nil {
		return "", "", err
	}

	return fmt.Sprintf("%x", md5h.Sum(nil)), fmt.Sprintf("%x", sha256h.Sum(nil)), nil
}

// SaveSnapshot writes a snapshot to a JSON file
func SaveSnapshot(snapshot *DirectorySnapshot, filePath string) error {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

// LoadSnapshot reads a snapshot from a JSON file
func LoadSnapshot(filePath string) (*DirectorySnapshot, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var snapshot DirectorySnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, err
	}
	return &snapshot, nil
}

// ComparisonResult holds the differences between two snapshots
type ComparisonResult struct {
	Missing    []FileSnapshot   // In expected but not actual
	Extra      []FileSnapshot   // In actual but not expected
	Different  []FileDifference // Same file but different content
	MatchCount int
}

// FileDifference represents a file that differs between snapshots
type FileDifference struct {
	Path     string
	Expected FileSnapshot
	Actual   FileSnapshot
}

// CompareSnapshots compares two directory snapshots and returns differences
func CompareSnapshots(expected, actual *DirectorySnapshot) *ComparisonResult {
	result := &ComparisonResult{}

	// Create maps for faster lookup
	expectedMap := make(map[string]FileSnapshot)
	for _, f := range expected.Files {
		expectedMap[f.Path] = f
	}

	actualMap := make(map[string]FileSnapshot)
	for _, f := range actual.Files {
		actualMap[f.Path] = f
	}

	// Find missing and different files
	for _, expectedFile := range expected.Files {
		actualFile, exists := actualMap[expectedFile.Path]
		if !exists {
			result.Missing = append(result.Missing, expectedFile)
		} else if !filesEqual(expectedFile, actualFile) {
			result.Different = append(result.Different, FileDifference{
				Path:     expectedFile.Path,
				Expected: expectedFile,
				Actual:   actualFile,
			})
		} else {
			result.MatchCount++
		}
	}

	// Find extra files
	for _, actualFile := range actual.Files {
		if _, exists := expectedMap[actualFile.Path]; !exists {
			result.Extra = append(result.Extra, actualFile)
		}
	}

	return result
}

// filesEqual compares two file snapshots for equality
func filesEqual(f1, f2 FileSnapshot) bool {
	// For directories, just compare path
	if f1.IsDir && f2.IsDir {
		return f1.Path == f2.Path
	}

	// For symlinks, compare target
	if f1.IsLink && f2.IsLink {
		return f1.Path == f2.Path && f1.LinkDst == f2.LinkDst
	}

	// For regular files, compare checksums
	if !f1.IsDir && !f1.IsLink && !f2.IsDir && !f2.IsLink {
		return f1.SHA256 == f2.SHA256 && f1.Size == f2.Size && f1.Mode == f2.Mode
	}

	return false
}

// String returns a formatted string representation of comparison results
func (cr *ComparisonResult) String() string {
	s := fmt.Sprintf("Match count: %d\n", cr.MatchCount)
	if len(cr.Missing) > 0 {
		s += fmt.Sprintf("Missing files (%d):\n", len(cr.Missing))
		for _, f := range cr.Missing {
			s += fmt.Sprintf("  - %s\n", f.Path)
		}
	}
	if len(cr.Extra) > 0 {
		s += fmt.Sprintf("Extra files (%d):\n", len(cr.Extra))
		for _, f := range cr.Extra {
			s += fmt.Sprintf("  + %s\n", f.Path)
		}
	}
	if len(cr.Different) > 0 {
		s += fmt.Sprintf("Different files (%d):\n", len(cr.Different))
		for _, d := range cr.Different {
			s += fmt.Sprintf("  ! %s\n", d.Path)
			s += fmt.Sprintf("    Expected: SHA256=%s Mode=%s\n", d.Expected.SHA256, d.Expected.Mode)
			s += fmt.Sprintf("    Actual:   SHA256=%s Mode=%s\n", d.Actual.SHA256, d.Actual.Mode)
		}
	}
	return s
}

// IsIdentical returns true if snapshots are identical
func (cr *ComparisonResult) IsIdentical() bool {
	return len(cr.Missing) == 0 && len(cr.Extra) == 0 && len(cr.Different) == 0
}
