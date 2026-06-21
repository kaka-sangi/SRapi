package vertex

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
)

func mustGeneratePKCS8(t *testing.T) ([]byte, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal pkcs8: %v", err)
	}
	pkcs1 := x509.MarshalPKCS1PrivateKey(key)
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}),
		pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: pkcs1})
}

func TestNormalize_AcceptsPKCS8AndRewritesToPKCS1(t *testing.T) {
	pkcs8PEM, _ := mustGeneratePKCS8(t)
	raw, err := json.Marshal(map[string]any{
		"type":        "service_account",
		"project_id":  "demo-project",
		"private_key": string(pkcs8PEM),
		"client_email": "demo@demo-project.iam.gserviceaccount.com",
	})
	if err != nil {
		t.Fatalf("marshal raw: %v", err)
	}
	out, err := NormalizeServiceAccountJSON(raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	key, _ := parsed["private_key"].(string)
	if !strings.Contains(key, "BEGIN RSA PRIVATE KEY") {
		t.Fatalf("expected pkcs1 RSA PRIVATE KEY in output, got: %s", key)
	}
	if parsed["project_id"] != "demo-project" {
		t.Fatalf("project_id was lost during normalize: %v", parsed["project_id"])
	}
}

func TestNormalize_RejectsMissingPrivateKey(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"project_id": "x"})
	if _, err := NormalizeServiceAccountJSON(raw); err == nil {
		t.Fatal("expected error for missing private_key")
	}
}

func TestNormalize_RejectsGarbagePEM(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"private_key": "this is not a pem",
		"project_id":  "x",
	})
	if _, err := NormalizeServiceAccountJSON(raw); err == nil {
		t.Fatal("expected error for non-PEM private_key")
	}
}

func TestNormalize_RecoversFromCRLFAndAnsiEscape(t *testing.T) {
	pkcs8PEM, _ := mustGeneratePKCS8(t)
	dirty := strings.ReplaceAll(string(pkcs8PEM), "\n", "\r\n")
	// Splice a fake ANSI escape that an operator might have pasted in
	// from a terminal capture into the middle of the PEM.
	dirty = strings.Replace(dirty, "BEGIN", "\x1b[0mBEGIN", 1)
	raw, _ := json.Marshal(map[string]any{
		"private_key": dirty,
		"project_id":  "x",
	})
	out, err := NormalizeServiceAccountJSON(raw)
	if err != nil {
		t.Fatalf("normalize should recover from CRLF/ANSI noise: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if !strings.Contains(parsed["private_key"].(string), "BEGIN RSA PRIVATE KEY") {
		t.Fatalf("recovered output did not produce RSA PRIVATE KEY: %v", parsed["private_key"])
	}
}
