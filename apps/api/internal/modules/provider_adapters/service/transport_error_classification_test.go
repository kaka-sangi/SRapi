package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"testing"
)

// TestClassifyTransportErrorPersistence verifies that classifyTransportError tags
// durable transport faults (DNS not-found, refused connection, rejected proxy
// credentials) with the persistent metadata marker so the gateway cooldown path
// parks the account, while transient faults (context cancel/deadline, a generic
// blip) keep the plain "network_error" class with no marker and stay schedulable.
// All inputs are synthetic. Mirrors sub2api classifyOpenAITransportError.
func TestClassifyTransportErrorPersistence(t *testing.T) {
	cases := []struct {
		name           string
		err            error
		wantPersistent bool
	}{
		{
			name:           "no such host (string marker) is persistent",
			err:            errors.New(`dial tcp: lookup bad.example.invalid: no such host`),
			wantPersistent: true,
		},
		{
			name:           "typed DNS not-found is persistent",
			err:            &net.DNSError{Err: "no such host", Name: "bad.example.invalid", IsNotFound: true},
			wantPersistent: true,
		},
		{
			name:           "typed connection refused is persistent",
			err:            fmt.Errorf("dial tcp 10.0.0.1:443: connect: %w", syscall.ECONNREFUSED),
			wantPersistent: true,
		},
		{
			name:           "socks5 credential rejection is persistent",
			err:            errors.New("socks connect tcp: username/password authentication failed"),
			wantPersistent: true,
		},
		{
			name:           "context canceled (client gone) is transient",
			err:            context.Canceled,
			wantPersistent: false,
		},
		{
			name:           "context deadline exceeded is transient",
			err:            context.DeadlineExceeded,
			wantPersistent: false,
		},
		{
			name:           "typed DNS timeout (not not-found) is transient",
			err:            &net.DNSError{Err: "i/o timeout", Name: "ok.example.com", IsTimeout: true},
			wantPersistent: false,
		},
		{
			name:           "generic transport blip is transient",
			err:            errors.New("unexpected EOF"),
			wantPersistent: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provErr := classifyTransportError(tc.err)

			// Class and status stay stable across persistent/transient so the
			// gateway's existing network_error cooldown path always applies.
			if provErr.Class != "network_error" {
				t.Fatalf("Class = %q, want %q", provErr.Class, "network_error")
			}
			if provErr.StatusCode != http.StatusBadGateway {
				t.Fatalf("StatusCode = %d, want %d", provErr.StatusCode, http.StatusBadGateway)
			}

			_, marked := provErr.Metadata[transportErrorPersistentMetadataKey]
			if marked != tc.wantPersistent {
				t.Fatalf("persistent marker = %v, want %v (metadata=%v)", marked, tc.wantPersistent, provErr.Metadata)
			}
			if tc.wantPersistent {
				if got := provErr.Metadata[transportErrorPersistentMetadataKey]; got != true {
					t.Fatalf("persistent marker value = %v, want true", got)
				}
			} else if provErr.Metadata != nil {
				t.Fatalf("transient error must carry no metadata, got %v", provErr.Metadata)
			}
		})
	}
}
