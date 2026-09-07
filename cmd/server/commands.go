package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/infrastructure/config"
)

type commandOptions struct {
	name, username, configPath string
	help                       bool
	configFile                 string
	configProvided             bool
	showConfig                 bool
	port                       int
	foreground                 bool
}

func knownCommand(name string) bool {
	switch name {
	case "serve", "start", "restart", "stop", "status", "migrate", "healthcheck", "password", "config", "help":
		return true
	}
	return false
}

func parseCommand(args []string) (commandOptions, error) {
	result := commandOptions{name: "serve", configPath: ".env"}
	invalid := errors.New("命令或参数无效，请执行 zentrola help 查看用法")
	if len(args) == 0 {
		return result, nil
	}
	result.name = args[0]
	if result.name == "--help" || result.name == "-h" {
		result.name = "help"
	}
	if !knownCommand(result.name) {
		return result, invalid
	}
	if result.name == "help" {
		result.help = true
		if len(args) == 2 && knownCommand(args[1]) {
			result.name = args[1]
		} else if len(args) > 1 {
			return result, invalid
		}
		return result, nil
	}
	if len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
		result.help = true
		return result, nil
	}
	flags := flag.NewFlagSet(result.name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configSeen := false
	if result.name == "config" {
		flags.Func("file", "绑定已有通用配置文件", func(value string) error {
			if configSeen || strings.TrimSpace(value) == "" {
				return invalid
			}
			configSeen = true
			result.configFile = value
			return nil
		})
		flags.BoolVar(&result.showConfig, "show", false, "显示配置路径与环境")
	} else if result.name != "stop" && result.name != "status" {
		flags.Func("config", "通用配置文件路径（默认当前目录的 .env）", func(value string) error {
			if configSeen || strings.TrimSpace(value) == "" {
				return invalid
			}
			configSeen = true
			result.configProvided = true
			result.configPath = value
			return nil
		})
	}
	if result.name == "start" || result.name == "restart" || result.name == "serve" {
		flags.Func("port", "本次运行端口（1～65535）", func(value string) error {
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 || n > 65535 || result.port != 0 {
				return invalid
			}
			result.port = n
			return nil
		})
	}
	if result.name == "start" {
		flags.BoolVar(&result.foreground, "foreground", false, "前台运行，通过 Ctrl+C 停止")
	}
	if result.name == "password" {
		seen := false
		flags.Func("username", "管理员账号", func(value string) error {
			if seen {
				return invalid
			}
			seen = true
			result.username = value
			return nil
		})
	}
	if err := flags.Parse(args[1:]); err != nil {
		return result, invalid
	}
	if flags.NArg() != 0 {
		return result, invalid
	}
	if result.configFile != "" && result.showConfig {
		return result, invalid
	}
	if result.name == "password" && !appsec.ValidAdminUsername(result.username) {
		return result, errors.New("用法：zentrola password --username <账号>；账号必填，须为 1～64 bytes，不能含控制字符或首尾空白")
	}
	return result, nil
}

func loadCommandConfig(path string) (config.Config, error) {
	cfg, err := config.Load(path)
	if err != nil {
		resolved, pathErr := filepath.Abs(path)
		if pathErr != nil {
			resolved = path
		}
		return config.Config{}, fmt.Errorf("加载配置 %q 失败：%w；可用 config --file <路径> 统一绑定，或用 --config <路径> 临时指定", resolved, err)
	}
	return cfg, nil
}

