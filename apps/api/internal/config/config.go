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
	Server               ServerConfig
	Storage              StorageConfig
	Database             DependencyConfig
	Redis                DependencyConfig
	Gateway              GatewayConfig
	Security             SecurityConfig
	Bootstrap            BootstrapConfig
	Retention            RetentionConfig
	AuthCleanup          AuthCleanupConfig
	BalanceCharger       BalanceChargerConfig
	HealthProbe          HealthProbeConfig
	QualityEval          QualityEvalConfig
	SLOEvaluator         SLOEvaluatorConfig
	AlertNotifications   AlertNotificationsConfig
	Email                EmailConfig
	Observability        ObservabilityConfig
	Captcha              CaptchaConfig
	QuotaRefresh         QuotaRefreshConfig
	AccountsTokenRefresh AccountsTokenRefreshConfig
	LiteLLMPricing       LiteLLMPricingConfig
	ConnectivityTest     ConnectivityTestConfig
	ScheduledTest        ScheduledTestConfig
	ProxyProbe           ProxyProbeConfig
	OAuth                OAuthConfig
	// Codex carries the global Codex provider modes ported from CLIProxyAPI:
	// the per-channel OAuth model-name alias map and the global
	// disable-image-generation enum. Both are no-ops when unset, which is the
	// default behaviour.
	Codex CodexConfig
}

// CodexConfig holds global Codex provider modes ported verbatim from
// CLIProxyAPI's config.OAuthModelAlias and config.DisableImageGenerationMode.
//
// ModelAlias is a nested map keyed by upstream OAuth channel (e.g. "openai"),
// then by the canonical client-visible model name. The value is the upstream
// alias the request should be rewritten to before being sent to the provider.
// Exact-match only — no glob/wildcard support, mirroring the CLIProxyAPI
// behaviour. Channel keys are lower-cased; canonical names are matched
// case-sensitively (the upstream model registry is case-sensitive).
//
// DisableImageGeneration is a three-state enum that controls whether the
// hosted `image_generation` tool is allowed on Codex non-images endpoints:
//   - "never"  (default): pass through; the gateway makes no decision.
//   - "always": always reject any request that ships an `image_generation`
//     tool in its tools array — the gateway short-circuits with a
//     ProviderError class="image_generation_disabled".
//   - "auto":  reject only when the inbound User-Agent looks like a Codex
//     CLI version known to mis-route the tool. The matcher is the same
//     case-insensitive regex CLIProxyAPI uses (see CodexDisableImageGenAutoUARegex).
type CodexConfig struct {
	ModelAlias             map[string]map[string]string
	DisableImageGeneration string
}

