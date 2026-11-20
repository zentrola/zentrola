package openaicodex

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var codexVersionPattern = regexp.MustCompile(`^codex-cli ([0-9]{1,4}\.[0-9]{1,4}\.[0-9]{1,6})(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

// ClientVersion 读取指定 Codex CLI 的基础版本，去除预发布及构建后缀，供模型目录请求使用。
func ClientVersion(ctx context.Context, executable string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if strings.TrimSpace(executable) == "" {
		executable = "codex"
	}
	command := exec.CommandContext(ctx, executable, "--version")
	command.Env = codexEnvironment(os.Environ())
	command.WaitDelay = 200 * time.Millisecond
	tree, err := newProcessTree()
	if err != nil {
		return "", err
	}
	defer tree.release()
	tree.configure(command)
	command.Cancel = tree.kill
	var output bytes.Buffer
	command.Stdout = &output
	if err := command.Start(); err != nil {
		return "", err
	}
	if err := tree.attach(); err != nil {
		_ = tree.kill()
		_ = command.Wait()
		return "", err
	}
	err = command.Wait()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		return "", err
	}
	return parseClientVersion(output.String())
}

func parseClientVersion(output string) (string, error) {
	match := codexVersionPattern.FindStringSubmatch(strings.TrimSpace(output))
	if match == nil {
		return "", errors.New("invalid Codex CLI version output")
	}
	return match[1], nil
}
