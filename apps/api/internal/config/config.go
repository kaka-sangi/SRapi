package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHost            = "0.0.0.0"
	defaultPort            = 8080
	defaultShutdownSeconds = 45
	defaultVersion         = "0.1.0"
	defaultGatewayBodySize = 268435456
	StorageBackendPostgres = "postgres"
	StorageBackendMemory   = "memory"
)

type Config struct {
	Server           ServerConfig
	Storage          StorageConfig
	Database         DependencyConfig
	Redis            DependencyConfig
	Gateway          GatewayConfig
	Security         SecurityConfig
	Bootstrap        BootstrapConfig
	Retention        RetentionConfig
	AuthCleanup      AuthCleanupConfig
	BalanceCharger   BalanceChargerConfig
	HealthProbe      HealthProbeConfig
	QualityEval      QualityEvalConfig
	SLOEvaluator     SLOEvaluatorConfig
	Email            EmailConfig
	Observability    ObservabilityConfig
	Captcha          CaptchaConfig
	QuotaRefresh     QuotaRefreshConfig
	ConnectivityTest ConnectivityTestConfig
	OAuth            OAuthConfig
}

// QuotaRefreshConfig controls the scheduled per-account quota/subscription
// refresh worker. Disabled by default; accounts still need a configured quota
// endpoint for the worker to act on them.
type QuotaRefreshConfig struct {
	Enabled       bool
	Interval      time.Duration
	Timeout       time.Duration
	MaxConcurrent int
}

// ConnectivityTestConfig controls the scheduled connectivity test worker, which
// issues a real (billable) generative probe to opt-in accounts of any runtime
// class. Disabled by default; only accounts with a configured probe model are
// tested.
type ConnectivityTestConfig struct {
	Enabled       bool
	Interval      time.Duration
	Timeout       time.Duration
	MaxConcurrent int
}

type ServerConfig struct {
	Host            string
	Port            int
	Mode            string
	Version         string
	ShutdownTimeout time.Duration
}

type StorageConfig struct {
	Backend string
}

type DependencyConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string
}

type GatewayConfig struct {
	MaxBodySize                int64
	RequestTimeout             time.Duration
	StreamIdleTimeout          time.Duration
	RealtimeMaxOpenSlots       int
	RealtimeMaxOpenSlotsPerKey int
	// RequirePositiveBalance, when true, synchronously rejects balance-billed
	// gateway requests (pay-go users, or allowance-mode subscription overage)
	// from users whose balance no longer covers the request, closing the
	// deferred-charging overspend window. It never blocks hard_cap subscription
	// users (who never bill to balance). Default false preserves the historical
	// fail-open behavior.
	RequirePositiveBalance bool
}

type SecurityConfig struct {
	JWTSecret         string
	MasterKey         string
	TOTPEncryptionKey string
	APIKeyPepper      string
}

type BootstrapConfig struct {
	AdminEmail    string
	AdminPassword string
	AdminName     string
}

// CaptchaConfig controls human-verification (Cloudflare Turnstile by default) on
// auth endpoints. When Enabled is false the verifier is a no-op.
type CaptchaConfig struct {
	Enabled   bool
	Provider  string // turnstile | hcaptcha | recaptcha
	SecretKey string
	VerifyURL string // optional override of the provider's siteverify endpoint
}

// OAuthConfig holds console-login OAuth deployment-env config (never in
// AdminSettings). ClientSecrets is keyed by provider_key; when a key is present
// the token exchange runs as a confidential client (client_secret). Issuers is
// keyed by provider_key; when set, the provider's id_token is verified (OIDC:
// signature via JWKS + iss/aud/exp + nonce).
type OAuthConfig struct {
	ClientSecrets map[string]string
	Issuers       map[string]string
}

type RetentionConfig struct {
	UsageLogsDays              int
	SchedulerDecisionsDays     int
	SchedulerFeedbacksDays     int
	AuditLogsDays              int
	AccountHealthSnapshotsDays int
}

