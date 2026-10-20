package modelcatalog

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/config"
	"github.com/zentrola/zentrola/internal/infrastructure/postgres"
	"github.com/zentrola/zentrola/internal/infrastructure/security"
)

// TestLiveCatalogDiscovery 读取本地数据库中已配置的 API Key，调用各厂商官方模型目录，
// 验证适配器能解析当前的上游响应。只读取数据库，不写入同步结果。
// 默认跳过；设置 ZENTROLA_LIVE_CATALOG=1 后运行，配置文件默认为仓库根目录的 .env，可用 ZENTROLA_LIVE_CONFIG 覆盖。
// 设置 ZENTROLA_LIVE_PROXY 时所有厂商改用该代理，忽略服务商自身的代理配置。
func TestLiveCatalogDiscovery(t *testing.T) {
	if os.Getenv("ZENTROLA_LIVE_CATALOG") != "1" {
		t.Skip("set ZENTROLA_LIVE_CATALOG=1 to call real provider catalogs")
	}
	configPath := strings.TrimSpace(os.Getenv("ZENTROLA_LIVE_CONFIG"))
	if configPath == "" {
		configPath = filepath.Join("..", "..", "..", ".env")
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	master, err := security.LoadMasterKey(cfg.Security.MasterKey)
	if err != nil {
		t.Fatalf("load master key: %v", err)
	}
	credentials, err := security.NewCredentials(master)
	if err != nil {
		t.Fatalf("init credentials: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := postgres.Open(ctx, cfg.Postgres)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()

	discoverer := NewDiscoverer(nil)
	for _, providerCode := range []string{
		catalog.GoogleOfficialCode, catalog.DeepSeekOfficialCode, catalog.ZhipuOfficialCode,
		catalog.KimiOfficialCode, catalog.QwenOfficialCode,
	} {
		t.Run(providerCode, func(t *testing.T) {
			var (
				providerID, resourceID               int64
				runtimeStatus                        string
				proxyEnabled                         bool
				credential, proxyURL, proxyHeaders   catalog.SealedCredential
				proxyURLVersion, proxyHeadersVersion *int32
			)
			// 与管理端同步一致：优先未封禁的凭据，同类按优先级选择；这里只测试 API Key。
			err := pool.QueryRow(ctx, `
				SELECT p.id, c.id, c.runtime_status, c.credential_ciphertext, c.credential_nonce, c.key_version,
				       p.proxy_enabled, p.proxy_url_ciphertext, p.proxy_url_nonce, p.proxy_url_key_version,
				       p.proxy_headers_ciphertext, p.proxy_headers_nonce, p.proxy_headers_key_version
				FROM provider_credential c
				JOIN provider p ON p.id = c.provider_id
				WHERE p.provider_code = $1 AND p.is_deleted = false AND c.is_deleted = false AND c.auth_type = $2
				ORDER BY c.runtime_status = 'BLOCKED', c.priority, c.id
				LIMIT 1`, providerCode, mgmt.AuthTypeAPIKey,
			).Scan(&providerID, &resourceID, &runtimeStatus, &credential.Ciphertext, &credential.Nonce, &credential.KeyVersion,
				&proxyEnabled, &proxyURL.Ciphertext, &proxyURL.Nonce, &proxyURLVersion,
				&proxyHeaders.Ciphertext, &proxyHeaders.Nonce, &proxyHeadersVersion)
			if err != nil {
				t.Skipf("no API Key credential configured: %v", err)
			}

			rows, err := pool.Query(ctx, `
				SELECT protocol_type, base_url, network_scope FROM provider_endpoint
				WHERE provider_id = $1 ORDER BY protocol_type`, providerID)
			if err != nil {
				t.Fatalf("query endpoints: %v", err)
			}
			var endpoints []mgmt.ProviderEndpoint
			for rows.Next() {
				var endpoint mgmt.ProviderEndpoint
				if err := rows.Scan(&endpoint.ProtocolType, &endpoint.BaseURL, &endpoint.NetworkScope); err != nil {
					t.Fatalf("scan endpoint: %v", err)
				}
				endpoints = append(endpoints, endpoint)
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("read endpoints: %v", err)
			}

			var proxy *catalog.OutboundProxy
			if override := strings.TrimSpace(os.Getenv("ZENTROLA_LIVE_PROXY")); override != "" {
				proxy = &catalog.OutboundProxy{URL: override}
			} else if proxyEnabled {
				proxy = decryptLiveProxy(t, credentials, providerID, proxyURL, proxyURLVersion, proxyHeaders, proxyHeadersVersion)
			}

			plain, err := credentials.Decrypt(credential, catalog.CredentialOwner{ProviderID: providerID, ResourceID: resourceID})
			if err != nil {
				t.Fatalf("decrypt credential %d: %v", resourceID, err)
			}
			defer clear(plain)

			models, result := discoverer.Discover(ctx, mgmt.ModelDiscoverySource{
				ProviderCode: providerCode, Endpoints: endpoints, AuthType: mgmt.AuthTypeAPIKey,
			}, plain, proxy)
			t.Logf("credential=%d runtime=%s proxy=%t endpoints=%d", resourceID, runtimeStatus, proxy != nil, len(endpoints))
			if !result.OK || len(models) == 0 {
				t.Fatalf("discovery failed: code=%s http=%d models=%d", result.Code, result.HTTPStatus, len(models))
			}
			t.Logf("%d models in %dms", len(models), result.LatencyMS)
			for _, model := range models {
				t.Logf("  %-40s %-40s in=%v out=%v", model.Code, model.Name, model.InputModalities, model.OutputModalities)
			}
		})
	}
}

func decryptLiveProxy(t *testing.T, credentials *security.Credentials, providerID int64,
	urlSealed catalog.SealedCredential, urlVersion *int32,
	headersSealed catalog.SealedCredential, headersVersion *int32,
) *catalog.OutboundProxy {
	t.Helper()
	if urlVersion == nil {
		t.Fatal("provider proxy is enabled without a sealed URL")
	}
	urlSealed.KeyVersion = *urlVersion
	urlBytes, err := credentials.DecryptProviderProxy(urlSealed, catalog.ProviderProxyOwner{ProviderID: providerID, Field: "url"})
	if err != nil {
		t.Fatalf("decrypt provider proxy URL: %v", err)
	}
	proxy := &catalog.OutboundProxy{URL: string(urlBytes), Headers: map[string]string{}}
	clear(urlBytes)
	if headersVersion == nil || *headersVersion == 0 {
		return proxy
	}
	headersSealed.KeyVersion = *headersVersion
	headerBytes, err := credentials.DecryptProviderProxy(headersSealed, catalog.ProviderProxyOwner{ProviderID: providerID, Field: "headers"})
	if err != nil {
		t.Fatalf("decrypt provider proxy headers: %v", err)
	}
	defer clear(headerBytes)
	if err := json.Unmarshal(headerBytes, &proxy.Headers); err != nil {
		t.Fatalf("decode provider proxy headers: %v", err)
	}
	return proxy
}
