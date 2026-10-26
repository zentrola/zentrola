// Package zentrola 提供编译时嵌入的项目元数据。
package zentrola

import (
	_ "embed"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
)

//go:embed .env.example
var envExample string

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

var version = parseVersion(envExample)

// Version 返回编译时从 .env.example 嵌入的 APP_VERSION；缺失或不是语义化版本时返回空字符串。
func Version() string {
	return version
}

func parseVersion(content string) string {
	values, err := godotenv.Unmarshal(content)
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(values["APP_VERSION"])
	if !versionPattern.MatchString(value) {
		return ""
	}
	return value
}