// AuthCleanupConfig controls expired console session cleanup.
type AuthCleanupConfig struct {
	Interval time.Duration
}

// BalanceChargerConfig controls pending usage charging throughput.
type BalanceChargerConfig struct {
	Interval         time.Duration
	BatchLimit       int
	MaxBatchesPerRun int
}

// HealthProbeConfig controls the account health probe worker.
type HealthProbeConfig struct {
	Interval               time.Duration
	Timeout                time.Duration
	MaxConcurrent          int
	FailureThreshold       int
	ErrorRateThreshold     float32
	MinSamplesForErrorRate int
	Cooldown               time.Duration
}

// QualityEvalConfig controls the LLM-as-judge quality evaluation worker.
type QualityEvalConfig struct {
	Enabled       bool
	Interval      time.Duration
	Timeout       time.Duration
	BatchLimit    int
	SamplePercent float64
	OpenAIAPIKey  string
	OpenAIBaseURL string
	JudgeModel    string
	JudgeTimeout  time.Duration
}

// SLOEvaluatorConfig controls the SLO burn-rate alert evaluator worker.
type SLOEvaluatorConfig struct {
	Interval time.Duration
	Timeout  time.Duration
}

// EmailConfig controls outbound transactional email delivery.
type EmailConfig struct {
	PublicBaseURL string
	SMTPHost      string
	SMTPPort      int
	SMTPUsername  string
	SMTPPassword  string
	SMTPFrom      string
	SMTPFromName  string
	SMTPUseTLS    bool
}

// ObservabilityConfig controls process-wide tracing and structured diagnostics.
type ObservabilityConfig struct {
	ServiceName      string
	ServiceVersion   string
	Environment      string
	TracesEnabled    bool
	OTLPEndpoint     string
	OTLPInsecure     bool
	TraceSampleRatio float64
	BatchTimeout     time.Duration
}

