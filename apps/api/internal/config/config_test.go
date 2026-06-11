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
	cfg.Security.TOTPEncryptionKey = "local_dev_master_key_32_bytes_minimum_change_me"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "TOTP_ENCRYPTION_KEY") {
		t.Fatalf("expected weak TOTP encryption key rejection, got %v", err)
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

func TestStorageBackendDefaultsOverridesAndValidation(t *testing.T) {
	t.Setenv("STORAGE_BACKEND", "")
	cfg := Load()
	if cfg.StorageBackend() != StorageBackendPostgres {
		t.Fatalf("expected default postgres storage backend, got %s", cfg.StorageBackend())
	}

	t.Setenv("STORAGE_BACKEND", "memory")
	cfg = Load()
	if !cfg.UsesMemoryStorage() {
		t.Fatalf("expected memory storage backend, got %s", cfg.StorageBackend())
	}

	cfg.Storage.Backend = "sqlite"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "STORAGE_BACKEND") {
		t.Fatalf("expected invalid storage backend rejection, got %v", err)
	}

	cfg = validReleaseConfig()
	cfg.Storage.Backend = StorageBackendMemory
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "STORAGE_BACKEND=memory") {
		t.Fatalf("expected release memory backend rejection, got %v", err)
	}
}

func TestValidateAcceptsReleaseStrongSecrets(t *testing.T) {
	cfg := validReleaseConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected release config to validate, got %v", err)
	}
}

func TestEmailConfigDefaultsOverridesAndValidation(t *testing.T) {
	t.Setenv("EMAIL_PUBLIC_BASE_URL", "")
	t.Setenv("EMAIL_SMTP_HOST", "")
	t.Setenv("EMAIL_SMTP_PORT", "")
	t.Setenv("EMAIL_SMTP_USERNAME", "")
	t.Setenv("EMAIL_SMTP_PASSWORD", "")
	t.Setenv("EMAIL_SMTP_FROM", "")
	t.Setenv("EMAIL_SMTP_FROM_NAME", "")
	t.Setenv("EMAIL_SMTP_USE_TLS", "")
	cfg := Load()
	if cfg.Email.SMTPPort != 587 || cfg.Email.PublicBaseURL != "" || cfg.Email.SMTPHost != "" || cfg.Email.SMTPUseTLS {
		t.Fatalf("unexpected email defaults: %+v", cfg.Email)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected default email config to validate, got %v", err)
	}

	t.Setenv("EMAIL_PUBLIC_BASE_URL", "https://console.example.com/")
	t.Setenv("EMAIL_SMTP_HOST", "smtp.example.com")
	t.Setenv("EMAIL_SMTP_PORT", "2525")
	t.Setenv("EMAIL_SMTP_USERNAME", "sender")
	t.Setenv("EMAIL_SMTP_PASSWORD", "secret")
	t.Setenv("EMAIL_SMTP_FROM", "noreply@example.com")
	t.Setenv("EMAIL_SMTP_FROM_NAME", "SRapi")
	t.Setenv("EMAIL_SMTP_USE_TLS", "true")
	cfg = Load()
	if cfg.Email.PublicBaseURL != "https://console.example.com" ||
		cfg.Email.SMTPHost != "smtp.example.com" ||
		cfg.Email.SMTPPort != 2525 ||
		cfg.Email.SMTPUsername != "sender" ||
		cfg.Email.SMTPPassword != "secret" ||
		cfg.Email.SMTPFrom != "noreply@example.com" ||
		cfg.Email.SMTPFromName != "SRapi" ||
		!cfg.Email.SMTPUseTLS {
		t.Fatalf("unexpected email overrides: %+v", cfg.Email)
	}

	cfg.Email.SMTPPort = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "EMAIL_SMTP_PORT") {
		t.Fatalf("expected SMTP port validation failure, got %v", err)
	}
	cfg = Load()
	cfg.Email.PublicBaseURL = "javascript:alert(1)"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "EMAIL_PUBLIC_BASE_URL") {
		t.Fatalf("expected public base URL validation failure, got %v", err)
	}
}

