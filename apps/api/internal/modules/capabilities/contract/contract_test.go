package contract

import "testing"

func TestNormalizeDescriptorAcceptsCanonicalKeysOnly(t *testing.T) {
	descriptor, err := NormalizeDescriptor(Descriptor{Key: KeyStreaming})
	if err != nil {
		t.Fatalf("normalize canonical descriptor: %v", err)
	}
	if descriptor.Key != KeyStreaming || descriptor.Version != "v1" || descriptor.Level != DescriptorLevelRequired || descriptor.Status != DescriptorStatusStable {
		t.Fatalf("unexpected normalized descriptor: %+v", descriptor)
	}

	if _, err := NormalizeDescriptor(Descriptor{Key: "supports_stream"}); err == nil {
		t.Fatal("expected legacy convenience key to be rejected as descriptor source of truth")
	}
	if _, err := NormalizeDescriptor(Descriptor{Key: "streamng"}); err == nil {
		t.Fatal("expected misspelled capability key to be rejected")
	}
}

func TestCanonicalKeyFromConvenienceMapsDTOKeys(t *testing.T) {
	got, ok := CanonicalKeyFromConvenience("supports_tools")
	if !ok || got != KeyToolCalling {
		t.Fatalf("expected supports_tools to map to %s, got %q ok=%v", KeyToolCalling, got, ok)
	}
	got, ok = CanonicalKeyFromConvenience(KeyStructuredOutput)
	if !ok || got != KeyStructuredOutput {
		t.Fatalf("expected canonical key passthrough, got %q ok=%v", got, ok)
	}
}
