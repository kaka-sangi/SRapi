package crypto

import "testing"

func TestDeriveAESKeyRejectsWeakKey(t *testing.T) {
	if _, err := DeriveAESKey("short"); err == nil {
		t.Fatal("expected weak key rejection")
	}
}

func TestDeriveAESKeyIsDeterministic(t *testing.T) {
	keyA, err := DeriveAESKey("master_key_release_value_32_bytes_min")
	if err != nil {
		t.Fatalf("derive key: %v", err)
	}
	keyB, err := DeriveAESKey("master_key_release_value_32_bytes_min")
	if err != nil {
		t.Fatalf("derive key second time: %v", err)
	}
	if len(keyA) != 32 || len(keyB) != 32 {
		t.Fatalf("expected 32-byte keys, got %d and %d", len(keyA), len(keyB))
	}
	for i := range keyA {
		if keyA[i] != keyB[i] {
			t.Fatal("expected deterministic key derivation")
		}
	}
}
