package translator

import (
	"context"
	"sync"
)

// TranslateRequestFunc is the signature for a request-side translator: it
// takes the source format's raw JSON body and returns the target format's
// raw JSON body. modelName carries the canonical model name in case the
// translator needs to swap model aliases or inject Provider-Account-specific
// fields. stream is true when the caller is opening a streaming response —
// some translators emit different envelope shapes for streaming bodies.
//
// Translators must not panic on malformed input — return the original
// rawJSON unchanged if a structural issue would otherwise break the
// registry contract. The inline transforms this layer replaces were
// already defensive in this way, and changing that contract here would
// be a behavioural regression.
type TranslateRequestFunc func(modelName string, rawJSON []byte, stream bool) []byte

// TranslateResponseFunc is the signature for a response-side translator
// (both stream and non-stream variants funnel through this). originalRaw is
// the inbound request as the CLIENT sent it (pre-request-translation);
// requestRaw is the body actually sent upstream (post-request-translation).
// rawJSON is the upstream response body chunk to translate back.
//
// param is an opaque per-stream state pointer for translators that need to
// carry forward decoder state across chunks (e.g. SSE buffering, JWT
// signature accumulation). nil-safe.
//
// Returns the translated chunks. Stream translators may emit zero, one, or
// many output chunks per input chunk; non-stream translators emit exactly
// one entry of length 1 (or zero on translation failure).
type TranslateResponseFunc func(
	ctx context.Context,
	modelName string,
	originalRaw []byte,
	requestRaw []byte,
	rawJSON []byte,
	param *any,
) [][]byte

// Translator is the registered unit per (from, to) pair. Either field may
// be nil — a pair that only does response-side translation registers
// Request=nil; a pair that does only request transformation leaves Response
// nil and the registry's NeedConvert returns false for it.
type Translator struct {
	Request  TranslateRequestFunc
	Response TranslateResponseFunc
}

// Registry holds the (from, to) → Translator map. Safe for concurrent
// Register + lookup; the read-write mutex favours readers since lookups on
// the hot path vastly outnumber registrations (which happen once at
// package init).
type Registry struct {
	mu           sync.RWMutex
	translators  map[Pair]Translator
}

// NewRegistry returns an empty registry with no translators registered.
// Default() returns the process-wide singleton — tests that need
// isolation can construct their own via NewRegistry().
func NewRegistry() *Registry {
	return &Registry{translators: make(map[Pair]Translator)}
}

// Register installs a translator for the (from, to) pair. Idempotent — a
// second Register call for the same pair overwrites the first, which is
// the same semantic CLIProxyAPI's registry uses. Calls with an invalid
// pair (either format empty) are silently ignored so misconfigured init
// blocks don't blow up the process at startup.
func (r *Registry) Register(from, to Format, request TranslateRequestFunc, response TranslateResponseFunc) {
	pair := Pair{From: from, To: to}
	if !pair.Valid() {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.translators[pair] = Translator{Request: request, Response: response}
}

// Lookup returns the translator for the pair and a found bool. Identity
// pairs (from==to) return zero + false here — the registry doesn't store
// no-op identity translators; callers should short-circuit identity
// before calling Lookup.
func (r *Registry) Lookup(from, to Format) (Translator, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.translators[Pair{From: from, To: to}]
	return t, ok
}

// HasResponseTransformer reports whether the (from, to) pair has a
// response-side translator. Used by the gateway to decide whether to
// buffer/passthrough an upstream stream — mirrors CLIProxyAPI's
// NeedConvert helper at translator/translator.go:51.
func (r *Registry) HasResponseTransformer(from, to Format) bool {
	if (Pair{From: from, To: to}).Identity() {
		return false
	}
	t, ok := r.Lookup(from, to)
	return ok && t.Response != nil
}

// TranslateRequest applies the request-side translator. If no translator
// is registered for the pair OR the pair is identity, returns rawJSON
// unchanged — the same fall-through CLIProxyAPI exhibits when a pair has
// no Request mapping. nil-safe on input.
func (r *Registry) TranslateRequest(from, to Format, modelName string, rawJSON []byte, stream bool) []byte {
	if (Pair{From: from, To: to}).Identity() || len(rawJSON) == 0 {
		return rawJSON
	}
	t, ok := r.Lookup(from, to)
	if !ok || t.Request == nil {
		return rawJSON
	}
	return t.Request(modelName, rawJSON, stream)
}

// TranslateResponseStream applies the response-side translator and returns
// every output chunk a streaming translator emits for the single input
// chunk. nil-safe; empty/identity/missing translators fall through to a
// single-element slice containing the input unchanged.
func (r *Registry) TranslateResponseStream(
	ctx context.Context,
	from, to Format,
	modelName string,
	originalRaw []byte,
	requestRaw []byte,
	rawJSON []byte,
	param *any,
) [][]byte {
	if (Pair{From: from, To: to}).Identity() {
		return [][]byte{rawJSON}
	}
	t, ok := r.Lookup(from, to)
	if !ok || t.Response == nil {
		return [][]byte{rawJSON}
	}
	return t.Response(ctx, modelName, originalRaw, requestRaw, rawJSON, param)
}

// TranslateResponseNonStream is the single-chunk convenience wrapper used
// by call sites that have a complete upstream body already buffered (no
// SSE/WS streaming). Returns the first emitted chunk (empty bytes if the
// translator returned zero output). Mirrors CLIProxyAPI's
// TranslateNonStream signature.
func (r *Registry) TranslateResponseNonStream(
	ctx context.Context,
	from, to Format,
	modelName string,
	originalRaw []byte,
	requestRaw []byte,
	rawJSON []byte,
	param *any,
) []byte {
	chunks := r.TranslateResponseStream(ctx, from, to, modelName, originalRaw, requestRaw, rawJSON, param)
	if len(chunks) == 0 {
		return nil
	}
	return chunks[0]
}

// defaultRegistry is the process-wide singleton. Translators register here
// from translators/ subpackages via their package init() blocks.
var (
	defaultRegistry     = NewRegistry()
	defaultRegistryOnce sync.Once
)

// Default returns the process-wide registry. Tests that need isolation
// should construct a NewRegistry() instead.
func Default() *Registry {
	defaultRegistryOnce.Do(func() {
		// Reserved for any setup that needs to happen exactly once at
		// first Default() access. Currently the constructor itself is
		// sufficient; the Once guards against future expansion making
		// init order-sensitive.
	})
	return defaultRegistry
}
