package service

import (
	"bufio"
	"io"
	"sync"
)

// sseScanBufSize is the initial capacity of pooled SSE scan buffers. 64 KB
// matches the allocation the hot-path scanners already used; pooling avoids
// re-allocating (and subsequently garbage-collecting) one per stream.
const sseScanBufSize = 64 * 1024

// sseScanBufPool reuses 64 KB byte-slice buffers for bufio.Scanner initial
// allocations, reducing GC pressure under high-throughput streaming. Ported
// from sub2api's buffer-pool pattern.
//
// Each pool entry is a *[]byte so the slice header (pointer+len+cap) is
// stable across Get/Put and the scanner cannot silently discard our reference
// by growing the slice.
var sseScanBufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, sseScanBufSize)
		return &buf
	},
}

// acquireSSEScanner creates a bufio.Scanner over r using a pooled 64 KB scan
// buffer and the given maxTokenSize. The caller MUST call the returned release
// function after the scanner is no longer in use to return the buffer to the
// pool.
//
// Note: if the scanner internally grows beyond 64 KB (because a single SSE
// line exceeds that), the pooled buffer is still returned at its original
// capacity — subsequent users get the same 64 KB head-start without a fresh
// heap allocation. The scanner struct itself is not pooled (it is a small
// value type); the expensive part is the backing byte array.
func acquireSSEScanner(r io.Reader, maxTokenSize int) (scanner *bufio.Scanner, release func()) {
	bp := sseScanBufPool.Get().(*[]byte)
	buf := (*bp)[:0] // reset length, keep capacity
	scanner = bufio.NewScanner(r)
	scanner.Buffer(buf, maxTokenSize)
	return scanner, func() {
		// Reset to zero-length but keep the original capacity so the next user
		// gets a pre-allocated 64 KB buffer.
		*bp = (*bp)[:0]
		sseScanBufPool.Put(bp)
	}
}