func TestTOTPEncryptionKeyDefaultsToMasterKey(t *testing.T) {
	t.Setenv("SRAPI_MASTER_KEY", "master_key_for_totp_default_32_bytes")
	t.Setenv("TOTP_ENCRYPTION_KEY", "")
	cfg := Load()
	if cfg.Security.TOTPEncryptionKey != cfg.Security.MasterKey {
		t.Fatalf("expected TOTP key to default to master key")
	}

	t.Setenv("TOTP_ENCRYPTION_KEY", "dedicated_totp_key_32_bytes_minimum")
	cfg = Load()
	if cfg.Security.TOTPEncryptionKey != "dedicated_totp_key_32_bytes_minimum" {
		t.Fatalf("expected dedicated TOTP key, got %q", cfg.Security.TOTPEncryptionKey)
	}
}

func TestGatewayTimeoutDefaultsAndOverrides(t *testing.T) {
	t.Setenv("GATEWAY_MAX_BODY_SIZE", "")
	t.Setenv("GATEWAY_REQUEST_TIMEOUT_SECONDS", "")
	t.Setenv("GATEWAY_STREAM_IDLE_TIMEOUT_SECONDS", "")
	t.Setenv("GATEWAY_IMAGE_STREAM_IDLE_TIMEOUT_SECONDS", "")
	t.Setenv("GATEWAY_REALTIME_MAX_OPEN_SLOTS", "")
	t.Setenv("GATEWAY_REALTIME_MAX_OPEN_SLOTS_PER_API_KEY", "")
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
	if cfg.Gateway.ImageStreamIdleTimeout != 900*time.Second {
		t.Fatalf("expected default gateway image stream idle timeout 900s, got %s", cfg.Gateway.ImageStreamIdleTimeout)
	}
	if cfg.Gateway.RealtimeMaxOpenSlots != 0 || cfg.Gateway.RealtimeMaxOpenSlotsPerKey != 0 {
		t.Fatalf("expected realtime slot limits disabled by default, got %+v", cfg.Gateway)
	}

	t.Setenv("GATEWAY_MAX_BODY_SIZE", "12345")
	t.Setenv("GATEWAY_REQUEST_TIMEOUT_SECONDS", "42")
	t.Setenv("GATEWAY_STREAM_IDLE_TIMEOUT_SECONDS", "7")
	t.Setenv("GATEWAY_IMAGE_STREAM_IDLE_TIMEOUT_SECONDS", "11")
	t.Setenv("GATEWAY_REALTIME_MAX_OPEN_SLOTS", "100")
	t.Setenv("GATEWAY_REALTIME_MAX_OPEN_SLOTS_PER_API_KEY", "5")
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
	if cfg.Gateway.ImageStreamIdleTimeout != 11*time.Second {
		t.Fatalf("expected overridden gateway image stream idle timeout 11s, got %s", cfg.Gateway.ImageStreamIdleTimeout)
	}
	if cfg.Gateway.RealtimeMaxOpenSlots != 100 || cfg.Gateway.RealtimeMaxOpenSlotsPerKey != 5 {
		t.Fatalf("expected overridden realtime limits, got %+v", cfg.Gateway)
	}
}

