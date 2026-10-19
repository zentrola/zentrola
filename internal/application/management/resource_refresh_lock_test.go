package management

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appsec "github.com/zentrola/zentrola/internal/application/security"
	"github.com/zentrola/zentrola/internal/domain/admin"
	"github.com/zentrola/zentrola/internal/domain/catalog"
)

// replacingResourceWriter 模拟支持按原密文条件保存新凭据的写事务。
type replacingResourceWriter struct {
	*resourceTestWriter
	events *[]string
}

func (w replacingResourceWriter) ReplaceSubscriptionCredential(_ context.Context, _ int64, expected, replacement catalog.SealedCredential,
	_, _ *time.Time, _ time.Time,
) (bool, error) {
	*w.events = append(*w.events, "replace")
	if !sameSealedCredential(w.resource.Sealed, expected) {
		return false, nil
	}
	w.resource.Sealed = replacement
	w.resource.Version++
	return true, nil
}

// lockingResourceStore 记录订阅刷新锁的获取与释放顺序。
type lockingResourceStore struct {
	writer *resourceTestWriter
	events *[]string
}

func (s lockingResourceStore) Read(_ context.Context, _ admin.Identity, fn func(Reader) error) error {
	*s.events = append(*s.events, "read")
	return fn(s.writer)
}
func (s lockingResourceStore) Write(_ context.Context, _ admin.Identity, fn func(Writer) error) error {
	return fn(replacingResourceWriter{resourceTestWriter: s.writer, events: s.events})
}
func (s lockingResourceStore) LockSubscriptionRefresh(_ context.Context, id int64) (func(), error) {
	*s.events = append(*s.events, "lock")
	return func() { *s.events = append(*s.events, "unlock") }, nil
}

func subscriptionRefreshWriter() *resourceTestWriter {
	return &resourceTestWriter{
		resource: ResourceRecord{
			Resource: Resource{
				ID: 48, ProviderID: 40, Name: "OpenAI 个人订阅", Version: 3,
				AuthType: AuthTypeSubscription, AuthAdapter: AuthAdapterOpenAICodex, RuntimeStatus: "HEALTHY",
			},
			Sealed: catalog.SealedCredential{Ciphertext: []byte("imported-auth-cache"), KeyVersion: 1},
		},
		provider: Provider{ID: 40, Code: "openai-official"},
	}
}

func TestSubscriptionProbeHoldsRefreshLockAndKeepsRotatedCredentialOnVersionConflict(t *testing.T) {
	for _, tt := range []struct {
		name            string
		changeSealed    bool
		wantCiphertext  string
		wantReplaceCall bool
	}{
		{name: "only metadata changed", wantCiphertext: "refreshed-auth-cache", wantReplaceCall: true},
		{name: "credential changed concurrently", changeSealed: true, wantCiphertext: "admin-replaced"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writer := subscriptionRefreshWriter()
			events := []string{}
			adapter := subscriptionAdapterStub{
				probe: SubscriptionProbe{
					Inspection: SubscriptionInspection{AccountRef: "account-1", PlanCode: "plus"},
					Credential: []byte("refreshed-auth-cache"),
				},
				onProbe: func(*catalog.OutboundProxy) {
					events = append(events, "probe")
					// 探测期间管理员修改了资源其他字段（或凭据本身）。
					writer.resource.Version++
					if tt.changeSealed {
						writer.resource.Sealed = catalog.SealedCredential{Ciphertext: []byte("admin-replaced"), KeyVersion: 1}
					}
				},
			}
			service := New(lockingResourceStore{writer: writer, events: &events}, nil, providerTestCipher{}, nil,
				WithSubscriptionAdapter(adapter))

			result, err := service.TestResource(context.Background(), admin.Identity{ID: 1}, 48, appsec.RequestMeta{})
			if err != nil || result.OK || result.Code != "RESOURCE_CHANGED" {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if got := string(writer.resource.Sealed.Ciphertext); got != tt.wantCiphertext {
				t.Fatalf("stored credential=%q; want %q", got, tt.wantCiphertext)
			}
			sequence := strings.Join(events, ",")
			want := "read,lock,read,probe,unlock"
			if tt.wantReplaceCall {
				want = "read,lock,read,probe,replace,unlock"
			}
			if sequence != want {
				t.Fatalf("event sequence=%s; want %s", sequence, want)
			}
		})
	}
}

