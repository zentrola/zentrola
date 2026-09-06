// Package config 读取并校验系统启动配置，业务配置留在 PostgreSQL。
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Environment       string
	HTTPAddr          string
	ReadHeaderTimeout time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	StartupTimeout    time.Duration
	HealthTimeout     time.Duration
	LogLevel          slog.Level
	LogFormat         string
	Postgres          Postgres
	AutoMigrate       bool
	IDNode            int
	BootstrapSonnet   string
	BootstrapOpus     string
	CORS              CORS
	Security          Security
	Gateway           Gateway
	Usage             Usage
}

type Usage struct {
	QueueSize, BatchSize                         int
	FlushInterval, WriteTimeout, ShutdownTimeout time.Duration
}

type Gateway struct {
	MaxBodyBytes                                                 int64
	RequestTimeout, HeaderTimeout, BodyReadTimeout, WriteTimeout time.Duration
}

type Security struct {
	MasterKey             string
	ExternalMasterKeyPath string
	MasterKeyPath         string
	JWTSecret             string
	MaxLoginFailures      int64
	LoginLockDuration     time.Duration
}

type Postgres struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
	SSLMode  string
	MaxConns int32
}

type CORS struct {
	Enabled bool
	Origins []string
}

// Load 不修改进程环境，优先级为 System ENV > .env.{环境} > .env > 默认值。
// path 指向通用文件；环境文件在同一目录中，名称为 path + "." + 环境。
func Load(path string) (Config, error) {
	return load(path, os.LookupEnv)
}

func load(path string, systemLookup func(string) (string, bool)) (Config, error) {
	values, err := readOptionalEnv(path, ".env")
	if err != nil {
		return Config{}, err
	}
	environment, ok := systemLookup("APP_ENV")
	if !ok {
		environment, ok = values["APP_ENV"]
		if !ok {
			environment = "dev"
		}
	}
	environment, err = normalizeEnvironment(environment)
	if err != nil {
		return Config{}, err
	}
	overrides, err := readOptionalEnv(path+"."+environment, ".env."+environment)
	if err != nil {
		return Config{}, err
	}
	if _, ok := overrides["APP_ENV"]; ok {
		return Config{}, errors.New("APP_ENV must be set in system environment or .env, not in an environment-specific file")
	}
	for key, value := range overrides {
		values[key] = value
	}
	return parse(func(key string) (string, bool) {
		if key == "APP_ENV" {
			return environment, true
		}
		if value, ok := systemLookup(key); ok {
			return value, true
		}
		value, ok := values[key]
		return value, ok
	})
}

func readOptionalEnv(path, label string) (map[string]string, error) {
	values, err := godotenv.Read(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		// 解析错误可能含有配置原文，不能向日志传播。
		return nil, fmt.Errorf("cannot read or parse %s file", label)
	}
	return values, nil
}

func normalizeEnvironment(value string) (string, error) {
	switch value {
	case "dev", "development":
		return "dev", nil
	case "test":
		return "test", nil
	case "prod", "production":
		return "prod", nil
	default:
		return "", errors.New("APP_ENV must be dev, test or prod")
	}
}

