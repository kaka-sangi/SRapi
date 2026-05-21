package redis

import (
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
)

func TestOpenStoresConfiguredAddressAndDB(t *testing.T) {
	client, err := Open(config.DependencyConfig{
		Host:     "localhost",
		Port:     6379,
		Password: "",
		Database: "2",
	})
	if err != nil {
		t.Fatalf("open redis client: %v", err)
	}
	if got := client.Address(); got != "localhost:6379" {
		t.Fatalf("unexpected redis address: %s", got)
	}
	if got := client.Database(); got != 2 {
		t.Fatalf("unexpected redis db: %d", got)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close redis client: %v", err)
	}
}
