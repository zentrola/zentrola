package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	tests := []struct{ name, key, value, want string }{
		{"wildcard origin", "CORS_ALLOWED_ORIGINS", "*", "CORS_ALLOWED_ORIGINS"},
		{"origin path", "CORS_ALLOWED_ORIGINS", "http://localhost:5173/path", "CORS_ALLOWED_ORIGINS"},
		{"unbounded shutdown", "SHUTDOWN_TIMEOUT", "0s", "SHUTDOWN_TIMEOUT"},
		{"invalid pool", "POSTGRES_MAX_CONNS", "0", "POSTGRES_MAX_CONNS"},
		{"invalid redis port", "REDIS_PORT", "0", "REDIS_PORT"},
		{"invalid redis database", "REDIS_DB", "-1", "REDIS_DB"},
		{"missing redis host", "REDIS_HOST", "", "REDIS_HOST"},
		{"invalid port", "HTTP_ADDR", ":99999", "HTTP_ADDR"},
		{"missing password", "POSTGRES_PASSWORD", "", "POSTGRES_PASSWORD"},
		{"unknown log format", "LOG_FORMAT", "xml", "LOG_FORMAT"},
		{"unknown console log format", "LOG_CONSOLE_FORMAT", "xml", "LOG_CONSOLE_FORMAT"},
		{"unknown log color", "LOG_COLOR", "sometimes", "LOG_COLOR"},
		{"invalid log file size", "LOG_FILE_MAX_SIZE_MB", "0", "LOG_FILE_MAX_SIZE_MB"},
		{"invalid log file backups", "LOG_FILE_MAX_BACKUPS", "0", "LOG_FILE_MAX_BACKUPS"},
		{"invalid boolean", "CORS_ENABLED", "maybe", "CORS_ENABLED"},
		{"unbounded gateway", "GATEWAY_REQUEST_TIMEOUT", "0s", "GATEWAY_REQUEST_TIMEOUT"},
		{"invalid identity cache TTL", "GATEWAY_IDENTITY_CACHE_TTL", "0s", "GATEWAY_IDENTITY_CACHE_TTL"},
		{"invalid route cache TTL", "GATEWAY_ROUTE_CACHE_TTL", "0s", "GATEWAY_ROUTE_CACHE_TTL"},
		{"invalid subscription refresh ahead", "CODEX_SUBSCRIPTION_REFRESH_AHEAD", "0s", "CODEX_SUBSCRIPTION_REFRESH_AHEAD"},
		{"invalid subscription refresh interval", "CODEX_SUBSCRIPTION_REFRESH_INTERVAL", "0s", "CODEX_SUBSCRIPTION_REFRESH_INTERVAL"},
		{"invalid subscription refresh timeout", "CODEX_SUBSCRIPTION_REFRESH_RUN_TIMEOUT", "0s", "CODEX_SUBSCRIPTION_REFRESH_RUN_TIMEOUT"},
		{"oversized body limit", "GATEWAY_MAX_BODY_BYTES", "2147483647", "GATEWAY_MAX_BODY_BYTES"},
		{"empty usage queue", "USAGE_QUEUE_SIZE", "0", "USAGE_QUEUE_SIZE"},
		{"oversized usage batch", "USAGE_BATCH_SIZE", "10001", "USAGE_BATCH_SIZE"},
		{"usage flush disabled", "USAGE_FLUSH_INTERVAL", "0s", "USAGE_FLUSH_INTERVAL"},
		{"unbounded usage write", "USAGE_WRITE_TIMEOUT", "0s", "USAGE_WRITE_TIMEOUT"},
		{"unbounded usage shutdown", "USAGE_SHUTDOWN_TIMEOUT", "0s", "USAGE_SHUTDOWN_TIMEOUT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := map[string]string{"POSTGRES_PASSWORD": "test-only", tt.key: tt.value}
			_, err := parse(func(key string) (string, bool) { value, ok := values[key]; return value, ok })
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %s error, got %v", tt.want, err)
			}
		})
	}
}

