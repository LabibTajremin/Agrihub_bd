// Package config is the single typed configuration surface. Precedence:
// struct defaults → YAML file → AGRI_* environment → -set flags. The result is
// validated once at boot and never mutated afterwards.
package config

import "time"

// Config is the complete application configuration.
type Config struct {
	App           App           `yaml:"app"`
	HTTP          HTTP          `yaml:"http"`
	Database      Database      `yaml:"database"`
	Redis         Redis         `yaml:"redis"`
	Auth          Auth          `yaml:"auth"`
	Storage       Storage       `yaml:"storage"`
	Localization  Localization  `yaml:"localization"`
	AI            AI            `yaml:"ai"`
	Advisory      Advisory      `yaml:"advisory"`
	Weather       Weather       `yaml:"weather"`
	Observability Observability `yaml:"observability"`
	Features      Features      `yaml:"features"`
}

// App holds process identity.
type App struct {
	Name    string `yaml:"name" default:"agrismart" validate:"required"`
	Env     string `yaml:"env" default:"development" validate:"oneof=development test production"`
	BaseURL string `yaml:"base_url" default:"http://localhost:8080" validate:"url"`
}

// HTTP configures the server and middleware chain.
type HTTP struct {
	Addr                   string        `yaml:"addr" default:":8080" validate:"required"`
	ReadTimeout            time.Duration `yaml:"read_timeout" default:"10s" validate:"gt=0"`
	WriteTimeout           time.Duration `yaml:"write_timeout" default:"30s" validate:"gt=0"`
	IdleTimeout            time.Duration `yaml:"idle_timeout" default:"60s" validate:"gt=0"`
	RequestTimeout         time.Duration `yaml:"request_timeout" default:"15s" validate:"gt=0"`
	ShutdownTimeout        time.Duration `yaml:"shutdown_timeout" default:"20s" validate:"gt=0"`
	MaxBodyBytes           int64         `yaml:"max_body_bytes" default:"1048576" validate:"gt=0"`
	CORSAllowedOrigins     []string      `yaml:"cors_allowed_origins" default:"*" validate:"min=1"`
	TrustProxyHeaders      bool          `yaml:"trust_proxy_headers" default:"true"`
	RateLimitPerMinute     int           `yaml:"rate_limit_per_minute" default:"120" validate:"gt=0"`
	AuthRateLimitPerMinute int           `yaml:"auth_rate_limit_per_minute" default:"20" validate:"gt=0"`
}

// Database configures Postgres access.
type Database struct {
	URL                string        `yaml:"url" secret:"true" validate:"required"`
	PoolMode           string        `yaml:"pool_mode" default:"transaction" validate:"oneof=session transaction"`
	MaxConns           int           `yaml:"max_conns" default:"10" validate:"gt=0"`
	MinConns           int           `yaml:"min_conns" default:"0" validate:"gte=0"`
	ConnectTimeout     time.Duration `yaml:"connect_timeout" default:"5s" validate:"gt=0"`
	OutboxBatchSize    int           `yaml:"outbox_batch_size" default:"100" validate:"gt=0"`
	OutboxPollInterval time.Duration `yaml:"outbox_poll_interval" default:"2s" validate:"gt=0"`
}

// Redis is optional; an empty URL selects in-memory fallbacks.
type Redis struct {
	URL string `yaml:"url" secret:"true"`
}

// Auth configures OTP, tokens and password hashing.
type Auth struct {
	JWTSecret           string        `yaml:"jwt_secret" secret:"true" validate:"required,min=32"`
	JWTKeyID            string        `yaml:"jwt_key_id" default:"k1" validate:"required"`
	Issuer              string        `yaml:"issuer" default:"agrismart" validate:"required"`
	AccessTTL           time.Duration `yaml:"access_ttl" default:"15m" validate:"gt=0"`
	RefreshTTL          time.Duration `yaml:"refresh_ttl" default:"720h" validate:"gt=0"`
	OTPTTL              time.Duration `yaml:"otp_ttl" default:"5m" validate:"gt=0"`
	OTPLength           int           `yaml:"otp_length" default:"6" validate:"min=4,max=10"`
	OTPMaxAttempts      int           `yaml:"otp_max_attempts" default:"5" validate:"gt=0"`
	OTPPerNumberPerHour int           `yaml:"otp_per_number_per_hour" default:"5" validate:"gt=0"`
	OTPPerIPPerHour     int           `yaml:"otp_per_ip_per_hour" default:"20" validate:"gt=0"`
	Argon2Time          uint32        `yaml:"argon2_time" default:"1" validate:"gt=0"`
	Argon2MemoryKiB     uint32        `yaml:"argon2_memory_kib" default:"65536" validate:"gte=8"`
	Argon2Threads       uint8         `yaml:"argon2_threads" default:"2" validate:"gt=0"`
	Argon2KeyLen        uint32        `yaml:"argon2_key_len" default:"32" validate:"gte=16"`
}

