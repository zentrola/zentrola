package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/zentrola/zentrola/internal/application/gateway"
	"github.com/zentrola/zentrola/internal/application/health"
	"github.com/zentrola/zentrola/internal/application/management"
	appsec "github.com/zentrola/zentrola/internal/application/security"
	usageapp "github.com/zentrola/zentrola/internal/application/usage"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/infrastructure/anthropic"
	"github.com/zentrola/zentrola/internal/infrastructure/config"
	"github.com/zentrola/zentrola/internal/infrastructure/idgen"
	"github.com/zentrola/zentrola/internal/infrastructure/logging"
	"github.com/zentrola/zentrola/internal/infrastructure/modelcatalog"
	"github.com/zentrola/zentrola/internal/infrastructure/openai"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres"
	"github.com/zentrola/zentrola/internal/infrastructure/redisstate"
	cryptosec "github.com/zentrola/zentrola/internal/infrastructure/security"
	"github.com/zentrola/zentrola/internal/infrastructure/telemetry"
	httptransport "github.com/zentrola/zentrola/internal/transport/http"
)

// @title Zentrola API
// @version 1.0
// @description 企业 AI Coding 能力治理平台。管理接口先登录获取 data.token，再在 Authorize 中填写 AdminBearer（Bearer + 空格 + Token）。Gateway 使用成员 Access Key，与管理员 Token 分开。JSON 中的业务 ID 使用字符串。
// @BasePath /
// @securityDefinitions.apikey AdminBearer
// @in header
// @name Authorization
// @description 管理员登录返回的 Token，填写：Bearer <token>。
// @securityDefinitions.apikey GatewayBearer
// @in header
// @name Authorization
// @description 成员 Access Key，填写：Bearer <access-key>。可用于 OpenAI 或 Anthropic 接口。
// @securityDefinitions.apikey GatewayKey
// @in header
// @name x-api-key
// @description 成员 Access Key 原文，仅用于 Anthropic 接口；不要同时发送 Authorization。
func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		// run 的错误只包含安全诊断，不输出原始数据库错误或配置对象。
		slog.Error("zentrola stopped", "error", err.Error())
		// 后台子进程会关闭控制台日志，并把常规日志写入 JSON 文件。
		// run 返回时文件 writer 已经 flush/close，因此最终错误额外写到 stderr，
		// 由进程管理器保存到 run/server.log，避免致命错误丢失。
		if os.Getenv("ZENTROLA_BACKGROUND_CHILD") == "1" {
			_, _ = fmt.Fprintln(os.Stderr, "zentrola stopped:", err)
		}
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) (runErr error) {
	command, err := parseCommand(args)
	if err != nil {
		return err
	}
	if command.help {
		return writeHelp(output, command.name)
	}
	mode := command.name
	runtimePath, err := executableRuntimeDir()
	if err != nil {
		return err
	}
	if mode == "stop" || mode == "status" || mode == "restart" || mode == "start" {
		return manageProcess(command, runtimePath, output)
	}
	if mode == "healthcheck" && !command.configProvided {
		addr, found, err := currentManagedAddress(runtimePath)
		if err != nil {
			return err
		}
		if found {
			return healthcheck(addr, output)
		}
	}
	var selection configSelection
	child := mode == "serve" && os.Getenv("ZENTROLA_BACKGROUND_CHILD") == "1"
	if child {
		selection = configSelection{path: os.Getenv("ZENTROLA_CHILD_CONFIG"), pinned: os.Getenv("ZENTROLA_CHILD_PINNED") == "1"}
		if !filepath.IsAbs(selection.path) {
			return errors.New("invalid background server config path")
		}
		if selection.pinned {
			err = requireConfigFile(selection.path)
		}
	} else {
		selection, err = selectConfig(command)
	}
	if err != nil {
		return err
	}
	if child {
		return managedChild(command, selection, runtimePath, output)
	}
	if mode == "healthcheck" {
		addr := os.Getenv("HTTP_ADDR")
		if selection.pinned {
			addr, err = config.LoadHealthcheckAddress(selection.path)
			if err != nil {
				return err
			}
		}
		if err := healthcheck(addr, output); err != nil {
			return fmt.Errorf("health check failed: %w", err)
		}
		return nil
	}
	cfg, err := loadCommandConfig(selection.path)
	if err != nil {
		return err
	}
	if err := applyPort(&cfg, command.port); err != nil {
		return err
	}
	return runService(command, selection, cfg, nil, output)
}