func TestRedisConnectionDefaultsOverridesAndValidation(t *testing.T) {
	t.Setenv("REDIS_POOL_SIZE", "")
	t.Setenv("REDIS_MIN_IDLE_CONNS", "")
	t.Setenv("REDIS_DIAL_TIMEOUT_SECONDS", "")
	t.Setenv("REDIS_READ_TIMEOUT_SECONDS", "")
	t.Setenv("REDIS_WRITE_TIMEOUT_SECONDS", "")
	t.Setenv("REDIS_POOL_TIMEOUT_SECONDS", "")
	cfg := Load()
	if cfg.Redis.PoolSize != 32 ||
		cfg.Redis.MinIdleConns != 4 ||
		cfg.Redis.DialTimeoutSeconds != 3 ||
		cfg.Redis.ReadTimeoutSeconds != 2 ||
		cfg.Redis.WriteTimeoutSeconds != 2 ||
		cfg.Redis.PoolTimeoutSeconds != 3 {
		t.Fatalf("unexpected redis defaults: %+v", cfg.Redis)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected default redis config to validate, got %v", err)
	}

	t.Setenv("REDIS_POOL_SIZE", "12")
	t.Setenv("REDIS_MIN_IDLE_CONNS", "2")
	t.Setenv("REDIS_DIAL_TIMEOUT_SECONDS", "4")
	t.Setenv("REDIS_READ_TIMEOUT_SECONDS", "5")
	t.Setenv("REDIS_WRITE_TIMEOUT_SECONDS", "6")
	t.Setenv("REDIS_POOL_TIMEOUT_SECONDS", "7")
	cfg = Load()
	if cfg.Redis.PoolSize != 12 ||
		cfg.Redis.MinIdleConns != 2 ||
		cfg.Redis.DialTimeoutSeconds != 4 ||
		cfg.Redis.ReadTimeoutSeconds != 5 ||
		cfg.Redis.WriteTimeoutSeconds != 6 ||
		cfg.Redis.PoolTimeoutSeconds != 7 {
		t.Fatalf("unexpected redis overrides: %+v", cfg.Redis)
	}

	cfg.Redis.PoolSize = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "REDIS_POOL_SIZE") {
		t.Fatalf("expected redis pool validation failure, got %v", err)
	}

	cfg = Load()
	cfg.Redis.MinIdleConns = cfg.Redis.PoolSize + 1
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "REDIS_MIN_IDLE_CONNS") {
		t.Fatalf("expected redis min idle validation failure, got %v", err)
	}

	cfg = Load()
	cfg.Redis.ReadTimeoutSeconds = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "REDIS_READ_TIMEOUT_SECONDS") {
		t.Fatalf("expected redis read timeout validation failure, got %v", err)
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

	cfg = Load()
	cfg.Gateway.ImageStreamIdleTimeout = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "GATEWAY_IMAGE_STREAM_IDLE_TIMEOUT_SECONDS") {
		t.Fatalf("expected gateway image stream timeout rejection, got %v", err)
	}

	cfg = Load()
	cfg.Gateway.RealtimeMaxOpenSlots = -1
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "GATEWAY_REALTIME_MAX_OPEN_SLOTS") {
		t.Fatalf("expected gateway realtime max slots rejection, got %v", err)
	}

	cfg = Load()
	cfg.Gateway.RealtimeMaxOpenSlotsPerKey = -1
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "GATEWAY_REALTIME_MAX_OPEN_SLOTS_PER_API_KEY") {
		t.Fatalf("expected gateway realtime per-key slots rejection, got %v", err)
	}
}

func TestRetentionDefaultsOverridesAndValidation(t *testing.T) {
	t.Setenv("DATA_RETENTION_USAGE_LOGS_DAYS", "")
	t.Setenv("DATA_RETENTION_SCHEDULER_DECISIONS_DAYS", "")
	t.Setenv("DATA_RETENTION_SCHEDULER_FEEDBACKS_DAYS", "")
	t.Setenv("DATA_RETENTION_AUDIT_LOGS_DAYS", "")
	t.Setenv("DATA_RETENTION_ACCOUNT_HEALTH_SNAPSHOTS_DAYS", "")
	t.Setenv("DATA_RETENTION_BATCH_LIMIT", "")
	t.Setenv("AUTH_SESSION_CLEANUP_INTERVAL_SECONDS", "")
	cfg := Load()
	if cfg.Retention.UsageLogsDays != 90 ||
		cfg.Retention.SchedulerDecisionsDays != 90 ||
		cfg.Retention.SchedulerFeedbacksDays != 90 ||
		cfg.Retention.AuditLogsDays != 365 ||
		cfg.Retention.AccountHealthSnapshotsDays != 90 ||
		cfg.Retention.BatchLimit != 1000 ||
		cfg.AuthCleanup.Interval != 24*time.Hour {
		t.Fatalf("unexpected retention defaults: %+v", cfg.Retention)
	}

	t.Setenv("DATA_RETENTION_USAGE_LOGS_DAYS", "30")
	t.Setenv("DATA_RETENTION_SCHEDULER_DECISIONS_DAYS", "31")
	t.Setenv("DATA_RETENTION_SCHEDULER_FEEDBACKS_DAYS", "32")
	t.Setenv("DATA_RETENTION_AUDIT_LOGS_DAYS", "180")
	t.Setenv("DATA_RETENTION_ACCOUNT_HEALTH_SNAPSHOTS_DAYS", "45")
	t.Setenv("DATA_RETENTION_BATCH_LIMIT", "2500")
	t.Setenv("AUTH_SESSION_CLEANUP_INTERVAL_SECONDS", "3600")
	cfg = Load()
	if cfg.Retention.UsageLogsDays != 30 ||
		cfg.Retention.SchedulerDecisionsDays != 31 ||
		cfg.Retention.SchedulerFeedbacksDays != 32 ||
		cfg.Retention.AuditLogsDays != 180 ||
		cfg.Retention.AccountHealthSnapshotsDays != 45 ||
		cfg.Retention.BatchLimit != 2500 ||
		cfg.AuthCleanup.Interval != time.Hour {
		t.Fatalf("unexpected retention overrides: %+v", cfg.Retention)
	}

	cfg.Retention.UsageLogsDays = -1
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "DATA_RETENTION_USAGE_LOGS_DAYS") {
		t.Fatalf("expected retention validation failure, got %v", err)
	}

	cfg = Load()
	cfg.Retention.BatchLimit = 5001
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "DATA_RETENTION_BATCH_LIMIT") {
		t.Fatalf("expected retention batch validation failure, got %v", err)
	}

	cfg = Load()
	cfg.AuthCleanup.Interval = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "AUTH_SESSION_CLEANUP_INTERVAL_SECONDS") {
		t.Fatalf("expected auth session cleanup validation failure, got %v", err)
	}
}

