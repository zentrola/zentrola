package http

import (
	_ "embed"
	"net/http"

	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"
)

// 文档在开发阶段生成，运行时直接提供打包后的 JSON，不扫描源码。
//
//go:embed apidocs/swagger.json
var swaggerJSON []byte

func mountSwagger(r chi.Router, environment string) {
	if environment != "dev" && environment != "test" {
		return
	}
	r.Get("/swagger", func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/swagger/index.html", http.StatusFound)
	})
	r.Get("/swagger/doc.json", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(swaggerJSON)
	})
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("doc.json"),
		httpSwagger.DocExpansion("list"),
		httpSwagger.PersistAuthorization(false),
	))
}
