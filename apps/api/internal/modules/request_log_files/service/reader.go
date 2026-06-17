package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	rlfcontract "github.com/srapi/srapi/apps/api/internal/modules/request_log_files/contract"
)

// FileReader is the disk-backed contract.Reader implementation.
type FileReader struct {
	logDir string
}

// NewFileReader constructs a reader for the configured directory.
func NewFileReader(logDir string) *FileReader {
	return &FileReader{logDir: ResolveLogDir(logDir)}
}

// LogDir returns the directory the reader scans.
func (r *FileReader) LogDir() string {
	if r == nil {
		return ""
	}
	return r.logDir
}

// List scans the directory and returns descriptors filtered by the input.
// The result is sorted newest-first.
func (r *FileReader) List(ctx context.Context, filter rlfcontract.ListFilter) ([]rlfcontract.FileDescriptor, error) {
	if r == nil || strings.TrimSpace(r.logDir) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(r.logDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []rlfcontract.FileDescriptor{}, nil
		}
		return nil, err
	}
	out := make([]rlfcontract.FileDescriptor, 0, len(entries))
	for _, entry := range entries {
		if ctx != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isManagedLogFile(name) {
			continue
		}
		desc, ok := describeFile(r.logDir, entry, name)
		if !ok {
			continue
		}
		if filter.ErrorOnly && !desc.IsErrorOnly {
			continue
		}
		if prefix := strings.TrimSpace(filter.RequestIDPrefix); prefix != "" {
			if !strings.HasPrefix(desc.RequestID, prefix) {
				continue
			}
		}
		if filter.From != nil && desc.CreatedAt.Before(*filter.From) {
			continue
		}
		if filter.To != nil && !desc.CreatedAt.Before(*filter.To) {
			continue
		}
		out = append(out, desc)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

// Get returns one descriptor by file name. Returns ErrNotFound when the
// file does not exist or ErrInvalidName when the name escapes the
// configured directory.
func (r *FileReader) Get(_ context.Context, name string) (rlfcontract.FileDescriptor, error) {
	if r == nil {
		return rlfcontract.FileDescriptor{}, rlfcontract.ErrNotFound
	}
	if err := validateFileName(name); err != nil {
		return rlfcontract.FileDescriptor{}, err
	}
	fullPath := filepath.Join(r.logDir, name)
	info, err := os.Stat(fullPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return rlfcontract.FileDescriptor{}, rlfcontract.ErrNotFound
		}
		return rlfcontract.FileDescriptor{}, err
	}
	if info.IsDir() {
		return rlfcontract.FileDescriptor{}, rlfcontract.ErrNotFound
	}
	desc, ok := descriptorFromInfo(name, info)
	if !ok {
		return rlfcontract.FileDescriptor{}, rlfcontract.ErrNotFound
	}
	return desc, nil
}

// Open returns the raw file contents.
func (r *FileReader) Open(_ context.Context, name string) ([]byte, error) {
	if r == nil {
		return nil, rlfcontract.ErrNotFound
	}
	if err := validateFileName(name); err != nil {
		return nil, err
	}
	fullPath := filepath.Join(r.logDir, name)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, rlfcontract.ErrNotFound
		}
		return nil, err
	}
	return data, nil
}

// Delete removes one file.
func (r *FileReader) Delete(_ context.Context, name string) error {
	if r == nil {
		return rlfcontract.ErrNotFound
	}
	if err := validateFileName(name); err != nil {
		return err
	}
	fullPath := filepath.Join(r.logDir, name)
	if err := os.Remove(fullPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return rlfcontract.ErrNotFound
		}
		return err
	}
	return nil
}

// describeFile is the listing-time projection from a DirEntry to a
// FileDescriptor.
func describeFile(dir string, entry os.DirEntry, name string) (rlfcontract.FileDescriptor, bool) {
	info, err := entry.Info()
	if err != nil {
		return rlfcontract.FileDescriptor{}, false
	}
	return descriptorFromInfo(name, info)
}

// descriptorFromInfo extracts the embedded unix_ms timestamp + request id
// from the filename and assembles the FileDescriptor.
func descriptorFromInfo(name string, info os.FileInfo) (rlfcontract.FileDescriptor, bool) {
	isError := strings.HasPrefix(name, "error-")
	if !isError && !strings.HasPrefix(name, "request-") {
		return rlfcontract.FileDescriptor{}, false
	}
	rest := name
	if isError {
		rest = strings.TrimPrefix(rest, "error-")
	} else {
		rest = strings.TrimPrefix(rest, "request-")
	}
	rest = strings.TrimSuffix(rest, ".log")
	// rest is now "{unix_ms}-{request_id}".
	dash := strings.IndexByte(rest, '-')
	createdAt := info.ModTime().UTC()
	requestID := ""
	if dash > 0 {
		if ms, err := strconv.ParseInt(rest[:dash], 10, 64); err == nil {
			createdAt = time.UnixMilli(ms).UTC()
		}
		requestID = rest[dash+1:]
	} else {
		requestID = rest
	}
	return rlfcontract.FileDescriptor{
		Name:        name,
		Size:        info.Size(),
		CreatedAt:   createdAt,
		RequestID:   requestID,
		IsErrorOnly: isError,
	}, true
}

// validateFileName rejects names containing path separators or other
// characters that would let a caller escape the configured directory.
func validateFileName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return rlfcontract.ErrInvalidName
	}
	if strings.ContainsAny(name, "/\\") {
		return rlfcontract.ErrInvalidName
	}
	if name == "." || name == ".." || strings.Contains(name, "..") {
		return rlfcontract.ErrInvalidName
	}
	if !isManagedLogFile(name) {
		return rlfcontract.ErrInvalidName
	}
	return nil
}

// Ensure the reader implementation satisfies the contract surface.
var _ rlfcontract.Reader = (*FileReader)(nil)