func TestBalanceChargerDefaultsOverridesAndValidation(t *testing.T) {
	t.Setenv("BALANCE_CHARGER_INTERVAL_SECONDS", "")
	t.Setenv("BALANCE_CHARGER_BATCH_LIMIT", "")
	t.Setenv("BALANCE_CHARGER_MAX_BATCHES_PER_RUN", "")
	cfg := Load()
	if cfg.BalanceCharger.Interval != time.Minute ||
		cfg.BalanceCharger.BatchLimit != 500 ||
		cfg.BalanceCharger.MaxBatchesPerRun != 20 {
		t.Fatalf("unexpected balance charger defaults: %+v", cfg.BalanceCharger)
	}

	t.Setenv("BALANCE_CHARGER_INTERVAL_SECONDS", "30")
	t.Setenv("BALANCE_CHARGER_BATCH_LIMIT", "250")
	t.Setenv("BALANCE_CHARGER_MAX_BATCHES_PER_RUN", "8")
	cfg = Load()
	if cfg.BalanceCharger.Interval != 30*time.Second ||
		cfg.BalanceCharger.BatchLimit != 250 ||
		cfg.BalanceCharger.MaxBatchesPerRun != 8 {
		t.Fatalf("unexpected balance charger overrides: %+v", cfg.BalanceCharger)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected balance charger config to validate, got %v", err)
	}

	cfg.BalanceCharger.Interval = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "BALANCE_CHARGER_INTERVAL_SECONDS") {
		t.Fatalf("expected balance charger interval validation failure, got %v", err)
	}

	cfg = Load()
	cfg.BalanceCharger.BatchLimit = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "BALANCE_CHARGER_BATCH_LIMIT") {
		t.Fatalf("expected balance charger batch limit validation failure, got %v", err)
	}

	cfg = Load()
	cfg.BalanceCharger.MaxBatchesPerRun = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "BALANCE_CHARGER_MAX_BATCHES_PER_RUN") {
		t.Fatalf("expected balance charger max batches validation failure, got %v", err)
	}
}