// ProxyProbeConfig controls the proxy availability probe worker, which dials
// each active proxy through to a known probe URL on a fixed interval and
// folds the outcome into the proxy's rolling 7-day success/failure counters.
// Disabled by default; producers opt in via PROXY_PROBE_ENABLED=true so an
// unattended deployment does not start hitting outbound URLs without explicit
// consent.
type ProxyProbeConfig struct {
	Enabled       bool
	Interval      time.Duration
	Timeout       time.Duration
	MaxConcurrent int
	ProbeURL      string
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

// AccountsTokenRefreshConfig controls the proactive OAuth access-token
// refresh worker. The worker only acts on accounts whose runtime_class is
// oauth_refresh or oauth_device_code, whose status is active, whose
// needs_reauth_at is nil, and whose token_expires_at falls inside the
// RefreshThreshold window. Enabled by default — the upstream cost per pass
// is one refresh request per due account, and silently letting tokens
// expire is exactly the failure mode this worker exists to prevent.
type AccountsTokenRefreshConfig struct {
	Enabled          bool
	Interval         time.Duration
	RefreshThreshold time.Duration
	Timeout          time.Duration
	MaxConcurrent    int
}

// LiteLLMPricingConfig controls optional remote LiteLLM price-list sync.
// It is disabled until SourceURL is set.
type LiteLLMPricingConfig struct {
	SourceURL string
	Interval  time.Duration
	Timeout   time.Duration
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

// ScheduledTestConfig controls the scheduled-test-plan worker, which evaluates
// admin-managed plans on a fixed tick and runs each plan's real generative
// probe. Disabled by default; plans still run only when this worker is enabled.
type ScheduledTestConfig struct {
	Enabled bool
	Tick    time.Duration
	Timeout time.Duration
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
	// Connection-pool tuning (database only; zero = driver default).
	MaxOpenConns           int
	MaxIdleConns           int
	ConnMaxLifetimeSeconds int
	// Redis connection-pool and timeout tuning (zero = go-redis default).
	PoolSize            int
	MinIdleConns        int
	DialTimeoutSeconds  int
	ReadTimeoutSeconds  int
	WriteTimeoutSeconds int
	PoolTimeoutSeconds  int
}

type GatewayConfig struct {
	MaxBodySize                  int64
	RequestTimeout               time.Duration
	StreamIdleTimeout            time.Duration
	StreamKeepaliveInterval      time.Duration
	ImageStreamIdleTimeout       time.Duration
	ImageStreamKeepaliveInterval time.Duration
	RealtimeMaxOpenSlots         int
	RealtimeMaxOpenSlotsPerKey   int
	// RequirePositiveBalance, when true, synchronously rejects balance-billed
	// gateway requests (pay-go users, or allowance-mode subscription overage)
	// from users whose balance no longer covers the request, closing the
	// deferred-charging overspend window. It never blocks hard_cap subscription
	// users (who never bill to balance). The code default is false, but the
	// production deploy (deploy/docker-compose.yml) sets
	// GATEWAY_REQUIRE_POSITIVE_BALANCE=true — a paid gateway must not let a
	// zero-balance user draw down real upstreams.
	RequirePositiveBalance bool
	// UsageMaxConcurrency bounds how many gateway usage/billing writes run
	// asynchronously off the request critical path at once. Above this the
	// caller processes inline (backpressure, never dropped). 0 disables async
	// processing entirely, restoring fully-synchronous in-request billing.
	UsageMaxConcurrency int
}

type SecurityConfig struct {
	JWTSecret         string
	MasterKey         string
	TOTPEncryptionKey string
	APIKeyPepper      string
	PasswordHashCost  int
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
	SiteKey   string // public site key the frontend widget renders (safe to expose)
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
	BatchLimit                 int
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

// AlertNotificationsConfig controls SRapi-native Ops alert delivery.
type AlertNotificationsConfig struct {
	Interval   time.Duration
	Timeout    time.Duration
	BatchLimit int
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
			Host:                   getEnv("DATABASE_HOST", "localhost"),
			Port:                   getIntEnv("DATABASE_PORT", 5432),
			User:                   getEnv("DATABASE_USER", "srapi"),
			Password:               getEnv("DATABASE_PASSWORD", ""),
			Database:               getEnv("DATABASE_DBNAME", "srapi"),
			SSLMode:                getEnv("DATABASE_SSLMODE", "disable"),
			MaxOpenConns:           getIntEnv("DATABASE_MAX_OPEN_CONNS", 50),
			MaxIdleConns:           getIntEnv("DATABASE_MAX_IDLE_CONNS", 20),
			ConnMaxLifetimeSeconds: getIntEnv("DATABASE_CONN_MAX_LIFETIME_SECONDS", 1800),
		},
		Redis: DependencyConfig{
			Host:                getEnv("REDIS_HOST", "localhost"),
			Port:                getIntEnv("REDIS_PORT", 6379),
			Password:            getEnv("REDIS_PASSWORD", ""),
			Database:            getEnv("REDIS_DB", "0"),
			PoolSize:            getIntEnv("REDIS_POOL_SIZE", 32),
			MinIdleConns:        getIntEnv("REDIS_MIN_IDLE_CONNS", 4),
			DialTimeoutSeconds:  getIntEnv("REDIS_DIAL_TIMEOUT_SECONDS", 3),
			ReadTimeoutSeconds:  getIntEnv("REDIS_READ_TIMEOUT_SECONDS", 2),
			WriteTimeoutSeconds: getIntEnv("REDIS_WRITE_TIMEOUT_SECONDS", 2),
			PoolTimeoutSeconds:  getIntEnv("REDIS_POOL_TIMEOUT_SECONDS", 3),
		},
		Gateway: GatewayConfig{
			MaxBodySize:                  int64(getIntEnv("GATEWAY_MAX_BODY_SIZE", defaultGatewayBodySize)),
			RequestTimeout:               time.Duration(getIntEnv("GATEWAY_REQUEST_TIMEOUT_SECONDS", 600)) * time.Second,
			StreamIdleTimeout:            time.Duration(getIntEnv("GATEWAY_STREAM_IDLE_TIMEOUT_SECONDS", 120)) * time.Second,
			StreamKeepaliveInterval:      time.Duration(getIntEnv("GATEWAY_STREAM_KEEPALIVE_INTERVAL_SECONDS", 10)) * time.Second,
			ImageStreamIdleTimeout:       time.Duration(getIntEnv("GATEWAY_IMAGE_STREAM_IDLE_TIMEOUT_SECONDS", 900)) * time.Second,
			ImageStreamKeepaliveInterval: time.Duration(getIntEnv("GATEWAY_IMAGE_STREAM_KEEPALIVE_INTERVAL_SECONDS", 10)) * time.Second,
			RealtimeMaxOpenSlots:         getIntEnv("GATEWAY_REALTIME_MAX_OPEN_SLOTS", 0),
			RealtimeMaxOpenSlotsPerKey:   getIntEnv("GATEWAY_REALTIME_MAX_OPEN_SLOTS_PER_API_KEY", 0),
			RequirePositiveBalance:       getBoolEnv("GATEWAY_REQUIRE_POSITIVE_BALANCE", false),
			UsageMaxConcurrency:          getIntEnv("GATEWAY_USAGE_MAX_CONCURRENCY", 32),
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
			BatchLimit:                 getIntEnv("DATA_RETENTION_BATCH_LIMIT", 1000),
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
		AlertNotifications: AlertNotificationsConfig{
			Interval:   time.Duration(getIntEnv("OPS_ALERT_NOTIFICATIONS_INTERVAL_SECONDS", 30)) * time.Second,
			Timeout:    time.Duration(getIntEnv("OPS_ALERT_NOTIFICATIONS_TIMEOUT_SECONDS", 30)) * time.Second,
			BatchLimit: getIntEnv("OPS_ALERT_NOTIFICATIONS_BATCH_LIMIT", 20),
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
			SiteKey:   getEnv("CAPTCHA_SITE_KEY", ""),
			VerifyURL: getEnv("CAPTCHA_VERIFY_URL", ""),
		},
		QuotaRefresh: QuotaRefreshConfig{
			Enabled:       getBoolEnv("ACCOUNT_QUOTA_REFRESH_ENABLED", false),
			Interval:      time.Duration(getIntEnv("ACCOUNT_QUOTA_REFRESH_INTERVAL_SECONDS", 1800)) * time.Second,
			Timeout:       time.Duration(getIntEnv("ACCOUNT_QUOTA_REFRESH_TIMEOUT_SECONDS", 15)) * time.Second,
			MaxConcurrent: getIntEnv("ACCOUNT_QUOTA_REFRESH_MAX_CONCURRENT", 4),
		},
		AccountsTokenRefresh: AccountsTokenRefreshConfig{
			Enabled:          getBoolEnv("ACCOUNTS_TOKEN_REFRESH_ENABLED", true),
			Interval:         time.Duration(getIntEnv("ACCOUNTS_TOKEN_REFRESH_INTERVAL_SECONDS", 300)) * time.Second,
			RefreshThreshold: time.Duration(getIntEnv("ACCOUNTS_TOKEN_REFRESH_THRESHOLD_SECONDS", 300)) * time.Second,
			Timeout:          time.Duration(getIntEnv("ACCOUNTS_TOKEN_REFRESH_TIMEOUT_SECONDS", 30)) * time.Second,
			MaxConcurrent:    getIntEnv("ACCOUNTS_TOKEN_REFRESH_MAX_CONCURRENT", 4),
		},
		LiteLLMPricing: LiteLLMPricingConfig{
			SourceURL: getEnv("LITELLM_PRICING_SOURCE_URL", ""),
			Interval:  time.Duration(getIntEnv("LITELLM_PRICING_INTERVAL_SECONDS", 43200)) * time.Second,
			Timeout:   time.Duration(getIntEnv("LITELLM_PRICING_TIMEOUT_SECONDS", 15)) * time.Second,
		},
		ConnectivityTest: ConnectivityTestConfig{
			Enabled:       getBoolEnv("ACCOUNT_CONNECTIVITY_TEST_ENABLED", false),
			Interval:      time.Duration(getIntEnv("ACCOUNT_CONNECTIVITY_TEST_INTERVAL_SECONDS", 3600)) * time.Second,
			Timeout:       time.Duration(getIntEnv("ACCOUNT_CONNECTIVITY_TEST_TIMEOUT_SECONDS", 30)) * time.Second,
			MaxConcurrent: getIntEnv("ACCOUNT_CONNECTIVITY_TEST_MAX_CONCURRENT", 2),
		},
		ScheduledTest: ScheduledTestConfig{
			Enabled: getBoolEnv("ACCOUNT_SCHEDULED_TEST_ENABLED", false),
			Tick:    time.Duration(getIntEnv("ACCOUNT_SCHEDULED_TEST_TICK_SECONDS", 60)) * time.Second,
			Timeout: time.Duration(getIntEnv("ACCOUNT_SCHEDULED_TEST_TIMEOUT_SECONDS", 30)) * time.Second,
		},
		ProxyProbe: ProxyProbeConfig{
			Enabled:       getBoolEnv("PROXY_PROBE_ENABLED", false),
			Interval:      time.Duration(getIntEnv("PROXY_PROBE_INTERVAL_SECONDS", 21600)) * time.Second,
			Timeout:       time.Duration(getIntEnv("PROXY_PROBE_TIMEOUT_SECONDS", 8)) * time.Second,
			MaxConcurrent: getIntEnv("PROXY_PROBE_MAX_CONCURRENT", 4),
			ProbeURL:      getEnv("PROXY_PROBE_URL", ""),
		},
		OAuth: OAuthConfig{
			ClientSecrets: parseStringMapEnv("OAUTH_CLIENT_SECRETS_JSON"),
			Issuers:       parseStringMapEnv("OAUTH_ISSUERS_JSON"),
		},
		Codex: CodexConfig{
			ModelAlias:             parseNestedStringMapEnv("CODEX_MODEL_ALIAS_JSON"),
			DisableImageGeneration: normalizeCodexDisableImageGenMode(getEnv("CODEX_DISABLE_IMAGE_GENERATION", "never")),
		},
	}
}

// parseNestedStringMapEnv parses a JSON object of objects env var
// ({"channel":{"canonical":"alias"}}) into a nested string map. The outer
// channel keys are lower-cased and trimmed; inner keys are trimmed but kept
// case-sensitive (upstream model IDs are case-sensitive). Returns an empty
// map for any unset / malformed input so callers never see nil.
func parseNestedStringMapEnv(key string) map[string]map[string]string {
	out := map[string]map[string]string{}
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return out
	}
	parsed := map[string]map[string]string{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return out
	}
	for channel, aliases := range parsed {
		channel = strings.ToLower(strings.TrimSpace(channel))
		if channel == "" || len(aliases) == 0 {
			continue
		}
		clean := map[string]string{}
		for canonical, alias := range aliases {
			canonical = strings.TrimSpace(canonical)
			alias = strings.TrimSpace(alias)
			if canonical == "" || alias == "" {
				continue
			}
			clean[canonical] = alias
		}
		if len(clean) > 0 {
			out[channel] = clean
		}
	}
	return out
}

// normalizeCodexDisableImageGenMode coerces an env-supplied value into the
// canonical three-state enum {"never","always","auto"}. Unknown values fall
// back to "never" — the gateway must fail safe (allow), never block on
// misconfiguration.
func normalizeCodexDisableImageGenMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "always":
		return "always"
	case "auto":
		return "auto"
	default:
		return "never"
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
	if c.Gateway.StreamKeepaliveInterval < 0 {
		return fmt.Errorf("GATEWAY_STREAM_KEEPALIVE_INTERVAL_SECONDS must be zero or positive")
	}
	if c.Gateway.ImageStreamIdleTimeout <= 0 {
		return fmt.Errorf("GATEWAY_IMAGE_STREAM_IDLE_TIMEOUT_SECONDS must be positive")
	}
	if c.Gateway.ImageStreamKeepaliveInterval < 0 {
		return fmt.Errorf("GATEWAY_IMAGE_STREAM_KEEPALIVE_INTERVAL_SECONDS must be zero or positive")
	}
	if c.Gateway.RealtimeMaxOpenSlots < 0 {
		return fmt.Errorf("GATEWAY_REALTIME_MAX_OPEN_SLOTS must be zero or positive")
	}
	if c.Gateway.RealtimeMaxOpenSlotsPerKey < 0 {
		return fmt.Errorf("GATEWAY_REALTIME_MAX_OPEN_SLOTS_PER_API_KEY must be zero or positive")
	}
	if c.Redis.PoolSize <= 0 {
		return fmt.Errorf("REDIS_POOL_SIZE must be positive")
	}
	if c.Redis.MinIdleConns < 0 {
		return fmt.Errorf("REDIS_MIN_IDLE_CONNS must be zero or positive")
	}
	if c.Redis.MinIdleConns > c.Redis.PoolSize {
		return fmt.Errorf("REDIS_MIN_IDLE_CONNS must be less than or equal to REDIS_POOL_SIZE")
	}
	if c.Redis.DialTimeoutSeconds <= 0 {
		return fmt.Errorf("REDIS_DIAL_TIMEOUT_SECONDS must be positive")
	}
	if c.Redis.ReadTimeoutSeconds <= 0 {
		return fmt.Errorf("REDIS_READ_TIMEOUT_SECONDS must be positive")
	}
	if c.Redis.WriteTimeoutSeconds <= 0 {
		return fmt.Errorf("REDIS_WRITE_TIMEOUT_SECONDS must be positive")
	}
	if c.Redis.PoolTimeoutSeconds <= 0 {
		return fmt.Errorf("REDIS_POOL_TIMEOUT_SECONDS must be positive")
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
	if c.Retention.BatchLimit <= 0 || c.Retention.BatchLimit > 5000 {
		return fmt.Errorf("DATA_RETENTION_BATCH_LIMIT must be between 1 and 5000")
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
	if c.AlertNotifications.Interval <= 0 {
		return fmt.Errorf("OPS_ALERT_NOTIFICATIONS_INTERVAL_SECONDS must be positive")
	}
	if c.AlertNotifications.Timeout <= 0 {
		return fmt.Errorf("OPS_ALERT_NOTIFICATIONS_TIMEOUT_SECONDS must be positive")
	}
	if c.AlertNotifications.BatchLimit <= 0 {
		return fmt.Errorf("OPS_ALERT_NOTIFICATIONS_BATCH_LIMIT must be positive")
	}
	if c.LiteLLMPricing.Interval <= 0 {
		return fmt.Errorf("LITELLM_PRICING_INTERVAL_SECONDS must be positive")
	}
	if c.LiteLLMPricing.Timeout <= 0 {
		return fmt.Errorf("LITELLM_PRICING_TIMEOUT_SECONDS must be positive")
	}
	if strings.TrimSpace(c.LiteLLMPricing.SourceURL) != "" && !validHTTPBaseURL(c.LiteLLMPricing.SourceURL) {
		return fmt.Errorf("LITELLM_PRICING_SOURCE_URL must be an absolute http or https URL without query or fragment")
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
		if c.Security.PasswordHashCost < 12 {
			return fmt.Errorf("PASSWORD_HASH_COST must be at least 12 in release mode")
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
		PasswordHashCost:  getIntEnv("PASSWORD_HASH_COST", 12),
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
