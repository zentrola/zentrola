package http

import (
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
)

// mountWeb 只公开构建产物的入口和 assets，不暴露源码、配置或任意工作区文件。
// 前端使用 hash 路由，API / Swagger 的未知路径仍由原处理器返回 404。
func mountWeb(r chi.Router) {
	dist := os.DirFS("web/dist")
	r.Get("/", webFile(dist, "index.html"))
	r.Get("/favicon.svg", webFile(dist, "favicon.svg"))
	r.Get("/assets/*", webFile(dist, ""))
}

func webFile(dist fs.FS, fixed string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := fixed
		if name == "" {
			name = strings.TrimPrefix(r.URL.Path, "/")
			if !fs.ValidPath(name) || !strings.HasPrefix(name, "assets/") {
				http.NotFound(w, r)
				return
			}
		}
		body, err := fs.ReadFile(dist, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; connect-src 'self'; font-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-cache")
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		if kind := mime.TypeByExtension(path.Ext(name)); kind != "" {
			w.Header().Set("Content-Type", kind)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}
