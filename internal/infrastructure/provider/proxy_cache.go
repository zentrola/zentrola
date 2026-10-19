package provider

import (
	"container/list"
	"crypto/sha256"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"
)

const (
	// proxyTransportCacheLimit 限制同时缓存的代理 Transport 数量；超出时淘汰最久未使用的配置。
	// 每个 Transport 持有独立连接池，64 足以覆盖常见的服务商代理配置数量。
	proxyTransportCacheLimit = 64
	// proxyTransportIdleTTL 是代理配置的闲置回收时间；配置变更后旧配置不再被使用，最迟在该时间后回收。
	proxyTransportIdleTTL = 10 * time.Minute
)

// proxyTransports 是进程内共享的代理 Transport 缓存。键包含基础 Transport 身份，
// 因此不同上游适配器的超时与网络策略不会互相串用。
var proxyTransports = newProxyTransportCache(proxyTransportCacheLimit, proxyTransportIdleTTL, time.Now)

// proxyTransportKey 只保存代理 URL 与代理请求头的摘要，缓存键本身不含明文代理凭据。
type proxyTransportKey struct {
	base         *http.Transport
	networkScope string
	digest       [sha256.Size]byte
}

type proxyTransportEntry struct {
	key       proxyTransportKey
	transport http.RoundTripper
	inner     *http.Transport
	lastUsed  time.Time
}

type proxyTransportCache struct {
	mu      sync.Mutex
	entries map[proxyTransportKey]*list.Element
	// order 按最近使用排序，队首为最近使用的配置。
	order   *list.List
	limit   int
	idleTTL time.Duration
	now     func() time.Time
}

func newProxyTransportCache(limit int, idleTTL time.Duration, now func() time.Time) *proxyTransportCache {
	return &proxyTransportCache{
		entries: make(map[proxyTransportKey]*list.Element), order: list.New(),
		limit: limit, idleTTL: idleTTL, now: now,
	}
}

// get 返回 key 对应的 Transport，不存在时调用 create 创建；created 表示本次新建。
// 被淘汰的 Transport 只关闭空闲连接，仍在进行的请求不受影响，其连接在请求结束后由 IdleConnTimeout 回收。
func (c *proxyTransportCache) get(key proxyTransportKey, create func() (http.RoundTripper, *http.Transport)) (transport http.RoundTripper, created bool) {
	c.mu.Lock()
	now := c.now()
	evicted := c.evictIdleLocked(now)
	if element, ok := c.entries[key]; ok {
		entry := element.Value.(*proxyTransportEntry)
		entry.lastUsed = now
		c.order.MoveToFront(element)
		c.mu.Unlock()
		closeIdleTransports(evicted)
		return entry.transport, false
	}
	wrapped, inner := create()
	c.entries[key] = c.order.PushFront(&proxyTransportEntry{key: key, transport: wrapped, inner: inner, lastUsed: now})
	for c.limit > 0 && c.order.Len() > c.limit {
		evicted = append(evicted, c.removeLocked(c.order.Back()))
	}
	c.mu.Unlock()
	closeIdleTransports(evicted)
	return wrapped, true
}

// evictIdleLocked 从最久未使用的一端移除闲置超过 idleTTL 的配置。
func (c *proxyTransportCache) evictIdleLocked(now time.Time) []*http.Transport {
	var evicted []*http.Transport
	for c.idleTTL > 0 {
		back := c.order.Back()
		if back == nil || now.Sub(back.Value.(*proxyTransportEntry).lastUsed) < c.idleTTL {
			break
		}
		evicted = append(evicted, c.removeLocked(back))
	}
	return evicted
}

func (c *proxyTransportCache) removeLocked(element *list.Element) *http.Transport {
	entry := element.Value.(*proxyTransportEntry)
	c.order.Remove(element)
	delete(c.entries, entry.key)
	return entry.inner
}

// closeBase 移除并关闭基于 base 创建的全部代理 Transport 的空闲连接，供上游客户端关闭时调用。
func (c *proxyTransportCache) closeBase(base *http.Transport) {
	c.mu.Lock()
	var evicted []*http.Transport
	for element := c.order.Front(); element != nil; {
		next := element.Next()
		if element.Value.(*proxyTransportEntry).key.base == base {
			evicted = append(evicted, c.removeLocked(element))
		}
		element = next
	}
	c.mu.Unlock()
	closeIdleTransports(evicted)
}

func (c *proxyTransportCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

func closeIdleTransports(transports []*http.Transport) {
	for _, transport := range transports {
		transport.CloseIdleConnections()
	}
}

// CloseIdleProxyConnections 关闭并移除基于 base 的全部代理 Transport；base 非 *http.Transport 时忽略。
func CloseIdleProxyConnections(base *http.Client) {
	if base == nil {
		return
	}
	if transport, ok := base.Transport.(*http.Transport); ok {
		proxyTransports.closeBase(transport)
	}
}

// proxyDigest 计算代理 URL（含凭据）与代理请求头的摘要。请求头按规范化名称排序，
// 使等价配置得到相同摘要；字段间以 0 字节分隔，避免拼接歧义。
func proxyDigest(proxyURL *url.URL, headers map[string]string) [sha256.Size]byte {
	hash := sha256.New()
	hash.Write([]byte(proxyURL.String()))
	hash.Write([]byte{0})
	canonical := make(http.Header, len(headers))
	for name, value := range headers {
		canonical.Set(name, value)
	}
	names := make([]string, 0, len(canonical))
	for name := range canonical {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		hash.Write([]byte(name))
		hash.Write([]byte{0})
		hash.Write([]byte(canonical.Get(name)))
		hash.Write([]byte{0})
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return digest
}
