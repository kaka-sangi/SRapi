package errorpassthrough

import (
	"testing"

	"entgo.io/ent/dialect"
	_ "github.com/mattn/go-sqlite3"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	"github.com/srapi/srapi/apps/api/internal/modules/error_passthrough/contract"
)

func TestStorePersistsResponseOverrides(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, "file:"+t.TempDir()+"/error-passthrough.db?_fk=1")
	defer client.Close()
	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := t.Context()
	status := 422
	rule, err := store.CreateRule(ctx, contract.CreateRule{
		Name:           "schema override",
		Enabled:        true,
		Action:         contract.ActionMask,
		StatusCodes:    []int{400},
		ResponseStatus: &status,
		CustomMessage:  "upstream rejected schema",
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if rule.ResponseStatus == nil || *rule.ResponseStatus != 422 {
		t.Fatalf("expected response status 422, got %+v", rule.ResponseStatus)
	}
	if rule.CustomMessage != "upstream rejected schema" {
		t.Fatalf("expected custom message, got %q", rule.CustomMessage)
	}

	var clearedStatus *int
	emptyMessage := ""
	updated, err := store.UpdateRule(ctx, rule.ID, contract.UpdateRule{
		ResponseStatus: &clearedStatus,
		CustomMessage:  &emptyMessage,
	})
	if err != nil {
		t.Fatalf("clear overrides: %v", err)
	}
	if updated.ResponseStatus != nil {
		t.Fatalf("expected response status to clear, got %+v", updated.ResponseStatus)
	}
	if updated.CustomMessage != "" {
		t.Fatalf("expected custom message to clear, got %q", updated.CustomMessage)
	}
}