func TestSubscriptionRefreshDefaults(t *testing.T) {
	cfg, err := parse(func(key string) (string, bool) {
		if key == "POSTGRES_PASSWORD" {
			return "test-only", true
		}
		return "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Gateway.SubscriptionRefreshAhead != 30*time.Minute ||
		cfg.Gateway.SubscriptionRefreshInterval != time.Minute ||
		cfg.Gateway.SubscriptionRunTimeout != 4*time.Minute {
		t.Fatalf("unexpected subscription refresh defaults: %+v", cfg.Gateway)
	}
}

func TestLogConfiguration(t *testing.T) {
	t.Run("legacy format remains the console fallback", func(t *testing.T) {
		cfg, err := parse(func(key string) (string, bool) {
			values := map[string]string{"POSTGRES_PASSWORD": "test-only", "LOG_FORMAT": "json"}
			value, ok := values[key]
			return value, ok
		})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.LogConsoleFormat != "json" {
			t.Fatalf("got console format %q, want json", cfg.LogConsoleFormat)
		}
	})

	t.Run("independent console and file settings", func(t *testing.T) {
		cfg, err := parse(func(key string) (string, bool) {
			values := map[string]string{
				"POSTGRES_PASSWORD":    "test-only",
				"LOG_CONSOLE_FORMAT":   "pretty",
				"LOG_COLOR":            "always",
				"LOG_FILE_PATH":        " data/logs/zentrola.jsonl ",
				"LOG_FILE_MAX_SIZE_MB": "25",
				"LOG_FILE_MAX_BACKUPS": "7",
			}
			value, ok := values[key]
			return value, ok
		})
		if err != nil {
			t.Fatal(err)
		}
		if cfg.LogConsoleFormat != "pretty" || cfg.LogColor != "always" || cfg.LogFilePath != "data/logs/zentrola.jsonl" ||
			cfg.LogFileMaxSizeMB != 25 || cfg.LogFileMaxBackups != 7 {
			t.Fatalf("unexpected log configuration: %+v", cfg)
		}
	})
}

func TestLoadEnvironmentOverridesFile(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("POSTGRES_PASSWORD=file-secret\nHTTP_ADDR=:8001\nLOG_LEVEL=debug\nMASTER_KEY=file-master\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HTTP_ADDR", ":8002")
	t.Setenv("POSTGRES_PASSWORD", "env-secret")
	t.Setenv("MASTER_KEY", "environment-master")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8002" || cfg.Postgres.Password != "env-secret" || cfg.Security.MasterKey != "file-master" {
		t.Fatal("environment did not override file")
	}
	t.Setenv("POSTGRES_PASSWORD", "")
	if _, err := Load(path); err == nil {
		t.Fatal("empty environment secret must not fall back to file")
	}
}

func TestMasterKeyOnlyLoadsFromSelectedConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("APP_ENV=prod\nPOSTGRES_PASSWORD=test-only\nMASTER_KEY=common-master\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".prod", []byte("MASTER_KEY=overlay-master\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MASTER_KEY", "environment-master")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Security.MasterKey != "common-master" {
		t.Fatalf("got MASTER_KEY %q, want selected config file value", cfg.Security.MasterKey)
	}
}

func TestLoadLayeredConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		system      map[string]string
		environment string
		addr        string
		format      string
		wantError   string
	}{
		{
			name: "default dev without files", environment: "dev", addr: ":9527", format: "text",
		},
		{
			name:        "default dev loads overlay without common file",
			files:       map[string]string{"dev": "HTTP_ADDR=:8001\n"},
			environment: "dev", addr: ":8001",
		},
		{
			name: "common selects test and overlay adds and overrides",
			files: map[string]string{
				"":     "APP_ENV=test\nHTTP_ADDR=:8001\nLOG_LEVEL=debug\n",
				"test": "HTTP_ADDR=:8002\nLOG_FORMAT=json\n",
				"dev":  "HTTP_ADDR=:8003\n", "prod": "invalid=\"unterminated\n",
			},
			environment: "test", addr: ":8002", format: "json",
		},
		{
			name: "system selects prod and wins over both files",
			files: map[string]string{
				"":     "APP_ENV=dev\nHTTP_ADDR=:8001\n",
				"prod": "HTTP_ADDR=:8002\nLOG_FORMAT=json\n",
				"dev":  "invalid=\"unterminated\n",
			},
			system:      map[string]string{"APP_ENV": "prod", "HTTP_ADDR": ":8003"},
			environment: "prod", addr: ":8003", format: "json",
		},
		{
			name:        "missing overlay inherits common",
			files:       map[string]string{"": "APP_ENV=prod\nHTTP_ADDR=:8001\n"},
			environment: "prod", addr: ":8001",
		},
		{
			name:        "legacy development uses dev overlay",
			files:       map[string]string{"": "APP_ENV=development\n", "dev": "HTTP_ADDR=:8001\n"},
			environment: "dev", addr: ":8001",
		},
		{
			name:        "legacy production uses prod overlay",
			files:       map[string]string{"prod": "HTTP_ADDR=:8001\n"},
			system:      map[string]string{"APP_ENV": "production"},
			environment: "prod", addr: ":8001",
		},
		{
			name:      "empty overlay value does not inherit common",
			files:     map[string]string{"": "POSTGRES_HOST=localhost\n", "dev": "POSTGRES_HOST=\n"},
			wantError: "POSTGRES_HOST is required",
		},
		{
			name:   "empty system value does not inherit either file",
			files:  map[string]string{"": "POSTGRES_HOST=localhost\n", "dev": "POSTGRES_HOST=dev-db\n"},
			system: map[string]string{"POSTGRES_HOST": ""}, wantError: "POSTGRES_HOST is required",
		},
		{
			name:  "overlay cannot change environment",
			files: map[string]string{"dev": "APP_ENV=prod\n"}, wantError: "APP_ENV must be set in",
		},
		{
			name:  "overlay cannot redeclare same environment",
			files: map[string]string{"dev": "APP_ENV=dev\n"}, wantError: "APP_ENV must be set in",
		},
		{
			name:   "empty system environment cannot fall back",
			files:  map[string]string{"": "APP_ENV=prod\n"},
			system: map[string]string{"APP_ENV": ""}, wantError: "APP_ENV must be dev, test or prod",
		},
		{
			name:  "empty common environment cannot fall back",
			files: map[string]string{"": "APP_ENV=\n"}, wantError: "APP_ENV must be dev, test or prod",
		},
		{
			name:   "invalid environment rejected before file selection",
			system: map[string]string{"APP_ENV": "../../sensitive-input"}, wantError: "APP_ENV must be dev, test or prod",
		},
		{
			name:  "malformed common file",
			files: map[string]string{"": "SECRET=\"sensitive-input\n"}, wantError: "cannot read or parse .env file",
		},
		{
			name:  "malformed selected overlay",
			files: map[string]string{"dev": "SECRET=\"sensitive-input\n"}, wantError: "cannot read or parse .env.dev file",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".env")
			for suffix, content := range tt.files {
				file := path
				if suffix != "" {
					file += "." + suffix
				}
				if err := os.WriteFile(file, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := load(path, func(key string) (string, bool) {
				if value, ok := tt.system[key]; ok {
					return value, true
				}
				if key == "POSTGRES_PASSWORD" {
					return "test-only", true
				}
				return "", false
			})
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected %s, got %v", tt.wantError, err)
				}
				if strings.Contains(err.Error(), "sensitive-input") {
					t.Fatal("config error exposed input")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Environment != tt.environment || cfg.HTTPAddr != tt.addr {
				t.Fatalf("got environment=%s address=%s, want %s %s", cfg.Environment, cfg.HTTPAddr, tt.environment, tt.addr)
			}
			if cfg.Gateway.Development != (tt.environment == "dev") {
				t.Fatalf("gateway development logging mismatch for %s", tt.environment)
			}
			if tt.format != "" && cfg.LogFormat != tt.format {
				t.Fatalf("got log format %s, want %s", cfg.LogFormat, tt.format)
			}
			if cfg.Postgres.MaxConns != 10 {
				t.Fatal("missing settings must retain code defaults")
			}
			if cfg.Redis.Host != "127.0.0.1" || cfg.Redis.Port != 6379 || cfg.Redis.Database != 2 || cfg.Redis.Password != "" {
				t.Fatalf("unexpected Redis defaults: %+v", cfg.Redis)
			}
		})
	}
}

func TestRedisConfiguration(t *testing.T) {
	values := map[string]string{
		"POSTGRES_PASSWORD": "test-only",
		"REDIS_HOST":        "redis.internal",
		"REDIS_PORT":        "6380",
		"REDIS_DB":          "2",
		"REDIS_PASSWORD":    "redis-secret",
	}
	cfg, err := parse(func(key string) (string, bool) { value, ok := values[key]; return value, ok })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Redis.Host != "redis.internal" || cfg.Redis.Port != 6380 || cfg.Redis.Database != 2 || cfg.Redis.Password != "redis-secret" {
		t.Fatalf("unexpected Redis configuration: %+v", cfg.Redis)
	}
}

func TestLoadUnreadableConfiguration(t *testing.T) {
	for _, suffix := range []string{"", ".dev"} {
		t.Run("directory instead of file"+suffix, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".env")
			if err := os.Mkdir(path+suffix, 0700); err != nil {
				t.Fatal(err)
			}
			_, err := load(path, func(string) (string, bool) { return "", false })
			if err == nil || !strings.Contains(err.Error(), "cannot read or parse .env"+suffix+" file") {
				t.Fatalf("expected unreadable configuration error, got %v", err)
			}
		})
	}
}

func TestLoadDoesNotModifyProcessEnvironment(t *testing.T) {
	const key = "ZENTROLA_CONFIG_TEST_MARKER"
	t.Setenv("APP_ENV", "dev")
	t.Setenv("POSTGRES_PASSWORD", "test-only")
	t.Setenv(key, "process-value")
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(key+"=common-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".dev", []byte(key+"=overlay-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatal(err)
	}
	if os.Getenv(key) != "process-value" {
		t.Fatal("loading files must not modify the process environment")
	}
}

func TestConfigErrorsDoNotEchoValues(t *testing.T) {
	_, err := parse(func(key string) (string, bool) {
		if key == "LOG_LEVEL" || key == "POSTGRES_PORT" {
			return "sensitive-input", true
		}
		return "", false
	})
	if err == nil || strings.Contains(err.Error(), "sensitive-input") {
		t.Fatalf("unsafe config error: %v", err)
	}
}