func parse(lookup func(string) (string, bool)) (Config, error) {
	var problems []error
	get := func(key, fallback string) string {
		if value, ok := lookup(key); ok {
			return value
		}
		return fallback
	}
	duration := func(key, fallback string) time.Duration {
		value, err := time.ParseDuration(get(key, fallback))
		if err != nil || value <= 0 {
			problems = append(problems, fmt.Errorf("%s must be a positive duration", key))
		}
		return value
	}
	boolean := func(key, fallback string) bool {
		value, err := strconv.ParseBool(get(key, fallback))
		if err != nil {
			problems = append(problems, fmt.Errorf("%s must be a boolean", key))
		}
		return value
	}
	integer := func(key, fallback string, max int) int {
		value, err := strconv.Atoi(get(key, fallback))
		if err != nil || value < 1 || value > max {
			problems = append(problems, fmt.Errorf("%s must be between 1 and %d", key, max))
		}
		return value
	}
	cfg := Config{
		Environment:       get("APP_ENV", "dev"),
		HTTPAddr:          get("HTTP_ADDR", ":8080"),
		ReadHeaderTimeout: duration("HTTP_READ_HEADER_TIMEOUT", "5s"),
		IdleTimeout:       duration("HTTP_IDLE_TIMEOUT", "120s"),
		ShutdownTimeout:   duration("SHUTDOWN_TIMEOUT", "20s"),
		StartupTimeout:    duration("STARTUP_TIMEOUT", "30s"),
		HealthTimeout:     duration("HEALTH_CHECK_TIMEOUT", "2s"),
		LogFormat:         get("LOG_FORMAT", "text"),
		AutoMigrate:       boolean("MIGRATIONS_AUTO_APPLY", "true"),
		IDNode:            integer("ID_NODE", "1", 65535),
		BootstrapSonnet:   get("BOOTSTRAP_SONNET_MODEL", "claude-sonnet-5"),
		BootstrapOpus:     get("BOOTSTRAP_OPUS_MODEL", "claude-opus-5"),
		Postgres: Postgres{
			Host:     get("POSTGRES_HOST", "127.0.0.1"),
			Port:     integer("POSTGRES_PORT", "5432", 65535),
			Database: get("POSTGRES_DB", "zentrola"),
			User:     get("POSTGRES_USER", "postgres"),
			Password: get("POSTGRES_PASSWORD", ""),
			SSLMode:  get("POSTGRES_SSLMODE", "disable"),
			MaxConns: int32(integer("POSTGRES_MAX_CONNS", "10", 1000)),
		},
		CORS: CORS{Enabled: boolean("CORS_ENABLED", "false")},
		Usage: Usage{
			QueueSize: integer("USAGE_QUEUE_SIZE", "10000", 1000000), BatchSize: integer("USAGE_BATCH_SIZE", "100", 10000),
			FlushInterval: duration("USAGE_FLUSH_INTERVAL", "500ms"), WriteTimeout: duration("USAGE_WRITE_TIMEOUT", "3s"), ShutdownTimeout: duration("USAGE_SHUTDOWN_TIMEOUT", "5s"),
		},
		Gateway: Gateway{
			MaxBodyBytes:    int64(integer("GATEWAY_MAX_BODY_BYTES", "33554432", 128<<20)),
			RequestTimeout:  duration("GATEWAY_REQUEST_TIMEOUT", "15m"),
			HeaderTimeout:   duration("GATEWAY_HEADER_TIMEOUT", "120s"),
			BodyReadTimeout: duration("GATEWAY_BODY_READ_TIMEOUT", "30s"),
			WriteTimeout:    duration("GATEWAY_WRITE_TIMEOUT", "30s"),
		},
		Security: Security{
			MasterKey: get("ACP_MASTER_KEY", ""), ExternalMasterKeyPath: get("ACP_MASTER_KEY_FILE", ""), MasterKeyPath: get("MASTER_KEY_PATH", "data/secrets/master.key"),
			JWTSecret:        get("ADMIN_JWT_SECRET", ""),
			MaxLoginFailures: int64(integer("ADMIN_LOGIN_MAX_FAILURES", "5", 1000)), LoginLockDuration: duration("ADMIN_LOGIN_LOCK_DURATION", "15m"),
		},
	}
	if err := cfg.LogLevel.UnmarshalText([]byte(get("LOG_LEVEL", "info"))); err != nil {
		problems = append(problems, errors.New("LOG_LEVEL is invalid"))
	}
	if cfg.LogFormat != "text" && cfg.LogFormat != "json" {
		problems = append(problems, errors.New("LOG_FORMAT must be text or json"))
	}
	if environment, err := normalizeEnvironment(cfg.Environment); err != nil {
		problems = append(problems, err)
	} else {
		cfg.Environment = environment
	}
	_, port, err := net.SplitHostPort(cfg.HTTPAddr)
	if err != nil {
		problems = append(problems, errors.New("HTTP_ADDR must be host:port"))
	} else if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		problems = append(problems, errors.New("HTTP_ADDR port is invalid"))
	}
	for key, value := range map[string]string{
		"POSTGRES_HOST": cfg.Postgres.Host, "POSTGRES_DB": cfg.Postgres.Database,
		"POSTGRES_USER": cfg.Postgres.User, "POSTGRES_PASSWORD": cfg.Postgres.Password,
	} {
		if strings.TrimSpace(value) == "" {
			problems = append(problems, fmt.Errorf("%s is required", key))
		}
	}
	for key, value := range map[string]string{"BOOTSTRAP_SONNET_MODEL": cfg.BootstrapSonnet, "BOOTSTRAP_OPUS_MODEL": cfg.BootstrapOpus} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, " \t\r\n") || len(value) > 128 {
			problems = append(problems, fmt.Errorf("%s must be a nonempty model code of at most 128 bytes", key))
		}
	}
	switch cfg.Postgres.SSLMode {
	case "disable", "require", "verify-ca", "verify-full":
	default:
		problems = append(problems, errors.New("POSTGRES_SSLMODE is invalid"))
	}
	for _, raw := range strings.Split(get("CORS_ALLOWED_ORIGINS", ""), ",") {
		origin := strings.TrimSpace(raw)
		if origin == "" {
			continue
		}
		u, err := url.Parse(origin)
		if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" ||
			u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || strings.Contains(origin, "*") {
			problems = append(problems, errors.New("CORS_ALLOWED_ORIGINS must contain explicit HTTP origins without paths or wildcards"))
			continue
		}
		cfg.CORS.Origins = append(cfg.CORS.Origins, origin)
	}
	if cfg.CORS.Enabled && len(cfg.CORS.Origins) == 0 {
		problems = append(problems, errors.New("CORS_ALLOWED_ORIGINS is required when CORS is enabled"))
	}
	return cfg, errors.Join(problems...)
}