func Load() Config {
	return Config{
		Server: ServerConfig{
			Host:            getEnv("SERVER_HOST", defaultHost),
			Port:            getIntEnv("SERVER_PORT", defaultPort),
			Mode:            getEnv("SERVER_MODE", "local"),
			Version:         getEnv("SRAPI_VERSION", defaultVersion),
			ShutdownTimeout: time.Duration(getIntEnv("SERVER_SHUTDOWN_TIMEOUT_SECONDS", defaultShutdownSeconds)) * time.Second,
		},
		Storage: StorageConfig{
			Backend: normalizeStorageBackend(getEnv("STORAGE_BACKEND", StorageBackendPostgres)),
		},
		Database: DependencyConfig{
			Host:     getEnv("DATABASE_HOST", "localhost"),
			Port:     getIntEnv("DATABASE_PORT", 5432),
			User:     getEnv("DATABASE_USER", "srapi"),
			Password: getEnv("DATABASE_PASSWORD", ""),
			Database: getEnv("DATABASE_DBNAME", "srapi"),
			SSLMode:  getEnv("DATABASE_SSLMODE", "disable"),
		},
		Redis: DependencyConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getIntEnv("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			Database: getEnv("REDIS_DB", "0"),
		},
		Gateway: GatewayConfig{
			MaxBodySize:                int64(getIntEnv("GATEWAY_MAX_BODY_SIZE", defaultGatewayBodySize)),
			RequestTimeout:             time.Duration(getIntEnv("GATEWAY_REQUEST_TIMEOUT_SECONDS", 600)) * time.Second,
			StreamIdleTimeout:          time.Duration(getIntEnv("GATEWAY_STREAM_IDLE_TIMEOUT_SECONDS", 120)) * time.Second,
			RealtimeMaxOpenSlots:       getIntEnv("GATEWAY_REALTIME_MAX_OPEN_SLOTS", 0),
			RealtimeMaxOpenSlotsPerKey: getIntEnv("GATEWAY_REALTIME_MAX_OPEN_SLOTS_PER_API_KEY", 0),
			RequirePositiveBalance:     getBoolEnv("GATEWAY_REQUIRE_POSITIVE_BALANCE", false),
		},
		Security: securityConfigFromEnv(),
		Bootstrap: BootstrapConfig{
			AdminEmail:    getEnv("BOOTSTRAP_ADMIN_EMAIL", "admin@srapi.local"),
			AdminPassword: getEnv("BOOTSTRAP_ADMIN_PASSWORD", "password123"),
			AdminName:     getEnv("BOOTSTRAP_ADMIN_NAME", "Admin"),
		},
		Retention: RetentionConfig{
			UsageLogsDays:              getIntEnv("DATA_RETENTION_USAGE_LOGS_DAYS", 90),
			SchedulerDecisionsDays:     getIntEnv("DATA_RETENTION_SCHEDULER_DECISIONS_DAYS", 90),
			SchedulerFeedbacksDays:     getIntEnv("DATA_RETENTION_SCHEDULER_FEEDBACKS_DAYS", 90),
			AuditLogsDays:              getIntEnv("DATA_RETENTION_AUDIT_LOGS_DAYS", 365),
			AccountHealthSnapshotsDays: getIntEnv("DATA_RETENTION_ACCOUNT_HEALTH_SNAPSHOTS_DAYS", 90),
		},
		AuthCleanup: AuthCleanupConfig{
			Interval: time.Duration(getIntEnv("AUTH_SESSION_CLEANUP_INTERVAL_SECONDS", 86400)) * time.Second,
		},
		BalanceCharger: BalanceChargerConfig{
			Interval:         time.Duration(getIntEnv("BALANCE_CHARGER_INTERVAL_SECONDS", 60)) * time.Second,
			BatchLimit:       getIntEnv("BALANCE_CHARGER_BATCH_LIMIT", 500),
			MaxBatchesPerRun: getIntEnv("BALANCE_CHARGER_MAX_BATCHES_PER_RUN", 20),
		},
		HealthProbe: HealthProbeConfig{
			Interval:               time.Duration(getIntEnv("ACCOUNT_HEALTH_PROBE_INTERVAL_SECONDS", 300)) * time.Second,
			Timeout:                time.Duration(getIntEnv("ACCOUNT_HEALTH_PROBE_TIMEOUT_SECONDS", 10)) * time.Second,
			MaxConcurrent:          getIntEnv("ACCOUNT_HEALTH_PROBE_MAX_CONCURRENT", 8),
			FailureThreshold:       getIntEnv("ACCOUNT_HEALTH_PROBE_FAILURE_THRESHOLD", 3),
			ErrorRateThreshold:     float32(getIntEnv("ACCOUNT_HEALTH_PROBE_ERROR_RATE_THRESHOLD_PERCENT", 50)) / 100,
			MinSamplesForErrorRate: getIntEnv("ACCOUNT_HEALTH_PROBE_MIN_SAMPLES_FOR_ERROR_RATE", 3),
			Cooldown:               time.Duration(getIntEnv("ACCOUNT_HEALTH_PROBE_COOLDOWN_SECONDS", 300)) * time.Second,
		},
		QualityEval: QualityEvalConfig{
			Enabled:       getBoolEnv("QUALITY_EVAL_ENABLED", false),
			Interval:      time.Duration(getIntEnv("QUALITY_EVAL_INTERVAL_SECONDS", 3600)) * time.Second,
			Timeout:       time.Duration(getIntEnv("QUALITY_EVAL_TIMEOUT_SECONDS", 30)) * time.Second,
			BatchLimit:    getIntEnv("QUALITY_EVAL_BATCH_LIMIT", 100),
			SamplePercent: getFloatEnv("QUALITY_EVAL_SAMPLE_PERCENT", 1),
			OpenAIAPIKey:  getEnv("QUALITY_EVAL_OPENAI_API_KEY", ""),
			OpenAIBaseURL: getEnv("QUALITY_EVAL_OPENAI_BASE_URL", ""),
			JudgeModel:    getEnv("QUALITY_EVAL_JUDGE_MODEL", "gpt-4o-mini"),
			JudgeTimeout:  time.Duration(getIntEnv("QUALITY_EVAL_JUDGE_TIMEOUT_SECONDS", 20)) * time.Second,
		},
		SLOEvaluator: SLOEvaluatorConfig{
			Interval: time.Duration(getIntEnv("SLO_EVALUATOR_INTERVAL_SECONDS", 60)) * time.Second,
			Timeout:  time.Duration(getIntEnv("SLO_EVALUATOR_TIMEOUT_SECONDS", 30)) * time.Second,
		},
		Email: EmailConfig{
			PublicBaseURL: strings.TrimRight(getEnv("EMAIL_PUBLIC_BASE_URL", ""), "/"),
			SMTPHost:      getEnv("EMAIL_SMTP_HOST", ""),
			SMTPPort:      getIntEnv("EMAIL_SMTP_PORT", 587),
			SMTPUsername:  getEnv("EMAIL_SMTP_USERNAME", ""),
			SMTPPassword:  getEnv("EMAIL_SMTP_PASSWORD", ""),
			SMTPFrom:      getEnv("EMAIL_SMTP_FROM", ""),
			SMTPFromName:  getEnv("EMAIL_SMTP_FROM_NAME", ""),
			SMTPUseTLS:    getBoolEnv("EMAIL_SMTP_USE_TLS", false),
		},
		Observability: ObservabilityConfig{
			ServiceName:      getEnv("OTEL_SERVICE_NAME", getEnv("LOG_SERVICE_NAME", "srapi")),
			ServiceVersion:   getEnv("OTEL_SERVICE_VERSION", getEnv("SRAPI_VERSION", defaultVersion)),
			Environment:      getEnv("OTEL_ENVIRONMENT", getEnv("LOG_ENV", "local")),
			TracesEnabled:    getBoolEnv("OTEL_TRACES_ENABLED", false),
			OTLPEndpoint:     getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
			OTLPInsecure:     getBoolEnv("OTEL_EXPORTER_OTLP_INSECURE", true),
			TraceSampleRatio: getFloatEnv("OTEL_TRACES_SAMPLE_RATIO", 1),
			BatchTimeout:     time.Duration(getIntEnv("OTEL_BATCH_TIMEOUT_SECONDS", 5)) * time.Second,
		},
		Captcha: CaptchaConfig{
			Enabled:   getBoolEnv("CAPTCHA_ENABLED", false),
			Provider:  getEnv("CAPTCHA_PROVIDER", "turnstile"),
			SecretKey: getEnv("CAPTCHA_SECRET_KEY", ""),
			VerifyURL: getEnv("CAPTCHA_VERIFY_URL", ""),
		},
		QuotaRefresh: QuotaRefreshConfig{
			Enabled:       getBoolEnv("ACCOUNT_QUOTA_REFRESH_ENABLED", false),
			Interval:      time.Duration(getIntEnv("ACCOUNT_QUOTA_REFRESH_INTERVAL_SECONDS", 1800)) * time.Second,
			Timeout:       time.Duration(getIntEnv("ACCOUNT_QUOTA_REFRESH_TIMEOUT_SECONDS", 15)) * time.Second,
			MaxConcurrent: getIntEnv("ACCOUNT_QUOTA_REFRESH_MAX_CONCURRENT", 4),
		},
		ConnectivityTest: ConnectivityTestConfig{
			Enabled:       getBoolEnv("ACCOUNT_CONNECTIVITY_TEST_ENABLED", false),
			Interval:      time.Duration(getIntEnv("ACCOUNT_CONNECTIVITY_TEST_INTERVAL_SECONDS", 3600)) * time.Second,
			Timeout:       time.Duration(getIntEnv("ACCOUNT_CONNECTIVITY_TEST_TIMEOUT_SECONDS", 30)) * time.Second,
			MaxConcurrent: getIntEnv("ACCOUNT_CONNECTIVITY_TEST_MAX_CONCURRENT", 2),
		},
		OAuth: OAuthConfig{
			ClientSecrets: parseStringMapEnv("OAUTH_CLIENT_SECRETS_JSON"),
			Issuers:       parseStringMapEnv("OAUTH_ISSUERS_JSON"),
		},
	}
}

