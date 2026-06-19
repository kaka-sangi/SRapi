package service

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

// DefaultMaxTotalBytes caps managed request-log-file disk usage. The request
// capture is an operator-controlled diagnostic tool; a hard default prevents
// an accidentally enabled deployment from growing without bound.
const DefaultMaxTotalBytes = 512 * 1024 * 1024

// EnvMaxTotalMB caps total managed request-log-file disk usage in MiB. Zero or
// unset uses DefaultMaxTotalBytes; negative values disable size-based eviction.
const EnvMaxTotalMB = "SRAPI_REQUEST_LOG_MAX_TOTAL_MB"

// DefaultCleanupInterval is how often the background goroutine sweeps.
const DefaultCleanupInterval = time.Hour

const maxTotalMiB = (1<<63 - 1) / (1024 * 1024)

// ResolveMaxTotalBytes returns the configured total-size cap for managed
// request-log files. Invalid values fall back to DefaultMaxTotalBytes so a
// typo does not silently disable disk protection.
func ResolveMaxTotalBytes() int64 {
	value := strings.TrimSpace(os.Getenv(EnvMaxTotalMB))
	if value == "" || value == "0" {
		return DefaultMaxTotalBytes
	}
	mb, err := strconv.ParseInt(value, 10, 64)
	if err != nil || mb == 0 {
		return DefaultMaxTotalBytes
	}
	if mb < 0 {
		return -1
	}
	if mb > maxTotalMiB {
		return DefaultMaxTotalBytes
	}
	return mb * 1024 * 1024
}

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
	// MaxTotalBytes caps the total size of managed request-/error-* files.
	// Zero falls back to DefaultMaxTotalBytes; a negative value disables
	// size-based eviction.
	MaxTotalBytes int64
	// Interval is how often the goroutine runs. Zero falls back to
	// DefaultCleanupInterval.
	Interval time.Duration
	// Now is an optional clock override for tests.
	Now func() time.Time
	// Logger receives best-effort background sweep diagnostics. SweepOnce still
	// returns errors to direct callers; Start uses this logger for async runs.
	Logger *slog.Logger
}

// Cleaner is the retention sweep implementation.
type Cleaner struct {
	cfg     CleanerConfig
	logger  *slog.Logger
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
	if cfg.MaxTotalBytes == 0 {
		cfg.MaxTotalBytes = DefaultMaxTotalBytes
	}
	if cfg.Interval == 0 {
		cfg.Interval = DefaultCleanupInterval
	}
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Cleaner{cfg: cfg, logger: logger}
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
		c.sweepAndLog(ctx)
		ticker := time.NewTicker(c.cfg.Interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.sweepAndLog(ctx)
			}
		}
	}()
}

func (c *Cleaner) sweepAndLog(ctx context.Context) {
	deleted, err := c.SweepOnce(ctx)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			c.logger.Warn(
				"request log file retention sweep failed",
				"error", err,
				"log_dir", c.LogDir(),
			)
		}
		return
	}
	if deleted > 0 {
		c.logger.Info(
			"request log file retention sweep completed",
			"deleted", deleted,
			"log_dir", c.LogDir(),
		)
	}
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
		size    int64
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
			size:    info.Size(),
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
			removed := make(map[string]struct{}, surplus)
			for i := 0; i < surplus; i++ {
				if err := os.Remove(errorFiles[i].path); err == nil {
					removed[errorFiles[i].path] = struct{}{}
					deleted++
				}
			}
			if len(removed) > 0 {
				kept := files[:0]
				for _, f := range files {
					if _, ok := removed[f.path]; ok {
						continue
					}
					kept = append(kept, f)
				}
				files = kept
			}
		}
	}

	// Stage 3: total managed-directory size cap. This is intentionally last:
	// age and error-count policies express stronger retention intent, then
	// the size cap protects disk by deleting the oldest remaining managed
	// captures across request-* and error-* files.
	if c.cfg.MaxTotalBytes > 0 {
		sort.Slice(files, func(i, j int) bool {
			return files[i].modTime.Before(files[j].modTime)
		})
		total := int64(0)
		for _, f := range files {
			total += f.size
		}
		for _, f := range files {
			if total <= c.cfg.MaxTotalBytes {
				break
			}
			if err := os.Remove(f.path); err == nil {
				total -= f.size
				deleted++
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
