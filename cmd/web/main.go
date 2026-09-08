package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
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
)

type webConfig struct {
	Address        string
	APIBaseURL     string
	APIOrigin      string
	GatewayBaseURL string
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

func loadConfig(file string) (webConfig, error) {
	values, err := godotenv.Read(file)
	if err != nil {
		return webConfig{}, fmt.Errorf("无法读取配置文件 %s", file)
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
	address := get("WEB_ADDR", ":3000")
	if _, _, err := net.SplitHostPort(address); err != nil {
		return webConfig{}, errors.New("WEB_ADDR 必须是有效的监听地址，例如 :3000")
	}
	rawBase := strings.TrimRight(get("WEB_API_BASE_URL", ""), "/")
	parsed, err := url.Parse(rawBase)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return webConfig{}, errors.New("WEB_API_BASE_URL 必须是有效的 HTTP(S) 地址，且不能包含账号、查询参数或 Fragment")
	}
	rawGateway := strings.TrimRight(get("WEB_GATEWAY_BASE_URL", rawBase), "/")
	gateway, err := url.Parse(rawGateway)
	if err != nil || (gateway.Scheme != "http" && gateway.Scheme != "https") || gateway.Host == "" ||
		gateway.User != nil || gateway.RawQuery != "" || gateway.Fragment != "" {
		return webConfig{}, errors.New("WEB_GATEWAY_BASE_URL 必须是有效的 HTTP(S) 地址，且不能包含账号、查询参数或 Fragment")
	}
	return webConfig{
		Address:        address,
		APIBaseURL:     rawBase,
		APIOrigin:      parsed.Scheme + "://" + parsed.Host,
		GatewayBaseURL: rawGateway,
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

func run() error {
	config, err := loadConfig(".env")
	if err != nil {
		return err
	}
	if _, err := fs.Stat(os.DirFS("dist"), "index.html"); err != nil {
		return errors.New("未找到 dist/index.html，请确认前端发布包完整且从发布目录运行")
	}
	server := &http.Server{
		Addr:              config.Address,
		Handler:           handler(config, os.DirFS("dist")),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() {
		log.Printf("Zentrola Admin Web 已启动：%s", displayURL(config.Address))
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
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
