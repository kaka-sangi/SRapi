package accountquotaalert

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
	admincontrolservice "github.com/srapi/srapi/apps/api/internal/modules/admin_control/service"
	admincontrolmemory "github.com/srapi/srapi/apps/api/internal/modules/admin_control/store/memory"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	eventsmemory "github.com/srapi/srapi/apps/api/internal/modules/events/store/memory"
	notificationscontract "github.com/srapi/srapi/apps/api/internal/modules/notifications/contract"
)

const testMasterKey = "account_quota_alert_master_key_min_32"

func TestRunOnceEnqueuesQuotaAlertOnRemainingRatioCrossing(t *testing.T) {
	accounts := accountmemory.New()
	account := mustCreateAccount(t, accounts)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	if _, err := accounts.RecordQuotaSnapshot(t.Context(), accountcontract.AccountQuotaSnapshot{
		AccountID:      account.ID,
		ProviderID:     account.ProviderID,
		QuotaType:      "codex_5h_percent",
		Remaining:      "30",
		Used:           "70",
		QuotaLimit:     "100",
		RemainingRatio: 0.30,
		SnapshotAt:     now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("record previous quota: %v", err)
	}
	latest, err := accounts.RecordQuotaSnapshot(t.Context(), accountcontract.AccountQuotaSnapshot{
		AccountID:      account.ID,
		ProviderID:     account.ProviderID,
		QuotaType:      "codex_5h_percent",
		Remaining:      "15",
		Used:           "85",
		QuotaLimit:     "100",
		RemainingRatio: 0.15,
		SnapshotAt:     now,
	})
	if err != nil {
		t.Fatalf("record latest quota: %v", err)
	}
	events := eventsmemory.New()
	worker, err := New(accounts, discardLogger(), Config{
		MasterKey: testMasterKey,
		Clock:     fixedClock{now: now.Add(time.Second)},
		Events:    events,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run worker once: %v", err)
	}
	if result.Selected != 1 || result.Checked != 1 || result.Enqueued != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	eventsSvc, err := eventsservice.New(events, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	outbox, err := eventsSvc.ListOutbox(t.Context())
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outbox) != 1 || outbox[0].EventType != notificationscontract.EventAccountQuotaAlertTriggered {
		t.Fatalf("expected one quota alert event, got %+v", outbox)
	}
	payload := outbox[0].Payload
	if payload["account_name"] != account.Name || payload["quota_snapshot_id"] != float64(latest.ID) || payload["quota_threshold"] != "0.20000000" {
		t.Fatalf("unexpected quota alert payload: %+v", payload)
	}
	payloadText := strings.ToLower(toString(payload))
	if strings.Contains(strings.ToLower(outbox[0].IdempotencyKey), "admin@") ||
		strings.Contains(payloadText, "unsubscribe") ||
		strings.Contains(payloadText, "smtp") ||
		strings.Contains(payloadText, "credential") ||
		strings.Contains(payloadText, "prompt") {
		t.Fatalf("quota alert event should not include recipient or credential material: %+v", outbox[0])
	}

	duplicate, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run duplicate worker once: %v", err)
	}
	if duplicate.Enqueued != 1 {
		t.Fatalf("idempotent duplicate run should observe the same crossing, got %+v", duplicate)
	}
	outbox, err = eventsSvc.ListOutbox(t.Context())
	if err != nil {
		t.Fatalf("list outbox after duplicate: %v", err)
	}
	if len(outbox) != 1 {
		t.Fatalf("expected idempotent duplicate enqueue, got %+v", outbox)
	}
}

func TestRunOnceIgnoresSyntheticQuotaSnapshots(t *testing.T) {
	accounts := accountmemory.New()
	account := mustCreateAccount(t, accounts)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	for _, snapshot := range []accountcontract.AccountQuotaSnapshot{
		{
			QuotaType:      accountcontract.QuotaTypeSyntheticMonthlyTokens,
			Remaining:      "unlimited",
			Used:           "1",
			QuotaLimit:     "unlimited",
			RemainingRatio: 1,
			SnapshotAt:     now,
		},
		{
			QuotaType:      "codex_5h_percent",
			Remaining:      "30",
			Used:           "70",
			QuotaLimit:     "100",
			RemainingRatio: 0.30,
			SnapshotAt:     now.Add(-time.Minute),
		},
		{
			QuotaType:      "codex_5h_percent",
			Remaining:      "15",
			Used:           "85",
			QuotaLimit:     "100",
			RemainingRatio: 0.15,
			SnapshotAt:     now.Add(-30 * time.Second),
		},
	} {
		snapshot.AccountID = account.ID
		snapshot.ProviderID = account.ProviderID
		if _, err := accounts.RecordQuotaSnapshot(t.Context(), snapshot); err != nil {
			t.Fatalf("record quota: %v", err)
		}
	}
	events := eventsmemory.New()
	worker, err := New(accounts, discardLogger(), Config{
		MasterKey: testMasterKey,
		Clock:     fixedClock{now: now.Add(time.Second)},
		Events:    events,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run worker once: %v", err)
	}
	if result.Checked != 1 || result.Enqueued != 1 {
		t.Fatalf("expected synthetic quota to be ignored and real crossing to alert, got %+v", result)
	}
}

func TestRunOnceSkipsQuotaAlertWhenDisabled(t *testing.T) {
	accounts := accountmemory.New()
	account := mustCreateAccount(t, accounts)
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	for _, snapshot := range []accountcontract.AccountQuotaSnapshot{
		{Remaining: "30", Used: "70", RemainingRatio: 0.30, SnapshotAt: now.Add(-time.Minute)},
		{Remaining: "15", Used: "85", RemainingRatio: 0.15, SnapshotAt: now},
	} {
		snapshot.AccountID = account.ID
		snapshot.ProviderID = account.ProviderID
		snapshot.QuotaType = "codex_7d_percent"
		snapshot.QuotaLimit = "100"
		if _, err := accounts.RecordQuotaSnapshot(t.Context(), snapshot); err != nil {
			t.Fatalf("record quota: %v", err)
		}
	}
	adminStore := admincontrolmemory.New()
	adminSvc, err := admincontrolservice.New(adminStore, nil)
	if err != nil {
		t.Fatalf("new admin service: %v", err)
	}
	settings, err := adminSvc.GetAdminSettings(t.Context())
	if err != nil {
		t.Fatalf("get settings: %v", err)
	}
	disabled := false
	settings.Email.AccountQuotaNotifyEnabled = &disabled
	if _, err := adminSvc.UpdateAdminSettings(t.Context(), settings, 1); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	events := eventsmemory.New()
	worker, err := New(accounts, discardLogger(), Config{
		MasterKey:    testMasterKey,
		Clock:        fixedClock{now: now},
		Events:       events,
		AdminControl: adminStore,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run worker once: %v", err)
	}
	if result.Enqueued != 0 {
		t.Fatalf("disabled quota alerts should not enqueue, got %+v", result)
	}
	eventsSvc, err := eventsservice.New(events, fixedClock{now: now})
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	outbox, err := eventsSvc.ListOutbox(t.Context())
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outbox) != 0 {
		t.Fatalf("expected no quota alert events, got %+v", outbox)
	}
}

func mustCreateAccount(t *testing.T, store *accountmemory.Store) accountcontract.ProviderAccount {
	t.Helper()
	account, err := store.Create(context.Background(), accountcontract.CreateStoredAccount{
		ProviderID:           7,
		Name:                 "codex-primary",
		RuntimeClass:         accountcontract.RuntimeClassCliClientToken,
		CredentialCiphertext: "ciphertext",
		CredentialVersion:    "v1",
		Status:               accountcontract.StatusActive,
		Weight:               1,
		Metadata:             map[string]any{},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	return account
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

func toString(value any) string {
	return fmt.Sprint(value)
}
