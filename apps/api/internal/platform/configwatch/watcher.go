package configwatch

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"
)

type Config struct {
	Path     string
	Interval time.Duration
	Debounce time.Duration // coalesce rapid changes; 0 means fire immediately
	Logger   *slog.Logger
}

func (c *Config) defaults() {
	if c.Interval <= 0 {
		c.Interval = 5 * time.Second
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
}

type fileState struct {
	modTime time.Time
	size    int64
}

type Watcher struct {
	cfg      Config
	onChange func(path string)

	mu             sync.Mutex
	last           fileState
	cancel         context.CancelFunc
	stopped        chan struct{}
	debounceTimer  *time.Timer
}

// New creates a Watcher that polls the file at cfg.Path every cfg.Interval.
// When a change in modification time or size is detected, onChange is invoked
// with the file path. The file must exist at creation time.
func New(cfg Config, onChange func(path string)) (*Watcher, error) {
	cfg.defaults()

	info, err := os.Stat(cfg.Path)
	if err != nil {
		return nil, err
	}

	return &Watcher{
		cfg:      cfg,
		onChange: onChange,
		last: fileState{
			modTime: info.ModTime(),
			size:    info.Size(),
		},
		stopped: make(chan struct{}),
	}, nil
}

// Start polls the configured file until ctx is cancelled or Stop is called.
// It blocks until shutdown is complete.
func (w *Watcher) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)

	w.mu.Lock()
	w.cancel = cancel
	w.mu.Unlock()

	defer func() {
		cancel()
		close(w.stopped)
	}()

	ticker := time.NewTicker(w.cfg.Interval)
	defer ticker.Stop()

	w.cfg.Logger.Info("config watcher started",
		slog.String("path", w.cfg.Path),
		slog.Duration("interval", w.cfg.Interval),
	)

	for {
		select {
		case <-ctx.Done():
			w.cfg.Logger.Info("config watcher stopped", slog.String("path", w.cfg.Path))
			return
		case <-ticker.C:
			w.poll()
		}
	}
}

// Stop signals the watcher to shut down and waits for it to finish.
func (w *Watcher) Stop() {
	w.mu.Lock()
	cancel := w.cancel
	w.mu.Unlock()

	if cancel != nil {
		cancel()
		<-w.stopped
	}
}

func (w *Watcher) poll() {
	info, err := os.Stat(w.cfg.Path)
	if err != nil {
		w.cfg.Logger.Error("failed to stat config file",
			slog.String("path", w.cfg.Path),
			slog.String("error", err.Error()),
		)
		return
	}

	current := fileState{
		modTime: info.ModTime(),
		size:    info.Size(),
	}

	w.mu.Lock()
	changed := current != w.last
	if changed {
		w.last = current
	}
	w.mu.Unlock()

	if changed {
		w.cfg.Logger.Info("config file changed",
			slog.String("path", w.cfg.Path),
			slog.Time("mod_time", current.modTime),
			slog.Int64("size", current.size),
		)
		if w.cfg.Debounce > 0 {
			w.mu.Lock()
			if w.debounceTimer != nil {
				w.debounceTimer.Stop()
			}
			w.debounceTimer = time.AfterFunc(w.cfg.Debounce, func() {
				w.onChange(w.cfg.Path)
			})
			w.mu.Unlock()
		} else {
			w.onChange(w.cfg.Path)
		}
	}
}