func runService(command commandOptions, selection configSelection, cfg config.Config, managed *managedProcess, output io.Writer) (runErr error) {
	mode := command.name
	// 显式指定配置后，Master Key 等相对运行路径也必须稳定。
	if selection.pinned {
		previous, err := os.Getwd()
		if err != nil {
			return errors.New("cannot read the current working directory")
		}
		if err := os.Chdir(filepath.Dir(selection.path)); err != nil {
			return errors.New("cannot enter the config file directory")
		}
		defer os.Chdir(previous)
	}
	var logFile *logging.AsyncWriter
	if cfg.LogFilePath != "" {
		opened, openErr := logging.OpenRotatingFile(cfg.LogFilePath, cfg.LogFileMaxSizeMB, cfg.LogFileMaxBackups)
		if openErr != nil {
			if errors.Is(openErr, logging.ErrFileInUse) {
				return errors.New("log file is already in use by another process")
			}
			return errors.New("cannot open log file")
		}
		logFile = logging.NewAsyncWriter(opened, os.Stderr, 4096)
		defer func() {
			flush, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := logFile.Close(flush); err != nil {
				runErr = errors.Join(runErr, errors.New("cannot flush log file"))
			}
		}()
	}
	var console io.Writer = os.Stdout
	// 内置后台模式已经把 stdout/stderr 重定向到 run/server.log；启用应用 JSON 文件时
	// 关闭常规控制台副本，避免同一条日志写入两个文件。
	if managed != nil && logFile != nil {
		console = nil
	}
	logger := logging.NewWithOptions(logging.Options{
		Console:       console,
		ConsoleFormat: cfg.LogConsoleFormat,
		Color:         cfg.LogColor,
		File:          logFile,
		ErrorOutput:   os.Stderr,
		Level:         cfg.LogLevel,
		AddSource:     true,
	})
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	tracerProvider := telemetry.Setup()
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := telemetry.Shutdown(shutdown, tracerProvider); err != nil {
			runErr = errors.Join(runErr, errors.New("cannot shut down telemetry"))
		}
	}()
	if managed != nil {
		cancelWatch := context.AfterFunc(managed.ctx, stop)
		defer cancelWatch()
	}
	// 先占用端口，避免端口冲突时已经迁移数据库或触碰资源凭证。
	var listener net.Listener
	if mode == "serve" || mode == "start" {
		var err error
		listener, err = net.Listen("tcp", cfg.HTTPAddr)
		if err != nil {
			return errors.New("cannot listen on HTTP_ADDR; port may already be in use")
		}
		defer listener.Close()
	}
	startup, cancel := context.WithTimeout(ctx, cfg.StartupTimeout)
	defer cancel()
	pool, err := postgres.Open(startup, cfg.Postgres)
	if err != nil {
		return err
	}
	defer pool.Close()
	if cfg.AutoMigrate || mode == "migrate" {
		if err := postgres.Migrate(startup, pool); err != nil {
			return err
		}
		logger.Info("database migrations complete")
	}
	if mode == "migrate" {
		return nil
	}
	ids := idgen.New(pool)
	if mode == "password" {
		passwords, err := cryptosec.NewPasswords(12)
		if err != nil {
			return err
		}
		reset := appsec.NewPasswordReset(postgres.NewSecurityStore(pool, ids), passwords)
		return resetPassword(startup, output, command.username, reset.Reset)
	}
	master, err := cryptosec.LoadMasterKey(cfg.Security.MasterKey, cfg.Security.ExternalMasterKeyPath, cfg.Security.MasterKeyPath)
	if err != nil {
		return err
	}
	credentials, err := cryptosec.NewCredentials(master)
	if err != nil {
		return err
	}
	tokens, err := cryptosec.NewJWT(cfg.Security.JWTSecret)
	if err != nil {
		return err
	}
	if err := tokens.ValidateIndependentMaster(master); err != nil {
		return err
	}
	passwords, err := cryptosec.NewPasswords(12)
	if err != nil {
		return err
	}
	securityStore := postgres.NewSecurityStore(pool, ids)
	adminService := appsec.NewAdmin(securityStore, passwords, tokens, admin.LoginPolicy{MaxFailures: cfg.Security.MaxLoginFailures, LockDuration: cfg.Security.LoginLockDuration})
	setup, err := adminService.SetupStatus(startup)
	if err != nil {
		return err
	}
	if setup.Required {
		logger.Info("administrator setup required; open the management page to create the first administrator")
	}
	disabled, err := securityStore.RecoverCredentials(startup, credentials, master.Created)
	if err != nil {
		return err
	}
	if disabled > 0 {
		logger.Warn("resources disabled: credentials cannot be recovered", "count", disabled, "error_code", "CREDENTIAL_UNRECOVERABLE")
	}
	keyService := appsec.NewKeys(securityStore, ids)
	connectionTester := anthropic.NewConnectionTester()
	managementService := management.New(
		postgres.NewManagementStore(pool, ids), ids, credentials, connectionTester,
		management.WithModelDiscoverer(modelcatalog.NewDiscoverer(logger)),
	)
	anthropicClient := anthropic.NewGatewayClient(cfg.Gateway.HeaderTimeout)
	defer anthropicClient.CloseIdleConnections()
	openaiClient := openai.NewGatewayClient(cfg.Gateway.HeaderTimeout)
	defer openaiClient.CloseIdleConnections()
	compatibleUpstream := gateway.NewCompatibleUpstream(anthropicClient, openaiClient)
	routeState := redisstate.New(cfg.Redis.Host, cfg.Redis.Port, cfg.Redis.Database, cfg.Redis.Password)
	defer routeState.Close()
	redisProbe, redisProbeCancel := context.WithTimeout(startup, time.Second)
	if err := routeState.Ping(redisProbe); err != nil {
		logger.Warn("route state Redis unavailable; cooldown state will fail open", "error_code", "REDIS_UNAVAILABLE")
	}
	redisProbeCancel()
	gatewayService := gateway.New(
		postgres.NewGatewayStore(pool), credentials, compatibleUpstream,
		gateway.WithRouteState(routeState), gateway.WithMaxAttempts(2),
	)
	usageStore := postgres.NewUsageStore(pool)
	usageWriter, err := usageapp.NewWriter(usageStore, ids, logger, usageapp.Options{QueueSize: cfg.Usage.QueueSize, BatchSize: cfg.Usage.BatchSize, FlushInterval: cfg.Usage.FlushInterval, WriteTimeout: cfg.Usage.WriteTimeout})
	if err != nil {
		return err
	}
	defer func() {
		flush, done := context.WithTimeout(context.Background(), cfg.Usage.ShutdownTimeout)
		defer done()
		if err := usageWriter.Close(flush); err != nil {
			runErr = errors.Join(runErr, err)
		}
		m := usageWriter.Metrics()
		logger.Info("usage writer stopped", "persisted", m.Persisted, "failed", m.Failed, "pending", m.Pending)
	}()
	gatewayHandler := httptransport.NewGatewayHandler(gatewayService, cfg.Gateway, logger, usageWriter)
	openaiHandler := httptransport.NewOpenAIGatewayHandler(gatewayService, cfg.Gateway, logger, usageWriter)
	cancel()

	readiness := health.New(
		health.Check{Name: "postgres", Run: pool.Ping},
		health.Check{Name: "master_key", Run: master.Check},
		health.Check{Name: "admin", Run: adminService.Check},
	)
	var active sync.WaitGroup
	var admission sync.Mutex
	stopping := false
	router := httptransport.NewRouter(logger, readiness, cfg.CORS, cfg.HealthTimeout, cfg.Environment, &httptransport.SecurityHandlers{Admin: adminService, Keys: keyService, Management: managementService, Gateway: gatewayHandler, OpenAI: openaiHandler, Usage: usageapp.NewQuery(usageStore), UsageWriter: usageWriter})
	server := &http.Server{
		Addr: cfg.HTTPAddr,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			admission.Lock()
			if stopping {
				admission.Unlock()
				w.WriteHeader(503)
				return
			}
			active.Add(1)
			admission.Unlock()
			defer active.Done()
			router.ServeHTTP(w, r)
		}),
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		// 不设全局 WriteTimeout，避免截断后续的长 SSE 请求。
		ErrorLog: slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
	logger.Info("zentrola listening", "address", listener.Addr().String(), "environment", cfg.Environment)
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve(listener) }()
	if managed != nil {
		managed.setPhase("running")
	}
	// 先结束所有 Handler 的 Usage 提交，再执行前面注册的 Writer.Close。
	defer func() {
		_ = server.Close()
		admission.Lock()
		stopping = true
		admission.Unlock()
		active.Wait()
	}()
	select {
	case err := <-serveResult:
		if !errors.Is(err, http.ErrServerClosed) {
			return errors.New("HTTP server failed")
		}
		return nil
	case <-ctx.Done():
		stop()
		logger.Info("shutdown started")
	}
	shutdown, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	if err := server.Shutdown(shutdown); err != nil {
		_ = server.Close()
		return errors.New("HTTP shutdown deadline exceeded")
	}
	logger.Info("HTTP shutdown complete; flushing usage")
	return nil
}

func healthcheck(addr string, output io.Writer) error {
	if addr == "" {
		addr = ":9527"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
	defer client.CloseIdleConnections()
	resp, err := client.Get("http://" + net.JoinHostPort(host, port) + "/health/live")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if _, err := io.Copy(output, resp.Body); err != nil {
		return fmt.Errorf("cannot read or print the health check response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %d", resp.StatusCode)
	}
	return nil
}
