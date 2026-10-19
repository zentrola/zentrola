package openaicodex

import "strings"

// inheritedEnvironmentKeys 是 app-server 子进程可继承的环境变量（按大写比较，Windows 环境变量不区分大小写）。
// 只保留定位可执行文件、用户与临时目录、系统根目录、TLS 证书、区域设置和全局代理所需的变量，
// 避免把服务端的数据库密码、主密钥等秘密泄露给子进程。CODEX_HOME 由调用方单独指定。
var inheritedEnvironmentKeys = map[string]struct{}{
	// 可执行文件查找；Windows 下 codex 常经由 npm 生成的 codex.cmd 启动，依赖 PATHEXT 与 ComSpec。
	"PATH": {}, "PATHEXT": {}, "COMSPEC": {},
	// 用户与系统目录。
	"HOME": {}, "USER": {}, "LOGNAME": {}, "USERPROFILE": {}, "USERNAME": {}, "USERDOMAIN": {},
	"HOMEDRIVE": {}, "HOMEPATH": {}, "APPDATA": {}, "LOCALAPPDATA": {}, "PROGRAMDATA": {},
	"SYSTEMROOT": {}, "SYSTEMDRIVE": {}, "WINDIR": {},
	"PROGRAMFILES": {}, "PROGRAMFILES(X86)": {}, "PROGRAMW6432": {},
	"COMMONPROGRAMFILES": {}, "COMMONPROGRAMFILES(X86)": {}, "COMMONPROGRAMW6432": {},
	"COMPUTERNAME": {}, "NUMBER_OF_PROCESSORS": {}, "PROCESSOR_ARCHITECTURE": {}, "OS": {},
	// 临时目录。
	"TMP": {}, "TEMP": {}, "TMPDIR": {},
	// TLS 证书。
	"SSL_CERT_FILE": {}, "SSL_CERT_DIR": {},
	// 区域设置与时区。
	"LANG": {}, "LANGUAGE": {}, "TZ": {},
	// 全局代理；配置了服务商代理时由 EnvironmentWithProxy 覆盖。
	"HTTP_PROXY": {}, "HTTPS_PROXY": {}, "ALL_PROXY": {}, "NO_PROXY": {},
}

// codexEnvironment 从 source 中筛选允许继承的环境变量，保留原始键名大小写。
func codexEnvironment(source []string) []string {
	result := make([]string, 0, len(inheritedEnvironmentKeys))
	for _, item := range source {
		key, _, ok := strings.Cut(item, "=")
		if !ok || key == "" {
			continue
		}
		upper := strings.ToUpper(key)
		if _, allowed := inheritedEnvironmentKeys[upper]; allowed || strings.HasPrefix(upper, "LC_") {
			result = append(result, item)
		}
	}
	return result
}
