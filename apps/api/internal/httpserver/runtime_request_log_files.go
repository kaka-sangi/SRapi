package httpserver

import (
	"context"
	"sync"

	rlfcontract "github.com/srapi/srapi/apps/api/internal/modules/request_log_files/contract"
	rlfservice "github.com/srapi/srapi/apps/api/internal/modules/request_log_files/service"
)

// requestLogFilesState holds the (optional) per-request file capture wiring.
// It lives separately from runtimeState so the much-larger struct doesn't
// have to grow another set of fields, and so a future runtime that disables
// the capture entirely can avoid the small allocation cost.
//
// The fields are lazily populated by ensureRequestLogFilesState — they are
// safe to access through the runtimeState accessors below, which return nil
// when capture is disabled.
type requestLogFilesState struct {
	writer  *rlfservice.FileWriter
	reader  *rlfservice.FileReader
	cleaner *rlfservice.Cleaner
}

// ensureRequestLogFilesState lazily constructs the file-capture trio. The
// writer is gated by ResolveEnabled (which reads SRAPI_REQUEST_LOG_ENABLED);
// the reader + cleaner are always constructed because admin endpoints want
// to inspect any pre-existing files even when capture is turned off mid-run.
func (rt *runtimeState) ensureRequestLogFilesState() *requestLogFilesState {
	if rt == nil {
		return nil
	}
	rt.requestLogFilesMu.Lock()
	defer rt.requestLogFilesMu.Unlock()
	if rt.requestLogFiles != nil {
		return rt.requestLogFiles
	}
	enabled := rlfservice.ResolveEnabled(false)
	logDir := rlfservice.ResolveLogDir("")
	state := &requestLogFilesState{
		writer:  rlfservice.NewFileWriter(rlfservice.Config{Enabled: enabled, LogDir: logDir}),
		reader:  rlfservice.NewFileReader(logDir),
		cleaner: rlfservice.NewCleaner(rlfservice.CleanerConfig{LogDir: logDir}),
	}
	if enabled {
		// Start the background sweep so disk usage is bounded even when
		// the runtime stays up for weeks. Cancellation is bound to the
		// rare runtime-shutdown path — for now we use context.Background
		// since the runtime itself does not currently surface a shutdown
		// ctx; the cleaner's running flag prevents accidental doubles.
		state.cleaner.Start(context.Background())
	}
	rt.requestLogFiles = state
	return state
}

// requestLogFileWriter returns the file-capture writer. When capture is
// disabled the returned writer is a no-op (Begin yields an empty Handle and
// every other method returns nil), so the gateway hot path can always call
// it without branching.
func (rt *runtimeState) requestLogFileWriter() rlfcontract.Writer {
	state := rt.ensureRequestLogFilesState()
	if state == nil {
		return nil
	}
	return state.writer
}

// requestLogFileReader returns the file-capture reader for the admin
// endpoints. Always non-nil; when the directory is empty / missing the
// reader's List returns an empty slice.
func (rt *runtimeState) requestLogFileReader() rlfcontract.Reader {
	state := rt.ensureRequestLogFilesState()
	if state == nil {
		return nil
	}
	return state.reader
}

// requestLogFileCleaner exposes the retention sweeper. Returned mainly for
// tests; the runtime starts the background goroutine itself.
func (rt *runtimeState) requestLogFileCleaner() rlfcontract.Cleaner {
	state := rt.ensureRequestLogFilesState()
	if state == nil {
		return nil
	}
	return state.cleaner
}

// requestLogFilesMu / requestLogFiles back ensureRequestLogFilesState. The
// fields live on runtimeState (defined in runtime_state.go) so we declare
// them here as a separate, narrowly-scoped attachment.
//
// (Go does not support adding fields to a struct from a different file in
// the same package via syntax, so the actual field declaration is colocated
// with the rest of runtimeState in runtime_state.go.)
var _ = sync.Mutex{}