// Storage selects and configures the media blob backend.
type Storage struct {
	Backend             string        `yaml:"backend" default:"local" validate:"oneof=local s3"`
	LocalDir            string        `yaml:"local_dir" default:"var/media" validate:"required"`
	LocalSigningKey     string        `yaml:"local_signing_key" secret:"true"`
	S3Endpoint          string        `yaml:"s3_endpoint" default:"localhost:9000"`
	S3Bucket            string        `yaml:"s3_bucket" default:"agrismart-media"`
	S3Region            string        `yaml:"s3_region" default:"us-east-1"`
	S3UseSSL            bool          `yaml:"s3_use_ssl" default:"false"`
	S3AccessKey         string        `yaml:"s3_access_key" secret:"true"`
	S3SecretKey         string        `yaml:"s3_secret_key" secret:"true"`
	PresignTTL          time.Duration `yaml:"presign_ttl" default:"15m" validate:"gt=0"`
	MaxUploadBytes      int64         `yaml:"max_upload_bytes" default:"8388608" validate:"gt=0"`
	AllowedContentTypes []string      `yaml:"allowed_content_types" default:"image/jpeg,image/png,audio/mpeg,audio/ogg" validate:"min=1"`
}

// Localization configures dictionary serving.
type Localization struct {
	DefaultLanguage string        `yaml:"default_language" default:"bn" validate:"required"`
	CacheMaxAge     time.Duration `yaml:"cache_max_age" default:"300s" validate:"gte=0"`
}

// AI selects the implementation behind the open-slot ports.
type AI struct {
	Provider               string  `yaml:"provider" default:"stub" validate:"required"` // validated against the registry at boot
	MinDiagnosisConfidence float64 `yaml:"min_diagnosis_confidence" default:"0.60" validate:"gte=0,lte=1"`
}

// Advisory holds the crop-suitability weights (must sum to 1.0).
type Advisory struct {
	WeightSoil   float64 `yaml:"weight_soil" default:"0.30" validate:"gte=0,lte=1"`
	WeightWater  float64 `yaml:"weight_water" default:"0.25" validate:"gte=0,lte=1"`
	WeightPest   float64 `yaml:"weight_pest" default:"0.15" validate:"gte=0,lte=1"`
	WeightMarket float64 `yaml:"weight_market" default:"0.20" validate:"gte=0,lte=1"`
	WeightSeed   float64 `yaml:"weight_seed" default:"0.10" validate:"gte=0,lte=1"`
	TopN         int     `yaml:"top_n" default:"5" validate:"gt=0,lte=20"`
}

// Weather configures the forecast provider and its circuit breaker.
type Weather struct {
	Provider                string        `yaml:"provider" default:"stub" validate:"oneof=stub http"`
	BaseURL                 string        `yaml:"base_url" default:"https://api.open-meteo.com" validate:"url"`
	HTTPTimeout             time.Duration `yaml:"http_timeout" default:"5s" validate:"gt=0"`
	StaleAfter              time.Duration `yaml:"stale_after" default:"3h" validate:"gt=0"`
	BreakerFailureThreshold int           `yaml:"breaker_failure_threshold" default:"3" validate:"gt=0"`
	BreakerOpenTimeout      time.Duration `yaml:"breaker_open_timeout" default:"30s" validate:"gt=0"`
	BreakerHalfOpenMaxCalls int           `yaml:"breaker_half_open_max_calls" default:"1" validate:"gt=0"`
}

// Observability configures logs and metrics.
type Observability struct {
	LogLevel       string `yaml:"log_level" default:"info" validate:"oneof=debug info warn error"`
	LogFormat      string `yaml:"log_format" default:"json" validate:"oneof=json text"`
	MetricsEnabled bool   `yaml:"metrics_enabled" default:"true"`
}

// Features toggles product behaviour.
type Features struct {
	GuestMode bool `yaml:"guest_mode" default:"true"`
	// ExposeOTP returns the OTP in the API response; development/test only.
	ExposeOTP bool `yaml:"expose_otp" default:"false"`
}