func writeHelp(output io.Writer, name string) error {
	if name == "start" || name == "restart" || name == "stop" || name == "status" || name == "serve" {
		_, err := fmt.Fprintln(output, `用法：zentrola start [--port <端口>] [--config <路径>] [--foreground]
      zentrola restart [--port <端口>] [--config <路径>]
      zentrola stop
      zentrola status

start 默认后台启动，成功后显示访问地址、PID 和日志文件；--port 只覆盖本次运行。
restart 沿用上次配置和端口，允许显式覆盖；stop 等待请求和 Usage 写入结束。
status 显示后台进程状态、PID、端口和健康检查结果。
状态和日志保存在程序旁的 run 目录；每个程序目录管理一个后台实例。
start --foreground 和 serve 前台运行，通过 Ctrl+C 或容器信号停止，不纳入后台进程管理。
不带命令时仍前台运行，兼容现有容器。`)
		return err
	}
	if name == "config" {
		_, err := fmt.Fprintln(output, `用法：zentrola config --file <已有 .env 路径>
      zentrola config --show

将通用配置文件的绝对路径保存到可执行文件旁的 config.json。
后续命令自动复用；配置优先级为单次 --config > 已保存绑定 > 当前目录 .env。
绑定只保存路径，--show 只显示路径、来源和环境；不复制或输出密码。
使用绑定或单次 --config 时，配置所在目录作为运行目录，保持相对资源路径稳定。
示例：.\zentrola.exe config --file ..\.env`)
		return err
	}
	if name == "healthcheck" {
		_, err := fmt.Fprintln(output, `用法：zentrola healthcheck [--config <路径>]

检查服务的 /health/live，原样输出接口响应体；HTTP 200 退出码为 0，失败为非 0。
优先检查当前受管理后台服务的实际端口；--config 可临时指定其他配置。
没有后台服务时复用 config 绑定，只读取 HTTP_ADDR，不连接数据库。
未绑定且未传 --config 时沿用系统 HTTP_ADDR（默认 :9527）。`)
		return err
	}
	if name == "password" {
		_, err := fmt.Fprintln(output, `用法：zentrola password --username <账号> [--config <路径>]

为已有且启用的管理员生成安全随机密码，解除登录锁定，并使旧登录 Token 失效。
重置成功后仅显示一次新密码，请保存到密码管理器；不会强制再次改密。
使用当前环境的数据库配置；执行前停止使用相同 ID_NODE 的服务进程。
请在可信终端执行，不要将输出收集到共享日志。
自动复用 config 绑定，--config 可临时覆盖；未绑定时读取当前目录的 .env。
示例：zentrola password --username admin
在 Windows 的 bin 目录中：.\zentrola.exe password --username admin --config ..\.env`)
		return err
	}
	_, err := fmt.Fprintln(output, `用法：zentrola [命令]

命令：
  start [--port <端口>]         后台启动；--foreground 可前台运行
  restart [--port <端口>]       沿用上次配置与端口重启
  stop                          优雅停止后台服务
  status                        查看后台服务状态与健康状态
  serve                         前台启动（兼容命令，也是默认行为）
  migrate                       执行数据库 Schema 迁移后退出
  healthcheck                   检查服务存活状态，用于容器健康检查
  password --username <账号>    随机重置指定管理员密码
  config --file <路径>          统一绑定已有配置文件
  config --show                 查看配置路径和环境
  help [命令]                   查看命令帮助

支持 zentrola --help、zentrola -h 及 <命令> --help。
运行命令统一复用 config 绑定，支持 --config <路径> 临时覆盖。
未绑定时默认读取当前目录 .env；healthcheck 沿用系统 HTTP_ADDR（默认 :9527）。
除帮助外，命令按既有配置规则运行；help 无需配置文件或数据库连接。
执行 password 前，停止使用相同 ID_NODE 的服务进程。`)
	return err
}

func resetPassword(ctx context.Context, output io.Writer, username string, reset func(context.Context, string) (string, error)) error {
	password, err := reset(ctx, username)
	if errors.Is(err, appsec.ErrNotFound) {
		return errors.New("未找到可重置的启用管理员；请检查账号及所属组织状态")
	}
	if err != nil {
		return errors.New("密码重置未确认成功；请检查数据库、迁移及审计写入后重试")
	}
	if _, err := fmt.Fprintf(output, "管理员 %s 的密码已重置，登录锁定已清除，旧 Token 已失效。\n新密码（仅显示一次）：%s\n请保存到密码管理器。\n", username, password); err != nil {
		return errors.New("密码已重置，但输出失败；请在可用终端重新执行密码重置命令")
	}
	return nil
}