func TestResetCreditConsumePersistsRotatedCredentialBeforeReportingConflict(t *testing.T) {
	writer := subscriptionRefreshWriter()
	events := []string{}
	adapter := resetCreditAdapterStub{
		consume: ResetCreditConsume{Outcome: "reset", Probe: SubscriptionProbe{
			Inspection: SubscriptionInspection{AccountRef: "account-1", PlanCode: "plus"},
			Credential: []byte("refreshed-auth-cache"),
		}},
		onConsume: func(string, string, *catalog.OutboundProxy) {
			events = append(events, "consume")
			writer.resource.Version++
		},
	}
	service := New(lockingResourceStore{writer: writer, events: &events}, nil, providerTestCipher{}, nil,
		WithSubscriptionAdapter(adapter))

	_, err := service.ConsumeResourceResetCredit(context.Background(), admin.Identity{ID: 1}, 48, "request-1", "", appsec.RequestMeta{})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("error=%v; want conflict", err)
	}
	if got := string(writer.resource.Sealed.Ciphertext); got != "refreshed-auth-cache" {
		t.Fatalf("rotated credential was dropped: %q", got)
	}
	if sequence := strings.Join(events, ","); sequence != "lock,read,consume,replace,unlock" {
		t.Fatalf("event sequence=%s", sequence)
	}
}

func TestAPIKeyConnectionTestDoesNotTakeSubscriptionRefreshLock(t *testing.T) {
	writer := &resourceTestWriter{
		resource: ResourceRecord{
			Resource: Resource{ID: 48, ProviderID: 40, Name: "key", AuthType: AuthTypeAPIKey, AuthAdapter: AuthAdapterAPIKey},
			Sealed:   catalog.SealedCredential{Ciphertext: []byte("sk-test"), KeyVersion: 1},
		},
		provider: Provider{ID: 40, Code: "custom", Endpoints: []ProviderEndpoint{{ProtocolType: "OPENAI", BaseURL: "https://api.example.com/v1"}}},
		model:    Model{ID: 90, Code: "model", Status: "ACTIVE"},
		mappings: []ProviderMapping{{ID: 1, ProviderID: 40, ModelID: 90}},
	}
	events := []string{}
	service := New(lockingResourceStore{writer: writer, events: &events}, nil, providerTestCipher{}, connectionTesterFunc(
		func(context.Context, ConnectionTarget, []byte, *catalog.OutboundProxy) ConnectionResult {
			return ConnectionResult{OK: true, Code: "OK", HTTPStatus: 200}
		},
	))
	if _, err := service.TestResource(context.Background(), admin.Identity{ID: 1}, 48, appsec.RequestMeta{}); err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event == "lock" {
			t.Fatalf("API key test took subscription refresh lock: %v", events)
		}
	}
}

// refreshCoordinatorStub 模拟网关的 Redis 订阅刷新锁：前 busy 次返回锁被占用。
type refreshCoordinatorStub struct {
	busy   int
	err    error
	events *[]string
}

func (c *refreshCoordinatorStub) AcquireSubscriptionRefreshResource(_ context.Context, id int64) (func(), bool, error) {
	if c.err != nil {
		return nil, false, c.err
	}
	if c.busy > 0 {
		c.busy--
		*c.events = append(*c.events, "busy")
		return nil, false, nil
	}
	*c.events = append(*c.events, "acquire")
	return func() { *c.events = append(*c.events, "release") }, true, nil
}

func TestSubscriptionProbeUsesGatewayRefreshCoordinator(t *testing.T) {
	for _, tt := range []struct {
		name      string
		busy      int
		err       error
		wantErr   error
		wantEvent string
	}{
		{name: "acquired after gateway releases", busy: 1, wantEvent: "read,busy,acquire,read,probe,replace,release"},
		{name: "coordinator unavailable fails closed", err: errors.New("redis unavailable"), wantErr: appsec.ErrUnavailable, wantEvent: "read"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writer := subscriptionRefreshWriter()
			events := []string{}
			adapter := subscriptionAdapterStub{
				probe: SubscriptionProbe{
					Inspection: SubscriptionInspection{AccountRef: "account-1", PlanCode: "plus"},
					Credential: []byte("refreshed-auth-cache"),
				},
				onProbe: func(*catalog.OutboundProxy) {
					events = append(events, "probe")
					writer.resource.Version++
				},
			}
			coordinator := &refreshCoordinatorStub{busy: tt.busy, err: tt.err, events: &events}
			service := New(lockingResourceStore{writer: writer, events: &events}, nil, providerTestCipher{}, nil,
				WithSubscriptionAdapter(adapter), WithSubscriptionRefreshCoordinator(coordinator))

			_, err := service.TestResource(context.Background(), admin.Identity{ID: 1}, 48, appsec.RequestMeta{})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error=%v; want %v", err, tt.wantErr)
			}
			// 注入协调器后不得再使用存储层的刷新锁，否则与网关持有的不是同一把锁。
			if sequence := strings.Join(events, ","); sequence != tt.wantEvent {
				t.Fatalf("event sequence=%s; want %s", sequence, tt.wantEvent)
			}
		})
	}
}

func TestSubscriptionRefreshCoordinatorWaitStopsWithCaller(t *testing.T) {
	events := []string{}
	service := New(lockingResourceStore{writer: subscriptionRefreshWriter(), events: &events}, nil, providerTestCipher{}, nil,
		WithSubscriptionRefreshCoordinator(&refreshCoordinatorStub{busy: 1 << 30, events: &events}))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := service.lockSubscriptionRefresh(ctx, 48); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error=%v; want caller deadline", err)
	}
}
