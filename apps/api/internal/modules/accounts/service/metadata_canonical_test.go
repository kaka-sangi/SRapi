package service

import (
	"context"
	"reflect"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
)

func TestCanonicalizeAccountMetadataAliasRewrite(t *testing.T) {
	cases := []struct {
		name string
		in   map[string]any
		want map[string]any
	}{
		{
			name: "all aliases rewritten",
			in: map[string]any{
				"codex_email":           "a@example.com",
				"codex_plan_type":       "pro",
				"codex_organization_id": "org-1",
				"codex_account_id":      "acct-1",
				"codex_user_id":         "user-1",
				"rpm_override":          120,
			},
			want: map[string]any{
				"email":               "a@example.com",
				"plan_type":           "pro",
				"organization_id":     "org-1",
				"upstream_account_id": "acct-1",
				"upstream_user_id":    "user-1",
				"rpm_limit":           120,
			},
		},
		{
			name: "chatgpt aliases rewritten",
			in: map[string]any{
				"chatgpt_account_id": "ws-1",
				"chatgpt_user_id":    "u-1",
			},
			want: map[string]any{
				"upstream_account_id": "ws-1",
				"upstream_user_id":    "u-1",
			},
		},
		{
			name: "canonical wins when both set",
			in: map[string]any{
				"email":       "canonical@example.com",
				"codex_email": "alias@example.com",
			},
			want: map[string]any{
				"email": "canonical@example.com",
			},
		},
		{
			name: "non-alias keys passthrough",
			in: map[string]any{
				"base_url":      "https://example",
				"manual_pause":  true,
				"random_string": "x",
			},
			want: map[string]any{
				"base_url":      "https://example",
				"manual_pause":  true,
				"random_string": "x",
			},
		},
		{
			name: "empty-string alias is dropped",
			in: map[string]any{
				"codex_email": "  ",
				"email":       "",
			},
			want: map[string]any{
				"email": "",
			},
		},
		{
			name: "nil and empty input pass through",
			in:   nil,
			want: nil,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := CanonicalizeAccountMetadata(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v want %#v", got, tc.want)
			}
		})
	}
}

func TestServiceCreateCanonicalizesMetadata(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	created, err := svc.Create(context.Background(), contract.CreateRequest{
		ProviderID:   1,
		Name:         "alias-write",
		RuntimeClass: contract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "k"},
		Metadata: map[string]any{
			"codex_email":        "ops@example.com",
			"chatgpt_account_id": "ws-1",
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, has := created.Metadata["codex_email"]; has {
		t.Error("alias codex_email leaked into stored metadata")
	}
	if _, has := created.Metadata["chatgpt_account_id"]; has {
		t.Error("alias chatgpt_account_id leaked into stored metadata")
	}
	if created.Metadata["email"] != "ops@example.com" {
		t.Errorf("canonical email not set: %v", created.Metadata["email"])
	}
	if created.Metadata["upstream_account_id"] != "ws-1" {
		t.Errorf("canonical upstream_account_id not set: %v", created.Metadata["upstream_account_id"])
	}
}

func TestServiceUpdateCanonicalizesMetadata(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	created, err := svc.Create(ctx, contract.CreateRequest{
		ProviderID:   1,
		Name:         "u",
		RuntimeClass: contract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "k"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	patch := map[string]any{
		"codex_plan_type":       "pro",
		"codex_organization_id": "org-2",
		"rpm_override":          240,
	}
	updated, err := svc.Update(ctx, created.ID, contract.UpdateRequest{Metadata: &patch})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	for _, alias := range []string{"codex_plan_type", "codex_organization_id", "rpm_override"} {
		if _, has := updated.Metadata[alias]; has {
			t.Errorf("alias %s leaked into stored metadata after Update", alias)
		}
	}
	if updated.Metadata["plan_type"] != "pro" {
		t.Errorf("plan_type not canonical: %v", updated.Metadata["plan_type"])
	}
	if updated.Metadata["organization_id"] != "org-2" {
		t.Errorf("organization_id not canonical: %v", updated.Metadata["organization_id"])
	}
	// cloneMap round-trips via json so an int literal lands as float64. Read it
	// with a numeric-tolerant compare rather than a typed equality.
	if got := metadataInt(updated.Metadata, "rpm_limit"); got != 240 {
		t.Errorf("rpm_limit not canonical: %v", updated.Metadata["rpm_limit"])
	}
}
