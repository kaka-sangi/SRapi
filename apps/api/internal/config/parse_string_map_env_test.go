package config

import (
	"testing"
)

func TestParseStringMapEnv(t *testing.T) {
	const key = "TEST_OAUTH_CLIENT_SECRETS_JSON"

	t.Setenv(key, "")
	if got := parseStringMapEnv(key); len(got) != 0 {
		t.Fatalf("unset = %v, want empty", got)
	}

	t.Setenv(key, `{"google-web":"s1","github":"s2"," ":"blank"}`)
	got := parseStringMapEnv(key)
	if got["google-web"] != "s1" || got["github"] != "s2" {
		t.Fatalf("parsed = %v, want google-web=s1 github=s2", got)
	}
	if _, ok := got[""]; ok {
		t.Fatalf("blank key should be dropped: %v", got)
	}

	t.Setenv(key, "not json")
	if got := parseStringMapEnv(key); len(got) != 0 {
		t.Fatalf("unparseable = %v, want empty (never nil/panic)", got)
	}
}
