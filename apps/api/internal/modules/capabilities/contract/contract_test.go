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
	got, ok = CanonicalKeyFromConvenience("supports_responses_compact")
	if !ok || got != KeyResponsesCompact {
		t.Fatalf("expected supports_responses_compact to map to %s, got %q ok=%v", KeyResponsesCompact, got, ok)
	}
	got, ok = CanonicalKeyFromConvenience("supports_responses_websocket")
	if !ok || got != KeyResponsesWebSocket {
		t.Fatalf("expected supports_responses_websocket to map to %s, got %q ok=%v", KeyResponsesWebSocket, got, ok)
	}
	got, ok = CanonicalKeyFromConvenience("web_search_preview")
	if !ok || got != KeyWebSearch {
		t.Fatalf("expected web_search_preview to map to %s, got %q ok=%v", KeyWebSearch, got, ok)
	}
	got, ok = CanonicalKeyFromConvenience(KeyStructuredOutput)
	if !ok || got != KeyStructuredOutput {
		t.Fatalf("expected canonical key passthrough, got %q ok=%v", got, ok)
	}
}

func TestDefaultDefinitionsIncludeWebSearch(t *testing.T) {
	for _, def := range DefaultDefinitions() {
		if def.Key == KeyWebSearch {
			if def.Version != "v1" || def.Category != "interaction" || def.Status != DefinitionStatusStable {
				t.Fatalf("unexpected web search definition: %+v", def)
			}
			return
		}
	}
	t.Fatalf("expected default definitions to include %s", KeyWebSearch)
}

func TestDefaultDefinitionsIncludeResponsesCompact(t *testing.T) {
	for _, def := range DefaultDefinitions() {
		if def.Key == KeyResponsesCompact {
			if def.Version != "v1" || def.Category != "endpoint" || def.Status != DefinitionStatusExperimental {
				t.Fatalf("unexpected responses compact definition: %+v", def)
			}
			return
		}
	}
	t.Fatalf("expected default definitions to include %s", KeyResponsesCompact)
}

func TestDefaultDefinitionsIncludeResponsesWebSocket(t *testing.T) {
	for _, def := range DefaultDefinitions() {
		if def.Key == KeyResponsesWebSocket {
			if def.Version != "v1" || def.Category != "endpoint" || def.Status != DefinitionStatusExperimental {
				t.Fatalf("unexpected responses websocket definition: %+v", def)
			}
			return
		}
	}
	t.Fatalf("expected default definitions to include %s", KeyResponsesWebSocket)
}
