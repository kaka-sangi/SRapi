package config

import (
	"strings"
	"testing"
	"time"
)

func TestValidateAllowsLocalDevelopmentDefaults(t *testing.T) {
	cfg := Load()
	cfg.Server.Mode = "local"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected local defaults to validate, got %v", err)
	}
}

func TestValidateRejectsReleaseWeakSecrets(t *testing.T) {
	cfg := validReleaseConfig()
	cfg.Security.JWTSecret = "local_dev_jwt_secret_32_bytes_minimum_change_me"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("expected weak JWT_SECRET rejection, got %v", err)
	}

	cfg = validReleaseConfig()
	cfg.Security.MasterKey = "local_dev_master_key_32_bytes_minimum_change_me"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "SRAPI_MASTER_KEY") {
		t.Fatalf("expected weak master key rejection, got %v", err)
	}

	cfg = validReleaseConfig()
	cfg.Security.APIKeyPepper = "change_me_api_key_pepper_but_long_enough"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "API_KEY_PEPPER") {
		t.Fatalf("expected weak API key pepper rejection, got %v", err)
	}

	cfg = validReleaseConfig()
	cfg.Database.Password = "srapi_dev_password_change_me"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "DATABASE_PASSWORD") {
		t.Fatalf("expected weak database password rejection, got %v", err)
	}

	cfg = validReleaseConfig()
	cfg.Bootstrap.AdminPassword = "password123"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "BOOTSTRAP_ADMIN_PASSWORD") {
		t.Fatalf("expected default bootstrap admin password rejection, got %v", err)
	}
}

func TestValidateAcceptsReleaseStrongSecrets(t *testing.T) {
	cfg := validReleaseConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected release config to validate, got %v", err)
	}
}

func TestGatewayTimeoutDefaultsAndOverrides(t *testing.T) {
	t.Setenv("GATEWAY_MAX_BODY_SIZE", "")
	t.Setenv("GATEWAY_REQUEST_TIMEOUT_SECONDS", "")
	t.Setenv("GATEWAY_STREAM_IDLE_TIMEOUT_SECONDS", "")
	cfg := Load()
	if cfg.Gateway.MaxBodySize != 268435456 {
		t.Fatalf("expected default gateway max body size 268435456, got %d", cfg.Gateway.MaxBodySize)
	}
	if cfg.Gateway.RequestTimeout != 600*time.Second {
		t.Fatalf("expected default gateway request timeout 600s, got %s", cfg.Gateway.RequestTimeout)
	}
	if cfg.Gateway.StreamIdleTimeout != 120*time.Second {
		t.Fatalf("expected default gateway stream idle timeout 120s, got %s", cfg.Gateway.StreamIdleTimeout)
	}

	t.Setenv("GATEWAY_MAX_BODY_SIZE", "12345")
	t.Setenv("GATEWAY_REQUEST_TIMEOUT_SECONDS", "42")
	t.Setenv("GATEWAY_STREAM_IDLE_TIMEOUT_SECONDS", "7")
	cfg = Load()
	if cfg.Gateway.MaxBodySize != 12345 {
		t.Fatalf("expected overridden gateway max body size 12345, got %d", cfg.Gateway.MaxBodySize)
	}
	if cfg.Gateway.RequestTimeout != 42*time.Second {
		t.Fatalf("expected overridden gateway request timeout 42s, got %s", cfg.Gateway.RequestTimeout)
	}
	if cfg.Gateway.StreamIdleTimeout != 7*time.Second {
		t.Fatalf("expected overridden gateway stream idle timeout 7s, got %s", cfg.Gateway.StreamIdleTimeout)
	}
}

func TestHealthcheckAddressUsesLoopbackForWildcardHost(t *testing.T) {
	cfg := Load()
	cfg.Server.Host = "0.0.0.0"
	if got := cfg.HealthcheckAddress(); got != "127.0.0.1:8080" {
		t.Fatalf("expected loopback healthcheck address, got %s", got)
	}
}

func TestValidateRejectsInvalidGatewayTimeouts(t *testing.T) {
	cfg := Load()
	cfg.Gateway.RequestTimeout = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "GATEWAY_REQUEST_TIMEOUT_SECONDS") {
		t.Fatalf("expected gateway request timeout rejection, got %v", err)
	}

	cfg = Load()
	cfg.Gateway.StreamIdleTimeout = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "GATEWAY_STREAM_IDLE_TIMEOUT_SECONDS") {
		t.Fatalf("expected gateway stream timeout rejection, got %v", err)
	}
}

func TestRetentionDefaultsOverridesAndValidation(t *testing.T) {
	t.Setenv("DATA_RETENTION_USAGE_LOGS_DAYS", "")
	t.Setenv("DATA_RETENTION_SCHEDULER_DECISIONS_DAYS", "")
	t.Setenv("DATA_RETENTION_SCHEDULER_FEEDBACKS_DAYS", "")
	t.Setenv("DATA_RETENTION_AUDIT_LOGS_DAYS", "")
	t.Setenv("DATA_RETENTION_ACCOUNT_HEALTH_SNAPSHOTS_DAYS", "")
	cfg := Load()
	if cfg.Retention.UsageLogsDays != 90 ||
		cfg.Retention.SchedulerDecisionsDays != 90 ||
		cfg.Retention.SchedulerFeedbacksDays != 90 ||
		cfg.Retention.AuditLogsDays != 365 ||
		cfg.Retention.AccountHealthSnapshotsDays != 90 {
		t.Fatalf("unexpected retention defaults: %+v", cfg.Retention)
	}

	t.Setenv("DATA_RETENTION_USAGE_LOGS_DAYS", "30")
	t.Setenv("DATA_RETENTION_SCHEDULER_DECISIONS_DAYS", "31")
	t.Setenv("DATA_RETENTION_SCHEDULER_FEEDBACKS_DAYS", "32")
	t.Setenv("DATA_RETENTION_AUDIT_LOGS_DAYS", "180")
	t.Setenv("DATA_RETENTION_ACCOUNT_HEALTH_SNAPSHOTS_DAYS", "45")
	cfg = Load()
	if cfg.Retention.UsageLogsDays != 30 ||
		cfg.Retention.SchedulerDecisionsDays != 31 ||
		cfg.Retention.SchedulerFeedbacksDays != 32 ||
		cfg.Retention.AuditLogsDays != 180 ||
		cfg.Retention.AccountHealthSnapshotsDays != 45 {
		t.Fatalf("unexpected retention overrides: %+v", cfg.Retention)
	}

	cfg.Retention.UsageLogsDays = -1
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "DATA_RETENTION_USAGE_LOGS_DAYS") {
		t.Fatalf("expected retention validation failure, got %v", err)
	}
}

func validReleaseConfig() Config {
	cfg := Load()
	cfg.Server.Mode = "release"
	cfg.Security.JWTSecret = "jwt_secret_release_value_32_bytes_minimum"
	cfg.Security.MasterKey = "master_key_release_value_32_bytes_min"
	cfg.Security.APIKeyPepper = "api_key_pepper_release_value_32_bytes_min"
	cfg.Database.Password = "postgres_release_password_32_bytes_min"
	cfg.Bootstrap.AdminPassword = "bootstrap_admin_release_password_minimum"
	return cfg
}