func TestHealthProbeDefaultsOverridesAndValidation(t *testing.T) {
	t.Setenv("ACCOUNT_HEALTH_PROBE_INTERVAL_SECONDS", "")
	t.Setenv("ACCOUNT_HEALTH_PROBE_TIMEOUT_SECONDS", "")
	t.Setenv("ACCOUNT_HEALTH_PROBE_MAX_CONCURRENT", "")
	t.Setenv("ACCOUNT_HEALTH_PROBE_FAILURE_THRESHOLD", "")
	t.Setenv("ACCOUNT_HEALTH_PROBE_ERROR_RATE_THRESHOLD_PERCENT", "")
	t.Setenv("ACCOUNT_HEALTH_PROBE_MIN_SAMPLES_FOR_ERROR_RATE", "")
	t.Setenv("ACCOUNT_HEALTH_PROBE_COOLDOWN_SECONDS", "")
	cfg := Load()
	if cfg.HealthProbe.Interval != 5*time.Minute ||
		cfg.HealthProbe.Timeout != 10*time.Second ||
		cfg.HealthProbe.MaxConcurrent != 8 ||
		cfg.HealthProbe.FailureThreshold != 3 ||
		cfg.HealthProbe.ErrorRateThreshold != 0.5 ||
		cfg.HealthProbe.MinSamplesForErrorRate != 3 ||
		cfg.HealthProbe.Cooldown != 5*time.Minute {
		t.Fatalf("unexpected health probe defaults: %+v", cfg.HealthProbe)
	}

	t.Setenv("ACCOUNT_HEALTH_PROBE_INTERVAL_SECONDS", "60")
	t.Setenv("ACCOUNT_HEALTH_PROBE_TIMEOUT_SECONDS", "3")
	t.Setenv("ACCOUNT_HEALTH_PROBE_MAX_CONCURRENT", "4")
	t.Setenv("ACCOUNT_HEALTH_PROBE_FAILURE_THRESHOLD", "2")
	t.Setenv("ACCOUNT_HEALTH_PROBE_ERROR_RATE_THRESHOLD_PERCENT", "75")
	t.Setenv("ACCOUNT_HEALTH_PROBE_MIN_SAMPLES_FOR_ERROR_RATE", "5")
	t.Setenv("ACCOUNT_HEALTH_PROBE_COOLDOWN_SECONDS", "120")
	cfg = Load()
	if cfg.HealthProbe.Interval != time.Minute ||
		cfg.HealthProbe.Timeout != 3*time.Second ||
		cfg.HealthProbe.MaxConcurrent != 4 ||
		cfg.HealthProbe.FailureThreshold != 2 ||
		cfg.HealthProbe.ErrorRateThreshold != 0.75 ||
		cfg.HealthProbe.MinSamplesForErrorRate != 5 ||
		cfg.HealthProbe.Cooldown != 2*time.Minute {
		t.Fatalf("unexpected health probe overrides: %+v", cfg.HealthProbe)
	}

	cfg.HealthProbe.MaxConcurrent = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "ACCOUNT_HEALTH_PROBE_MAX_CONCURRENT") {
		t.Fatalf("expected health probe validation failure, got %v", err)
	}

	cfg = Load()
	cfg.HealthProbe.ErrorRateThreshold = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "ACCOUNT_HEALTH_PROBE_ERROR_RATE_THRESHOLD_PERCENT") {
		t.Fatalf("expected health probe error rate validation failure, got %v", err)
	}
}

func TestQualityEvalDefaultsOverridesAndValidation(t *testing.T) {
	t.Setenv("QUALITY_EVAL_ENABLED", "")
	t.Setenv("QUALITY_EVAL_INTERVAL_SECONDS", "")
	t.Setenv("QUALITY_EVAL_TIMEOUT_SECONDS", "")
	t.Setenv("QUALITY_EVAL_BATCH_LIMIT", "")
	t.Setenv("QUALITY_EVAL_SAMPLE_PERCENT", "")
	t.Setenv("QUALITY_EVAL_OPENAI_API_KEY", "")
	t.Setenv("QUALITY_EVAL_OPENAI_BASE_URL", "")
	t.Setenv("QUALITY_EVAL_JUDGE_MODEL", "")
	t.Setenv("QUALITY_EVAL_JUDGE_TIMEOUT_SECONDS", "")
	cfg := Load()
	if cfg.QualityEval.Enabled ||
		cfg.QualityEval.Interval != time.Hour ||
		cfg.QualityEval.Timeout != 30*time.Second ||
		cfg.QualityEval.BatchLimit != 100 ||
		cfg.QualityEval.SamplePercent != 1 ||
		cfg.QualityEval.JudgeModel != "gpt-4o-mini" ||
		cfg.QualityEval.JudgeTimeout != 20*time.Second {
		t.Fatalf("unexpected quality eval defaults: %+v", cfg.QualityEval)
	}

	t.Setenv("QUALITY_EVAL_ENABLED", "true")
	t.Setenv("QUALITY_EVAL_INTERVAL_SECONDS", "1800")
	t.Setenv("QUALITY_EVAL_TIMEOUT_SECONDS", "15")
	t.Setenv("QUALITY_EVAL_BATCH_LIMIT", "25")
	t.Setenv("QUALITY_EVAL_SAMPLE_PERCENT", "2.5")
	t.Setenv("QUALITY_EVAL_OPENAI_API_KEY", "judge-key")
	t.Setenv("QUALITY_EVAL_OPENAI_BASE_URL", "https://judge.example/v1")
	t.Setenv("QUALITY_EVAL_JUDGE_MODEL", "gpt-4o-mini")
	t.Setenv("QUALITY_EVAL_JUDGE_TIMEOUT_SECONDS", "9")
	cfg = Load()
	if !cfg.QualityEval.Enabled ||
		cfg.QualityEval.Interval != 30*time.Minute ||
		cfg.QualityEval.Timeout != 15*time.Second ||
		cfg.QualityEval.BatchLimit != 25 ||
		cfg.QualityEval.SamplePercent != 2.5 ||
		cfg.QualityEval.OpenAIAPIKey != "judge-key" ||
		cfg.QualityEval.OpenAIBaseURL != "https://judge.example/v1" ||
		cfg.QualityEval.JudgeTimeout != 9*time.Second {
		t.Fatalf("unexpected quality eval overrides: %+v", cfg.QualityEval)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected enabled quality eval config to validate, got %v", err)
	}

	cfg.QualityEval.OpenAIAPIKey = ""
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "QUALITY_EVAL_OPENAI_API_KEY") {
		t.Fatalf("expected missing quality judge key validation failure, got %v", err)
	}

	cfg = Load()
	cfg.QualityEval.SamplePercent = 101
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "QUALITY_EVAL_SAMPLE_PERCENT") {
		t.Fatalf("expected quality sample percent validation failure, got %v", err)
	}
}

