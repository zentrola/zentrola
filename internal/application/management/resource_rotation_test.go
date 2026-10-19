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

// rotatedProbeError 模拟额度读取失败、但 app-server 已轮换 refresh token 的情况。
type rotatedProbeError struct{ credential []byte }

func (*rotatedProbeError) Error() string               { return "cannot read Codex ChatGPT rate limits" }
func (e *rotatedProbeError) RotatedCredential() []byte { return e.credential }

type failingSubscriptionAdapterStub struct {
	subscriptionAdapterStub
	err error
}

func (s failingSubscriptionAdapterStub) Probe(_ context.Context, _ []byte, proxy *catalog.OutboundProxy) (SubscriptionProbe, error) {
	if s.onProbe != nil {
		s.onProbe(proxy)
	}
	return SubscriptionProbe{}, s.err
}

func TestSubscriptionProbeFailurePersistsRotatedCredential(t *testing.T) {
	expiresAt := time.Date(2026, 10, 29, 3, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name           string
		err            error
		changeSealed   bool
		wantCiphertext string
		wantReplace    bool
	}{
		{name: "rotated credential is saved", err: &rotatedProbeError{credential: []byte("rotated-auth-cache")},
			wantCiphertext: "rotated-auth-cache", wantReplace: true},
		{name: "concurrent credential update wins", err: &rotatedProbeError{credential: []byte("rotated-auth-cache")},
			changeSealed: true, wantCiphertext: "admin-replaced"},
		{name: "failure without rotation keeps credential", err: errors.New("app-server stopped"),
			wantCiphertext: "imported-auth-cache"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writer := subscriptionRefreshWriter()
			events := []string{}
			adapter := failingSubscriptionAdapterStub{
				subscriptionAdapterStub: subscriptionAdapterStub{
					inspection: SubscriptionInspection{AccountRef: "account-1", CredentialExpiresAt: &expiresAt},
					onProbe: func(*catalog.OutboundProxy) {
						events = append(events, "probe")
						if tt.changeSealed {
							writer.resource.Sealed = catalog.SealedCredential{Ciphertext: []byte("admin-replaced"), KeyVersion: 1}
							writer.resource.Version++
						}
					},
				},
				err: tt.err,
			}
			service := New(lockingResourceStore{writer: writer, events: &events}, nil, providerTestCipher{}, nil,
				WithSubscriptionAdapter(adapter))

			result, err := service.TestResource(context.Background(), admin.Identity{ID: 1}, 48, appsec.RequestMeta{})
			if err != nil || result.OK {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if got := string(writer.resource.Sealed.Ciphertext); got != tt.wantCiphertext {
				t.Fatalf("stored credential=%q; want %q", got, tt.wantCiphertext)
			}
			if replaced := strings.Contains(strings.Join(events, ","), "replace"); replaced != tt.wantReplace {
				t.Fatalf("events=%v; want replace=%v", events, tt.wantReplace)
			}
		})
	}
}

func TestResetCreditConsumeFailurePersistsRotatedCredential(t *testing.T) {
	expiresAt := time.Date(2026, 10, 29, 3, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name           string
		err            error
		changeSealed   bool
		wantCiphertext string
		wantEvents     string
	}{
		{name: "rotated credential is saved", err: &rotatedProbeError{credential: []byte("rotated-auth-cache")},
			wantCiphertext: "rotated-auth-cache", wantEvents: "lock,read,consume,replace,unlock"},
		{name: "concurrent credential update wins", err: &rotatedProbeError{credential: []byte("rotated-auth-cache")},
			changeSealed: true, wantCiphertext: "admin-replaced", wantEvents: "lock,read,consume,unlock"},
		{name: "failure without rotation keeps credential", err: errors.New("app-server stopped"),
			wantCiphertext: "imported-auth-cache", wantEvents: "lock,read,consume,unlock"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writer := subscriptionRefreshWriter()
			events := []string{}
			adapter := resetCreditAdapterStub{
				subscriptionAdapterStub: subscriptionAdapterStub{
					inspection: SubscriptionInspection{AccountRef: "account-1", CredentialExpiresAt: &expiresAt},
				},
				err: tt.err,
				onConsume: func(string, string, *catalog.OutboundProxy) {
					events = append(events, "consume")
					if tt.changeSealed {
						writer.resource.Sealed = catalog.SealedCredential{Ciphertext: []byte("admin-replaced"), KeyVersion: 1}
						writer.resource.Version++
					}
				},
			}
			service := New(lockingResourceStore{writer: writer, events: &events}, nil, providerTestCipher{}, nil,
				WithSubscriptionAdapter(adapter))

			_, err := service.ConsumeResourceResetCredit(context.Background(), admin.Identity{ID: 1}, 48, "request-1", "", appsec.RequestMeta{})
			if !errors.Is(err, appsec.ErrUnavailable) {
				t.Fatalf("error=%v; want unavailable", err)
			}
			if got := string(writer.resource.Sealed.Ciphertext); got != tt.wantCiphertext {
				t.Fatalf("stored credential=%q; want %q", got, tt.wantCiphertext)
			}
			if sequence := strings.Join(events, ","); sequence != tt.wantEvents {
				t.Fatalf("event sequence=%s; want %s", sequence, tt.wantEvents)
			}
		})
	}
}
