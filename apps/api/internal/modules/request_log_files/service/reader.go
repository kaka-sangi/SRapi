package service

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	rlfcontract "github.com/srapi/srapi/apps/api/internal/modules/request_log_files/contract"
)

const descriptorMaxLineBytes = 4 * 1024

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
		enrichDescriptorFromFile(filepath.Join(r.logDir, name), &desc)
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
	enrichDescriptorFromFile(fullPath, &desc)
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

func enrichDescriptorFromFile(path string, desc *rlfcontract.FileDescriptor) {
	if desc == nil {
		return
	}
	metadata, err := parseDescriptorMetadata(path)
	if err != nil {
		return
	}
	if value := metadata.requestInfo["Request-ID"]; value != "" {
		desc.RequestID = value
	}
	desc.UserID = metadata.requestInfo["User-ID"]
	desc.APIKeyID = metadata.requestInfo["API-Key-ID"]
	desc.AccountID = metadata.requestInfo["Account-ID"]
	desc.SourceProtocol = metadata.requestInfo["Source-Protocol"]
	desc.SourceEndpoint = metadata.requestInfo["Source-Endpoint"]
	if startedAt, ok := parseRFC3339Time(metadata.requestInfo["Started-At"]); ok {
		desc.StartedAt = &startedAt
	}

	desc.Success = parseBoolPtr(metadata.summary["Success"])
	desc.StatusCode = parsePositiveIntPtr(metadata.summary["Status"])
	desc.ErrorClass = metadata.summary["Error-Class"]
	desc.LatencyMS = parseNonNegativeIntPtr(metadata.summary["Latency-MS"])
	desc.HasSummary = len(metadata.summary) > 0
	desc.AttemptCount = metadata.attemptCount
	desc.ResponseCount = metadata.responseCount
}

type descriptorMetadata struct {
	requestInfo   map[string]string
	summary       map[string]string
	attemptCount  int
	responseCount int
}

func parseDescriptorMetadata(path string) (descriptorMetadata, error) {
	f, err := os.Open(path)
	if err != nil {
		return descriptorMetadata{}, err
	}
	defer f.Close()

	meta := descriptorMetadata{
		requestInfo: map[string]string{},
		summary:     map[string]string{},
	}
	reader := bufio.NewReaderSize(f, 64*1024)
	section := ""
	for {
		line, err := readLimitedLine(reader, descriptorMaxLineBytes)
		if errors.Is(err, io.EOF) {
			return meta, nil
		}
		if err != nil {
			return descriptorMetadata{}, err
		}
		line = strings.TrimSpace(line)
		if name, ok := parseSectionHeader(line); ok {
			section = name
			recordDescriptorSection(name, &meta)
			continue
		}
		recordDescriptorField(section, line, &meta)
	}
}

func readLimitedLine(r *bufio.Reader, maxBytes int) (string, error) {
	var line []byte
	for {
		chunk, err := r.ReadSlice('\n')
		if len(line) < maxBytes {
			remaining := maxBytes - len(line)
			if len(chunk) < remaining {
				remaining = len(chunk)
			}
			line = append(line, chunk[:remaining]...)
		}
		switch {
		case err == nil:
			return strings.TrimRight(string(line), "\r\n"), nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			if len(line) > 0 {
				return strings.TrimRight(string(line), "\r\n"), nil
			}
			return "", io.EOF
		default:
			return "", err
		}
	}
}

func parseSectionHeader(line string) (string, bool) {
	if !strings.HasPrefix(line, "===") || !strings.HasSuffix(line, "===") {
		return "", false
	}
	name := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "==="), "==="))
	if name == "" {
		return "", false
	}
	return name, true
}

func recordDescriptorSection(name string, meta *descriptorMetadata) {
	kind, value, ok := parseNumberedSectionName(name)
	if !ok {
		return
	}
	if kind == "REQUEST" {
		if value > meta.attemptCount {
			meta.attemptCount = value
		}
		return
	}
	meta.responseCount++
}

func parseNumberedSectionName(name string) (string, int, bool) {
	parts := strings.Fields(name)
	if len(parts) != 2 {
		return "", 0, false
	}
	if parts[0] != "REQUEST" && parts[0] != "RESPONSE" {
		return "", 0, false
	}
	value, err := strconv.Atoi(parts[1])
	if err != nil || value < 1 {
		return "", 0, false
	}
	return parts[0], value, true
}

func recordDescriptorField(section, line string, meta *descriptorMetadata) {
	if line == "" {
		return
	}
	key, value, ok := parseFieldLine(line)
	if !ok {
		return
	}
	switch section {
	case "REQUEST INFO":
		meta.requestInfo[key] = value
	case "SUMMARY":
		meta.summary[key] = value
	}
}

func parseFieldLine(line string) (string, string, bool) {
	sep := strings.IndexByte(line, ':')
	if sep <= 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:sep])
	value := strings.TrimSpace(line[sep+1:])
	if key == "" || value == "" {
		return "", "", false
	}
	return key, value, true
}

func parseRFC3339Time(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC(), true
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC(), true
	}
	return time.Time{}, false
}

func parseBoolPtr(value string) *bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true":
		parsed := true
		return &parsed
	case "false":
		parsed := false
		return &parsed
	default:
		return nil
	}
}

func parsePositiveIntPtr(value string) *int {
	parsed, ok := parseInt(value)
	if !ok || parsed <= 0 {
		return nil
	}
	return &parsed
}

func parseNonNegativeIntPtr(value string) *int {
	parsed, ok := parseInt(value)
	if !ok || parsed < 0 {
		return nil
	}
	return &parsed
}

func parseInt(value string) (int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, false
	}
	return parsed, true
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