func TestSLOEvaluatorDefaultsOverridesAndValidation(t *testing.T) {
	t.Setenv("SLO_EVALUATOR_INTERVAL_SECONDS", "")
	t.Setenv("SLO_EVALUATOR_TIMEOUT_SECONDS", "")
	cfg := Load()
	if cfg.SLOEvaluator.Interval != time.Minute || cfg.SLOEvaluator.Timeout != 30*time.Second {
		t.Fatalf("unexpected SLO evaluator defaults: %+v", cfg.SLOEvaluator)
	}

	t.Setenv("SLO_EVALUATOR_INTERVAL_SECONDS", "120")
	t.Setenv("SLO_EVALUATOR_TIMEOUT_SECONDS", "5")
	cfg = Load()
	if cfg.SLOEvaluator.Interval != 2*time.Minute || cfg.SLOEvaluator.Timeout != 5*time.Second {
		t.Fatalf("unexpected SLO evaluator overrides: %+v", cfg.SLOEvaluator)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected SLO evaluator config to validate, got %v", err)
	}

	cfg.SLOEvaluator.Interval = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "SLO_EVALUATOR_INTERVAL_SECONDS") {
		t.Fatalf("expected SLO evaluator interval validation failure, got %v", err)
	}

	cfg = Load()
	cfg.SLOEvaluator.Timeout = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "SLO_EVALUATOR_TIMEOUT_SECONDS") {
		t.Fatalf("expected SLO evaluator timeout validation failure, got %v", err)
	}
}

func TestLiteLLMPricingDefaultsOverridesAndValidation(t *testing.T) {
	t.Setenv("LITELLM_PRICING_SOURCE_URL", "")
	t.Setenv("LITELLM_PRICING_INTERVAL_SECONDS", "")
	t.Setenv("LITELLM_PRICING_TIMEOUT_SECONDS", "")
	cfg := Load()
	if cfg.LiteLLMPricing.SourceURL != "" ||
		cfg.LiteLLMPricing.Interval != 12*time.Hour ||
		cfg.LiteLLMPricing.Timeout != 15*time.Second {
		t.Fatalf("unexpected LiteLLM pricing defaults: %+v", cfg.LiteLLMPricing)
	}

	t.Setenv("LITELLM_PRICING_SOURCE_URL", "https://prices.example.com/model_prices.json")
	t.Setenv("LITELLM_PRICING_INTERVAL_SECONDS", "3600")
	t.Setenv("LITELLM_PRICING_TIMEOUT_SECONDS", "7")
	cfg = Load()
	if cfg.LiteLLMPricing.SourceURL != "https://prices.example.com/model_prices.json" ||
		cfg.LiteLLMPricing.Interval != time.Hour ||
		cfg.LiteLLMPricing.Timeout != 7*time.Second {
		t.Fatalf("unexpected LiteLLM pricing overrides: %+v", cfg.LiteLLMPricing)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected LiteLLM pricing config to validate, got %v", err)
	}

	cfg.LiteLLMPricing.SourceURL = "not-a-url"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "LITELLM_PRICING_SOURCE_URL") {
		t.Fatalf("expected LiteLLM pricing URL validation failure, got %v", err)
	}

	cfg = Load()
	cfg.LiteLLMPricing.Interval = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "LITELLM_PRICING_INTERVAL_SECONDS") {
		t.Fatalf("expected LiteLLM pricing interval validation failure, got %v", err)
	}
}

