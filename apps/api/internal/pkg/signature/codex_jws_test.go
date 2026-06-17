package signature

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func makeCodexJWS(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT","kid":"abc"}`))
	body, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	payload := base64.RawURLEncoding.EncodeToString(body)
	sig := base64.RawURLEncoding.EncodeToString([]byte("not-verified"))
	return fmt.Sprintf("%s.%s.%s", header, payload, sig)
}

func TestParseCodexJWT_RoundTrip(t *testing.T) {
	tok := makeCodexJWS(t, map[string]any{
		"email": "alice@example.com",
		"iss":   "https://auth.openai.com",
		"sub":   "user_123",
		"aud":   []string{"https://api.openai.com/v1"},
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	claims, err := ParseCodexJWT(tok)
	if err != nil {
		t.Fatalf("ParseCodexJWT: %v", err)
	}
	if claims.GetUserEmail() != "alice@example.com" {
		t.Fatalf("email mismatch: %q", claims.Email)
	}
	if claims.Iss != "https://auth.openai.com" {
		t.Fatalf("iss mismatch: %q", claims.Iss)
	}
}

func TestParseCodexJWT_BadFormat(t *testing.T) {
	if _, err := ParseCodexJWT("not.a"); err == nil {
		t.Fatal("expected error on bad JWT format")
	}
}

func TestValidateCodexResponseJWS_LenientNoToken(t *testing.T) {
	res, err := ValidateCodexResponseJWS([]byte(`{"hello":"world"}`), nil)
	if err != nil {
		t.Fatalf("lenient mode should not error on missing token: %v", err)
	}
	if res.Present {
		t.Fatal("Present should be false when no token")
	}
	if !res.Valid {
		t.Fatal("Valid should be true in lenient + no token")
	}
}

func TestValidateCodexResponseJWS_ValidToken(t *testing.T) {
	tok := makeCodexJWS(t, map[string]any{
		"iss": "https://auth.openai.com",
		"aud": []string{"https://api.openai.com/v1"},
		"exp": time.Now().Add(time.Hour).Unix(),
		"sub": "user_xyz",
	})
	body, _ := json.Marshal(map[string]any{"token": tok})
	res, err := ValidateCodexResponseJWS(body, nil)
	if err != nil {
		t.Fatalf("valid token should pass: %v (reason=%s)", err, res.Reason)
	}
	if !res.Present || !res.Valid {
		t.Fatalf("expected present+valid, got %+v", res)
	}
}

func TestValidateCodexResponseJWS_TwoSegmentTokenIgnored(t *testing.T) {
	body := `{"token":"two.segments"}`
	res, err := ValidateCodexResponseJWS([]byte(body), nil)
	// The token doesn't pass looksLikeJWS (only 2 segments) so it
	// is treated as missing → lenient → valid. This documents the
	// behaviour: only well-formed JWS triplets are subjected to
	// claim validation.
	if err != nil {
		t.Fatalf("two-segment token should be ignored in lenient mode: %v", err)
	}
	if res.Present {
		t.Fatal("two-segment token should not be flagged as present")
	}
}

func TestValidateCodexResponseJWS_ForgedTamperedClaims(t *testing.T) {
	// Three segments but middle is not valid base64-encoded JSON.
	body := `{"token":"aGVhZGVy.%%%.signature"}`
	res, err := ValidateCodexResponseJWS([]byte(body), nil)
	if err == nil {
		t.Fatal("tampered/non-JSON claims segment must fail parsing")
	}
	if res.Valid {
		t.Fatalf("expected invalid, got %+v", res)
	}
	if !res.Present {
		t.Fatal("Present should be true once the token has 3 segments")
	}
}

func TestValidateCodexResponseJWS_ForgedExpired(t *testing.T) {
	tok := makeCodexJWS(t, map[string]any{
		"iss": "https://auth.openai.com",
		"aud": []string{"https://api.openai.com/v1"},
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	body, _ := json.Marshal(map[string]any{"token": tok})
	res, err := ValidateCodexResponseJWS(body, nil)
	if err == nil {
		t.Fatal("expired token should fail validation")
	}
	if res.Valid {
		t.Fatalf("expected invalid, got %+v", res)
	}
}

func TestValidateCodexResponseJWS_WrongIssuer(t *testing.T) {
	tok := makeCodexJWS(t, map[string]any{
		"iss": "https://attacker.example.com",
		"aud": []string{"https://api.openai.com/v1"},
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	body, _ := json.Marshal(map[string]any{"token": tok})
	res, err := ValidateCodexResponseJWS(body, nil)
	if err == nil {
		t.Fatal("wrong issuer should fail")
	}
	if res.Valid {
		t.Fatal("expected invalid")
	}
}

func TestValidateCodexResponseJWS_WrongAudience(t *testing.T) {
	tok := makeCodexJWS(t, map[string]any{
		"iss": "https://auth.openai.com",
		"aud": []string{"https://api.attacker.com"},
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	body, _ := json.Marshal(map[string]any{"token": tok})
	res, err := ValidateCodexResponseJWS(body, nil)
	if err == nil {
		t.Fatal("wrong audience should fail")
	}
	if res.Valid {
		t.Fatal("expected invalid")
	}
}

func TestValidateCodexResponseJWS_AuthEnvelope(t *testing.T) {
	tok := makeCodexJWS(t, map[string]any{
		"iss": "https://auth.openai.com",
		"aud": []string{"https://api.openai.com/v1"},
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	body, _ := json.Marshal(map[string]any{
		"auth": map[string]any{"id_token": tok},
	})
	res, err := ValidateCodexResponseJWS(body, nil)
	if err != nil || !res.Valid {
		t.Fatalf("auth.id_token should be picked up: err=%v res=%+v", err, res)
	}
}

func TestExtractCodexJWSToken_NonJSONIgnored(t *testing.T) {
	if extractCodexJWSToken([]byte("event: ping\ndata: {}\n\n")) != "" {
		t.Fatal("non-JSON envelope should yield no token")
	}
}
