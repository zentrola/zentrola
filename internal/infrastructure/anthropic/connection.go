// Package anthropic 提供 Anthropic 协议上游和官方账号连通性检查。
package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	mgmt "github.com/zentrola/zentrola/internal/application/management"
	"github.com/zentrola/zentrola/internal/domain/catalog"
	"github.com/zentrola/zentrola/internal/infrastructure/provider"
)

type ConnectionTester struct{ client *http.Client }

func NewConnectionTester() *ConnectionTester {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 5 * time.Second
	transport.ResponseHeaderTimeout = 10 * time.Second
	return &ConnectionTester{client: &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (t *ConnectionTester) Test(ctx context.Context, protocol, baseURL string, credential []byte, proxy *catalog.OutboundProxy) (result mgmt.ConnectionResult) {
	started := time.Now()
	defer func() { result.LatencyMS = time.Since(started).Milliseconds() }()
	base, allowed := allowedBaseURL(baseURL)
	if !allowed {
		result.Code = "UPSTREAM_URL_REJECTED"
		return
	}
	for _, ch := range credential {
		if ch < 33 || ch > 126 {
			result.Code = "CREDENTIAL_INVALID"
			return
		}
	}
	if len(credential) == 0 || len(credential) > 4096 {
		result.Code = "CREDENTIAL_INVALID"
		return
	}
	probeURL := base + "/v1/models?limit=1"
	bearer := false
	if protocol == "OPENAI" {
		probeURL = strings.TrimSuffix(base, "/") + "/models"
		bearer = true
	} else if protocol != "ANTHROPIC" {
		result.Code = "UPSTREAM_URL_REJECTED"
		return
	} else if base == "https://api.deepseek.com/anthropic" {
		probeURL = "https://api.deepseek.com/models"
		bearer = true
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL, nil)
	if err != nil {
		result.Code = "UPSTREAM_UNAVAILABLE"
		return
	}
	if bearer {
		req.Header.Set("Authorization", "Bearer "+string(credential))
	} else {
		req.Header.Set("x-api-key", string(credential))
		req.Header.Set("anthropic-version", "2023-06-01")
	}
	defer func() { req.Header.Del("Authorization"); req.Header.Del("x-api-key") }()
	req.Header.Set("Accept", "application/json")
	client, cleanup, err := provider.ClientWithProxy(t.client, proxy)
	if err != nil {
		result.Code = "PROXY_CONFIGURATION_UNRECOVERABLE"
		return
	}
	defer cleanup()
	resp, err := client.Do(req)
	if err != nil {
		result.Code = connectionErrorCode(ctx, err)
		return
	}
	defer resp.Body.Close()
	result.HTTPStatus = resp.StatusCode
	switch resp.StatusCode {
	case 200:
		data, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
		if err != nil {
			result.Code = connectionErrorCode(ctx, err)
			return
		}
		if len(data) > 64<<10 {
			result.Code = "UPSTREAM_INVALID_RESPONSE"
			return
		}
		var payload struct {
			Data []json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(data, &payload); err != nil || payload.Data == nil {
			result.Code = "UPSTREAM_INVALID_RESPONSE"
			return
		}
		result.OK = true
		result.Code = "OK"
	case 401, 403:
		result.Code = "UPSTREAM_AUTH_FAILED"
	case 429:
		result.Code = "UPSTREAM_RATE_LIMITED"
	default:
		result.Code = "UPSTREAM_UNAVAILABLE"
	}
	return
}

func connectionErrorCode(ctx context.Context, err error) string {
	if errors.Is(ctx.Err(), context.Canceled) {
		return "REQUEST_CANCELLED"
	}
	var netErr net.Error
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return "UPSTREAM_TIMEOUT"
	}
	return "UPSTREAM_UNAVAILABLE"
}