func (c Config) Address() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func (c Config) HealthcheckAddress() string {
	if c.Server.Host == "" || c.Server.Host == "0.0.0.0" || c.Server.Host == "::" {
		return fmt.Sprintf("127.0.0.1:%d", c.Server.Port)
	}
	return c.Address()
}

func (c Config) Validate() error {
	switch c.StorageBackend() {
	case StorageBackendPostgres, StorageBackendMemory:
	default:
		return fmt.Errorf("STORAGE_BACKEND must be postgres or memory")
	}
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("SERVER_PORT must be between 1 and 65535")
	}
	if c.Gateway.MaxBodySize <= 0 {
		return fmt.Errorf("GATEWAY_MAX_BODY_SIZE must be positive")
	}
	if c.Gateway.RequestTimeout <= 0 {
		return fmt.Errorf("GATEWAY_REQUEST_TIMEOUT_SECONDS must be positive")
	}
	if c.Gateway.StreamIdleTimeout <= 0 {
		return fmt.Errorf("GATEWAY_STREAM_IDLE_TIMEOUT_SECONDS must be positive")
	}
	if c.Gateway.RealtimeMaxOpenSlots < 0 {
		return fmt.Errorf("GATEWAY_REALTIME_MAX_OPEN_SLOTS must be zero or positive")
	}
	if c.Gateway.RealtimeMaxOpenSlotsPerKey < 0 {
		return fmt.Errorf("GATEWAY_REALTIME_MAX_OPEN_SLOTS_PER_API_KEY must be zero or positive")
	}
	if c.Retention.UsageLogsDays < 0 {
		return fmt.Errorf("DATA_RETENTION_USAGE_LOGS_DAYS must be zero or positive")
	}
	if c.Retention.SchedulerDecisionsDays < 0 {
		return fmt.Errorf("DATA_RETENTION_SCHEDULER_DECISIONS_DAYS must be zero or positive")
	}
	if c.Retention.SchedulerFeedbacksDays < 0 {
		return fmt.Errorf("DATA_RETENTION_SCHEDULER_FEEDBACKS_DAYS must be zero or positive")
	}
	if c.Retention.AuditLogsDays < 0 {
		return fmt.Errorf("DATA_RETENTION_AUDIT_LOGS_DAYS must be zero or positive")
	}
	if c.Retention.AccountHealthSnapshotsDays < 0 {
		return fmt.Errorf("DATA_RETENTION_ACCOUNT_HEALTH_SNAPSHOTS_DAYS must be zero or positive")
	}
	if c.AuthCleanup.Interval <= 0 {
		return fmt.Errorf("AUTH_SESSION_CLEANUP_INTERVAL_SECONDS must be positive")
	}
	if c.BalanceCharger.Interval <= 0 {
		return fmt.Errorf("BALANCE_CHARGER_INTERVAL_SECONDS must be positive")
	}
	if c.BalanceCharger.BatchLimit <= 0 {
		return fmt.Errorf("BALANCE_CHARGER_BATCH_LIMIT must be positive")
	}
	if c.BalanceCharger.MaxBatchesPerRun <= 0 {
		return fmt.Errorf("BALANCE_CHARGER_MAX_BATCHES_PER_RUN must be positive")
	}
	if c.HealthProbe.Interval <= 0 {
		return fmt.Errorf("ACCOUNT_HEALTH_PROBE_INTERVAL_SECONDS must be positive")
	}
	if c.HealthProbe.Timeout <= 0 {
		return fmt.Errorf("ACCOUNT_HEALTH_PROBE_TIMEOUT_SECONDS must be positive")
	}
	if c.HealthProbe.MaxConcurrent <= 0 {
		return fmt.Errorf("ACCOUNT_HEALTH_PROBE_MAX_CONCURRENT must be positive")
	}
	if c.HealthProbe.FailureThreshold <= 0 {
		return fmt.Errorf("ACCOUNT_HEALTH_PROBE_FAILURE_THRESHOLD must be positive")
	}
	if c.HealthProbe.ErrorRateThreshold <= 0 || c.HealthProbe.ErrorRateThreshold > 1 {
		return fmt.Errorf("ACCOUNT_HEALTH_PROBE_ERROR_RATE_THRESHOLD_PERCENT must be greater than 0 and at most 100")
	}
	if c.HealthProbe.MinSamplesForErrorRate <= 0 {
		return fmt.Errorf("ACCOUNT_HEALTH_PROBE_MIN_SAMPLES_FOR_ERROR_RATE must be positive")
	}
	if c.HealthProbe.Cooldown <= 0 {
		return fmt.Errorf("ACCOUNT_HEALTH_PROBE_COOLDOWN_SECONDS must be positive")
	}
	if c.QualityEval.Interval <= 0 {
		return fmt.Errorf("QUALITY_EVAL_INTERVAL_SECONDS must be positive")
	}
	if c.QualityEval.Timeout <= 0 {
		return fmt.Errorf("QUALITY_EVAL_TIMEOUT_SECONDS must be positive")
	}
	if c.QualityEval.BatchLimit <= 0 {
		return fmt.Errorf("QUALITY_EVAL_BATCH_LIMIT must be positive")
	}
	if c.QualityEval.SamplePercent <= 0 || c.QualityEval.SamplePercent > 100 {
		return fmt.Errorf("QUALITY_EVAL_SAMPLE_PERCENT must be greater than 0 and at most 100")
	}
	if c.QualityEval.JudgeTimeout <= 0 {
		return fmt.Errorf("QUALITY_EVAL_JUDGE_TIMEOUT_SECONDS must be positive")
	}
	if c.QualityEval.Enabled && strings.TrimSpace(c.QualityEval.OpenAIAPIKey) == "" {
		return fmt.Errorf("QUALITY_EVAL_OPENAI_API_KEY must be set when QUALITY_EVAL_ENABLED=true")
	}
	if c.SLOEvaluator.Interval <= 0 {
		return fmt.Errorf("SLO_EVALUATOR_INTERVAL_SECONDS must be positive")
	}
	if c.SLOEvaluator.Timeout <= 0 {
		return fmt.Errorf("SLO_EVALUATOR_TIMEOUT_SECONDS must be positive")
	}
	if c.Email.SMTPPort <= 0 || c.Email.SMTPPort > 65535 {
		return fmt.Errorf("EMAIL_SMTP_PORT must be between 1 and 65535")
	}
	if strings.TrimSpace(c.Email.PublicBaseURL) != "" && !validHTTPBaseURL(c.Email.PublicBaseURL) {
		return fmt.Errorf("EMAIL_PUBLIC_BASE_URL must be an absolute http or https URL without query or fragment")
	}
	if strings.TrimSpace(c.Observability.ServiceName) == "" {
		return fmt.Errorf("OTEL_SERVICE_NAME must not be empty")
	}
	if strings.TrimSpace(c.Observability.Environment) == "" {
		return fmt.Errorf("OTEL_ENVIRONMENT must not be empty")
	}
	if c.Observability.TraceSampleRatio < 0 || c.Observability.TraceSampleRatio > 1 {
		return fmt.Errorf("OTEL_TRACES_SAMPLE_RATIO must be between 0 and 1")
	}
	if c.Observability.BatchTimeout <= 0 {
		return fmt.Errorf("OTEL_BATCH_TIMEOUT_SECONDS must be positive")
	}
	if c.Observability.TracesEnabled && strings.TrimSpace(c.Observability.OTLPEndpoint) == "" {
		return fmt.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT must be set when OTEL_TRACES_ENABLED=true")
	}
	if c.Server.Mode == "release" {
		if c.StorageBackend() == StorageBackendMemory {
			return fmt.Errorf("STORAGE_BACKEND=memory is not allowed in release mode")
		}
		if weakSecret(c.Security.JWTSecret) {
			return fmt.Errorf("JWT_SECRET must be strong and at least 32 bytes in release mode")
		}
		if weakSecret(c.Security.MasterKey) {
			return fmt.Errorf("SRAPI_MASTER_KEY must be strong and at least 32 bytes in release mode")
		}
		if weakSecret(c.Security.TOTPEncryptionKey) {
			return fmt.Errorf("TOTP_ENCRYPTION_KEY must be strong and at least 32 bytes in release mode")
		}
		if weakSecret(c.Security.APIKeyPepper) {
			return fmt.Errorf("API_KEY_PEPPER must be strong and at least 32 bytes in release mode")
		}
		if c.Database.Password == "" || isWeakDevelopmentSecret(c.Database.Password) {
			return fmt.Errorf("DATABASE_PASSWORD must be set to a non-development value in release mode")
		}
		if weakBootstrapPassword(c.Bootstrap.AdminPassword) {
			return fmt.Errorf("BOOTSTRAP_ADMIN_PASSWORD must be changed to a non-development value in release mode")
		}
	}
	return nil
}