func TestObservabilityDefaultsOverridesAndValidation(t *testing.T) {
	t.Setenv("LOG_SERVICE_NAME", "")
	t.Setenv("LOG_ENV", "")
	t.Setenv("OTEL_SERVICE_NAME", "")
	t.Setenv("OTEL_SERVICE_VERSION", "")
	t.Setenv("OTEL_ENVIRONMENT", "")
	t.Setenv("OTEL_TRACES_ENABLED", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "")
	t.Setenv("OTEL_TRACES_SAMPLE_RATIO", "")
	t.Setenv("OTEL_BATCH_TIMEOUT_SECONDS", "")
	cfg := Load()
	if cfg.Observability.ServiceName != "srapi" ||
		cfg.Observability.Environment != "local" ||
		cfg.Observability.TracesEnabled ||
		cfg.Observability.OTLPEndpoint != "localhost:4317" ||
		!cfg.Observability.OTLPInsecure ||
		cfg.Observability.TraceSampleRatio != 1 ||
		cfg.Observability.BatchTimeout != 5*time.Second {
		t.Fatalf("unexpected observability defaults: %+v", cfg.Observability)
	}

	t.Setenv("OTEL_SERVICE_NAME", "srapi-api")
	t.Setenv("OTEL_SERVICE_VERSION", "2026.5")
	t.Setenv("OTEL_ENVIRONMENT", "staging")
	t.Setenv("OTEL_TRACES_ENABLED", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector:4317")
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "false")
	t.Setenv("OTEL_TRACES_SAMPLE_RATIO", "0.25")
	t.Setenv("OTEL_BATCH_TIMEOUT_SECONDS", "2")
	cfg = Load()
	if cfg.Observability.ServiceName != "srapi-api" ||
		cfg.Observability.ServiceVersion != "2026.5" ||
		cfg.Observability.Environment != "staging" ||
		!cfg.Observability.TracesEnabled ||
		cfg.Observability.OTLPEndpoint != "otel-collector:4317" ||
		cfg.Observability.OTLPInsecure ||
		cfg.Observability.TraceSampleRatio != 0.25 ||
		cfg.Observability.BatchTimeout != 2*time.Second {
		t.Fatalf("unexpected observability overrides: %+v", cfg.Observability)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected observability config to validate, got %v", err)
	}

	cfg.Observability.TraceSampleRatio = 1.01
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "OTEL_TRACES_SAMPLE_RATIO") {
		t.Fatalf("expected trace sample ratio validation failure, got %v", err)
	}

	cfg = Load()
	cfg.Observability.TracesEnabled = true
	cfg.Observability.OTLPEndpoint = ""
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "OTEL_EXPORTER_OTLP_ENDPOINT") {
		t.Fatalf("expected missing OTLP endpoint validation failure, got %v", err)
	}
}

func validReleaseConfig() Config {
	cfg := Load()
	cfg.Server.Mode = "release"
	cfg.Security.JWTSecret = "jwt_secret_release_value_32_bytes_minimum"
	cfg.Security.MasterKey = "master_key_release_value_32_bytes_min"
	cfg.Security.TOTPEncryptionKey = "totp_key_release_value_32_bytes_min"
	cfg.Security.APIKeyPepper = "api_key_pepper_release_value_32_bytes_min"
	cfg.Database.Password = "postgres_release_password_32_bytes_min"
	cfg.Storage.Backend = StorageBackendPostgres
	cfg.Bootstrap.AdminPassword = "bootstrap_admin_release_password_minimum"
	return cfg
}
