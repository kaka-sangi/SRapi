package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	rlfcontract "github.com/srapi/srapi/apps/api/internal/modules/request_log_files/contract"
)

// DefaultRetention is the maximum age of a captured file before the
// background cleaner removes it. 7 days mirrors the CLIProxyAPI default
// — long enough to triage a recent regression, short enough to keep disk
// usage bounded.
const DefaultRetention = 7 * 24 * time.Hour

// DefaultMaxErrorFiles caps the number of retained error-* files. When the
// directory holds more than this, the cleaner removes the oldest first.
const DefaultMaxErrorFiles = 500

// DefaultCleanupInterval is how often the background goroutine sweeps.
const DefaultCleanupInterval = time.Hour

// CleanerConfig configures the retention sweep.
type CleanerConfig struct {
	// LogDir is the directory the cleaner scans. Typically shared with
	// the FileWriter.
	LogDir string
	// Retention is the max age. Zero falls back to DefaultRetention; a
	// negative value disables age-based eviction.
	Retention time.Duration
	// MaxErrorFiles is the per-prefix cap for error-* files. Zero falls
	// back to DefaultMaxErrorFiles; a negative value disables the cap.
	MaxErrorFiles int
	// Interval is how often the goroutine runs. Zero falls back to
	// DefaultCleanupInterval.
	Interval time.Duration
	// Now is an optional clock override for tests.
	Now func() time.Time
}

// Cleaner is the retention sweep implementation.
type Cleaner struct {
	cfg     CleanerConfig
	running atomic.Bool
}

// NewCleaner constructs the cleaner with defaults filled in.
func NewCleaner(cfg CleanerConfig) *Cleaner {
	if cfg.Retention == 0 {
		cfg.Retention = DefaultRetention
	}
	if cfg.MaxErrorFiles == 0 {
		cfg.MaxErrorFiles = DefaultMaxErrorFiles
	}
	if cfg.Interval == 0 {
		cfg.Interval = DefaultCleanupInterval
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	return &Cleaner{cfg: cfg}
}

// LogDir returns the directory the cleaner operates on.
func (c *Cleaner) LogDir() string {
	if c == nil {
		return ""
	}
	return c.cfg.LogDir
}

// Start launches a background goroutine that calls SweepOnce on startup
// then every Interval until ctx is canceled. Calling Start twice for the
// same Cleaner returns the second call as a no-op so tests can call
// freely without leaking goroutines.
func (c *Cleaner) Start(ctx context.Context) {
	if c == nil {
		return
	}
	if !c.running.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer c.running.Store(false)
		if _, err := c.SweepOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			// We deliberately swallow non-cancel errors — the cleaner
			// is a best-effort housekeeping job; an unreadable
			// directory should not panic the API. The next tick will
			// retry.
			_ = err
		}
		ticker := time.NewTicker(c.cfg.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = c.SweepOnce(ctx)
			}
		}
	}()
}

// SweepOnce performs one retention pass over the configured directory.
// Returns the number of files removed.
func (c *Cleaner) SweepOnce(ctx context.Context) (int, error) {
	if c == nil {
		return 0, nil
	}
	if strings.TrimSpace(c.cfg.LogDir) == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(c.cfg.LogDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}

	type fileMeta struct {
		path    string
		modTime time.Time
		isError bool
	}
	files := make([]fileMeta, 0, len(entries))
	for _, entry := range entries {
		if ctx != nil && ctx.Err() != nil {
			return 0, ctx.Err()
		}
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !isManagedLogFile(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, fileMeta{
			path:    filepath.Join(c.cfg.LogDir, name),
			modTime: info.ModTime(),
			isError: strings.HasPrefix(name, "error-"),
		})
	}

	deleted := 0

	// Stage 1: age-based eviction across both request-* and error-* files.
	if c.cfg.Retention > 0 {
		cutoff := c.cfg.Now().Add(-c.cfg.Retention)
		kept := files[:0]
		for _, f := range files {
			if f.modTime.Before(cutoff) {
				if err := os.Remove(f.path); err == nil {
					deleted++
				}
				continue
			}
			kept = append(kept, f)
		}
		files = kept
	}

	// Stage 2: error-file count cap. Only error-* files are subject to
	// the per-count cap because the regular request-* files are typically
	// transient and pruned by the age stage; the error cap is for the
	// case where a sustained outage produces a flood of error dumps.
	if c.cfg.MaxErrorFiles > 0 {
		errorFiles := make([]fileMeta, 0, len(files))
		for _, f := range files {
			if f.isError {
				errorFiles = append(errorFiles, f)
			}
		}
		if len(errorFiles) > c.cfg.MaxErrorFiles {
			sort.Slice(errorFiles, func(i, j int) bool {
				return errorFiles[i].modTime.Before(errorFiles[j].modTime)
			})
			surplus := len(errorFiles) - c.cfg.MaxErrorFiles
			for i := 0; i < surplus; i++ {
				if err := os.Remove(errorFiles[i].path); err == nil {
					deleted++
				}
			}
		}
	}

	return deleted, nil
}

// isManagedLogFile reports whether the cleaner is responsible for the file.
// We deliberately only touch files matching the writer's naming convention
// (request-* / error-*) so a co-located unrelated *.log file is left alone.
func isManagedLogFile(name string) bool {
	if !strings.HasSuffix(name, ".log") {
		return false
	}
	return strings.HasPrefix(name, "request-") || strings.HasPrefix(name, "error-")
}

// Ensure Cleaner satisfies the contract surface. The contract type and the
// service type both live in the request_log_files module so this is purely
// a compile-time check.
var _ rlfcontract.Cleaner = (*Cleaner)(nil)
