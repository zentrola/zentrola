// Package zentrola 提供编译时嵌入的项目元数据。
package zentrola

import (
	_ "embed"
	"regexp"
	"strings"
)

//go:embed VERSION
var versionFile string

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

var version = parseVersion(versionFile)

// Version 返回编译时从 VERSION 嵌入的应用版本；缺失或不是语义化版本时返回空字符串。
func Version() string {
	return version
}

func parseVersion(content string) string {
	value := strings.TrimSpace(content)
	if !versionPattern.MatchString(value) {
		return ""
	}
	return value
}
