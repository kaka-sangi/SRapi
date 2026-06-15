package httpserver

import (
	"net/http"
	"testing"
)

func TestCodexRequestSettingsExtractsAcceptLanguage(t *testing.T) {
	headers := http.Header{}
	headers.Set("Accept-Language", "es-ES,es;q=0.9")

	settings := codexRequestSettings(headers, nil)
	if settings == nil {
		t.Fatal("expected non-nil settings")
	}
	if got, _ := settings["accept-language"].(string); got != "es-ES,es;q=0.9" {
		t.Fatalf("expected accept-language setting to be extracted, got %v", settings["accept-language"])
	}
}

func TestCodexRequestSettingsOmitsAcceptLanguageWhenAbsent(t *testing.T) {
	settings := codexRequestSettings(http.Header{}, nil)
	if _, ok := settings["accept-language"]; ok {
		t.Fatalf("expected accept-language setting to be absent, got %v", settings["accept-language"])
	}
}
