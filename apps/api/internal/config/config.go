package config

import (
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
	Server      ServerConfig
	Storage     StorageConfig
	Database    DependencyConfig
	Redis       DependencyConfig
	Gateway     GatewayConfig
	Security    SecurityConfig
	Bootstrap   BootstrapConfig
	Retention   RetentionConfig
	HealthProbe HealthProbeConfig
	QualityEval QualityEvalConfig
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
}

type SecurityConfig struct {
	JWTSecret    string
	MasterKey    string
	APIKeyPepper string
}

type BootstrapConfig struct {
	AdminEmail    string
	AdminPassword string
	AdminName     string
}

type RetentionConfig struct {
	UsageLogsDays              int
	SchedulerDecisionsDays     int
	SchedulerFeedbacksDays     int
	AuditLogsDays              int
	AccountHealthSnapshotsDays int
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
		},
		Security: SecurityConfig{
			JWTSecret:    getEnv("JWT_SECRET", ""),
			MasterKey:    getEnv("SRAPI_MASTER_KEY", "local_dev_master_key_32_bytes_minimum_change_me"),
			APIKeyPepper: getEnv("API_KEY_PEPPER", "local_dev_api_key_pepper_change_me_32+"),
		},
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
