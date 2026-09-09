package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/zentrola/zentrola/internal/infrastructure/logging"
)

type webConfig struct {
	Address        string
	APIBaseURL     string
	APIOrigin      string
	GatewayBaseURL string
	LogFormat      string
	LogColor       string
}

type webOptions struct {
	APIBaseURL     string
	GatewayBaseURL string
	Help           bool
}

func parseWebOptions(args []string) (webOptions, error) {
	var options webOptions
	flags := flag.NewFlagSet("zentrola-web", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&options.APIBaseURL, "api", "", "Zentrola Backend address")
	flags.StringVar(&options.GatewayBaseURL, "gateway", "", "public Gateway address")
	flags.BoolVar(&options.Help, "help", false, "show help")
	flags.BoolVar(&options.Help, "h", false, "show help")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return webOptions{}, errors.New("参数无效，请执行 zentrola-web --help 查看用法")
	}
	return options, nil
}

func writeHelp(output io.Writer) {
	_, _ = fmt.Fprintln(output, `用法：zentrola-web [--api <地址>] [--gateway <地址>]

选项：
  --api <地址>       覆盖管理 API 地址；未指定 --gateway 时也作为 Gateway 地址
  --gateway <地址>   单独覆盖首页展示给客户端的 Gateway 公网地址
  -h, --help         查看帮助

命令行参数优先于同目录 .env；只传 --api 时，即使没有 .env 也可使用默认端口 9528 启动。`)
}

func displayURL(address string) string {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "http://" + address
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

func loadConfig(file string, options webOptions) (webConfig, error) {
	values, err := godotenv.Read(file)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) || strings.TrimSpace(options.APIBaseURL) == "" {
			return webConfig{}, fmt.Errorf("无法读取配置文件 %s", file)
		}
		values = map[string]string{}
	}
	get := func(key, fallback string) string {
		if value, ok := os.LookupEnv(key); ok {
			return strings.TrimSpace(value)
		}
		if value, ok := values[key]; ok {
			return strings.TrimSpace(value)
		}
		return fallback
	}
	address := get("WEB_ADDR", ":9528")
	if _, _, err := net.SplitHostPort(address); err != nil {
		return webConfig{}, errors.New("WEB_ADDR 必须是有效的监听地址，例如 :9528")
	}
	rawBase := strings.TrimRight(get("WEB_API_BASE_URL", ""), "/")
	if strings.TrimSpace(options.APIBaseURL) != "" {
		rawBase = strings.TrimRight(strings.TrimSpace(options.APIBaseURL), "/")
	}
	parsed, err := url.Parse(rawBase)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return webConfig{}, errors.New("WEB_API_BASE_URL 必须是有效的 HTTP(S) 地址，且不能包含账号、查询参数或 Fragment")
	}
	rawGateway := strings.TrimRight(get("WEB_GATEWAY_BASE_URL", rawBase), "/")
	if strings.TrimSpace(options.GatewayBaseURL) != "" {
		rawGateway = strings.TrimRight(strings.TrimSpace(options.GatewayBaseURL), "/")
	} else if strings.TrimSpace(options.APIBaseURL) != "" {
		rawGateway = rawBase
	}
	gateway, err := url.Parse(rawGateway)
	if err != nil || (gateway.Scheme != "http" && gateway.Scheme != "https") || gateway.Host == "" ||
		gateway.User != nil || gateway.RawQuery != "" || gateway.Fragment != "" {
		return webConfig{}, errors.New("WEB_GATEWAY_BASE_URL 必须是有效的 HTTP(S) 地址，且不能包含账号、查询参数或 Fragment")
	}
	logFormat, logColor := get("LOG_CONSOLE_FORMAT", "pretty"), get("LOG_COLOR", "auto")
	if logFormat != "pretty" && logFormat != "text" && logFormat != "json" {
		return webConfig{}, errors.New("LOG_CONSOLE_FORMAT must be text, pretty or json")
	}
	if logColor != "auto" && logColor != "always" && logColor != "never" && logColor != "true" && logColor != "false" {
		return webConfig{}, errors.New("LOG_COLOR must be auto, always, never, true or false")
	}
	return webConfig{
		Address:        address,
		APIBaseURL:     rawBase,
		APIOrigin:      parsed.Scheme + "://" + parsed.Host,
		GatewayBaseURL: rawGateway,
		LogFormat:      logFormat,
		LogColor:       logColor,
	}, nil
}

func securityHeaders(apiOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self' "+apiOrigin+"; font-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func staticFile(root fs.FS, name string, immutable bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := fs.ReadFile(root, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if kind := mime.TypeByExtension(path.Ext(name)); kind != "" {
			w.Header().Set("Content-Type", kind)
		}
		if immutable {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		_, _ = w.Write(body)
	}
}

func handler(config webConfig, root fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", staticFile(root, "index.html", false))
	mux.HandleFunc("GET /index.html", staticFile(root, "index.html", false))
	mux.HandleFunc("GET /favicon.svg", staticFile(root, "favicon.svg", false))
	mux.HandleFunc("GET /assets/{name...}", func(w http.ResponseWriter, r *http.Request) {
		name := "assets/" + r.PathValue("name")
		if !fs.ValidPath(name) || name == "assets/" {
			http.NotFound(w, r)
			return
		}
		staticFile(root, name, true)(w, r)
	})
	mux.HandleFunc("GET /config.js", func(w http.ResponseWriter, _ *http.Request) {
		data, _ := json.Marshal(map[string]string{
			"apiBaseUrl":     config.APIBaseURL,
			"gatewayBaseUrl": config.GatewayBaseURL,
		})
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = fmt.Fprintf(w, "window.__ZENTROLA_CONFIG__ = %s;\n", data)
	})
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(`{"status":"LIVE"}`))
	})
	return securityHeaders(config.APIOrigin, mux)
}

func run(options webOptions) error {
	config, err := loadConfig(".env", options)
	if err != nil {
		return err
	}
	if _, err := fs.Stat(os.DirFS("dist"), "index.html"); err != nil {
		return errors.New("未找到 dist/index.html，请确认前端发布包完整且从发布目录运行")
	}
	logger := logging.NewWithOptions(logging.Options{Console: os.Stdout, ConsoleFormat: config.LogFormat, Color: config.LogColor, Level: slog.LevelInfo, AddSource: true})
	slog.SetDefault(logger)
	server := &http.Server{
		Addr:              config.Address,
		Handler:           handler(config, os.DirFS("dist")),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() {
		logger.Info("Zentrola Admin Web 已启动", "address", displayURL(config.Address))
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return errors.New("Admin Web 停止超时")
		}
		return <-done
	}
}

func main() {
	options, err := parseWebOptions(os.Args[1:])
	if err != nil {
		slog.Error("zentrola web stopped", "error", err.Error())
		os.Exit(2)
	}
	if options.Help {
		writeHelp(os.Stdout)
		return
	}
	if err := run(options); err != nil {
		slog.Error("zentrola web stopped", "error", err.Error())
		os.Exit(1)
	}
}