func (c Config) StorageBackend() string {
	return normalizeStorageBackend(c.Storage.Backend)
}

func (c Config) UsesMemoryStorage() bool {
	return c.StorageBackend() == StorageBackendMemory
}

func (d DependencyConfig) Address() string {
	return fmt.Sprintf("%s:%d", d.Host, d.Port)
}

func getEnv(key, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	return value
}

// parseStringMapEnv parses a JSON object env var into a string map. Returns an
// empty map when unset or unparseable so callers never see nil.
func parseStringMapEnv(key string) map[string]string {
	raw := strings.TrimSpace(os.Getenv(key))
	out := map[string]string{}
	if raw == "" {
		return out
	}
	parsed := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return out
	}
	for k, v := range parsed {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func securityConfigFromEnv() SecurityConfig {
	masterKey := getEnv("SRAPI_MASTER_KEY", "local_dev_master_key_32_bytes_minimum_change_me")
	return SecurityConfig{
		JWTSecret:         getEnv("JWT_SECRET", ""),
		MasterKey:         masterKey,
		TOTPEncryptionKey: getEnv("TOTP_ENCRYPTION_KEY", masterKey),
		APIKeyPepper:      getEnv("API_KEY_PEPPER", "local_dev_api_key_pepper_change_me_32+"),
	}
}

func normalizeStorageBackend(value string) string {
	backend := strings.ToLower(strings.TrimSpace(value))
	if backend == "" {
		return StorageBackendPostgres
	}
	return backend
}

func getIntEnv(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getFloatEnv(key string, fallback float64) float64 {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getBoolEnv(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func isWeakDevelopmentSecret(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return true
	}
	if strings.Contains(normalized, "local_dev") || strings.Contains(normalized, "change_me") || strings.Contains(normalized, "changeme") {
		return true
	}
	switch normalized {
	case "password", "postgres", "srapi", "srapi_dev_password_change_me":
		return true
	default:
		return false
	}
}

func weakSecret(value string) bool {
	return len(value) < 32 || isWeakDevelopmentSecret(value)
}

func weakBootstrapPassword(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if len(value) < 12 || isWeakDevelopmentSecret(value) {
		return true
	}
	switch normalized {
	case "password123", "admin", "admin123", "srapi_admin_password_change_me":
		return true
	default:
		return false
	}
}

func validHTTPBaseURL(value string) bool {
	normalized := strings.TrimSpace(value)
	if normalized == "" || strings.ContainsAny(normalized, "\r\n\t ") {
		return false
	}
	if strings.Contains(normalized, "?") || strings.Contains(normalized, "#") {
		return false
	}
	lower := strings.ToLower(normalized)
	return strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://")
}
