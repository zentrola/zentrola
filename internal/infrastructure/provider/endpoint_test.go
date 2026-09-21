package provider

import (
	"context"
	"net"
	"testing"

	"github.com/zentrola/zentrola/internal/domain/catalog"
)

type staticResolver []net.IPAddr

func (r staticResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return r, nil
}

func TestPrivateBaseURLAllowsExplicitLocalTargets(t *testing.T) {
	for _, raw := range []string{
		"http://192.168.1.20:8080/v1",
		"https://10.0.0.5:8443",
		"http://127.0.0.1:11434/v1",
		"http://models.internal:8080",
	} {
		if _, ok := BaseURLForScope(raw, catalog.NetworkScopePrivate); !ok {
			t.Fatalf("private endpoint rejected: %q", raw)
		}
	}
	for _, raw := range []string{
		"http://169.254.169.254/latest/meta-data",
		"http://[fe80::1]:8080",
		"ftp://192.168.1.20/models",
	} {
		if _, ok := BaseURLForScope(raw, catalog.NetworkScopePrivate); ok {
			t.Fatalf("unsafe private endpoint accepted: %q", raw)
		}
	}
}

func TestBaseURLRejectsUnknownNetworkScope(t *testing.T) {
	if _, ok := BaseURLForScope("https://api.example.com", "INVALID"); ok {
		t.Fatal("unknown network scope accepted")
	}
}

func TestBaseURL(t *testing.T) {
	for raw, want := range map[string]string{
		"https://api.example.com/anthropic/":                "https://api.example.com/anthropic",
		"https://dashscope.aliyuncs.com/compatible-mode/v1": "https://dashscope.aliyuncs.com/compatible-mode/v1",
	} {
		got, ok := BaseURL(raw)
		if !ok || got != want {
			t.Fatalf("valid endpoint rejected: %q => %q, %v", raw, got, ok)
		}
	}

	for _, raw := range []string{
		"http://api.example.com",
		"https://localhost",
		"https://127.0.0.1",
		"https://10.0.0.1",
		"https://key@api.example.com",
		"https://api.example.com/v1?token=secret",
		"https://api.example.com/path/../escape",
	} {
		if _, ok := BaseURL(raw); ok {
			t.Fatalf("unsafe endpoint accepted: %q", raw)
		}
	}
}

func TestResolvePublicAddressesRejectsAnyPrivateDNSResult(t *testing.T) {
	public := staticResolver{{IP: net.ParseIP("8.8.8.8")}}
	addresses, err := resolvePublicAddresses(context.Background(), public, "api.example.com")
	if err != nil || len(addresses) != 1 {
		t.Fatalf("public DNS result rejected: addresses=%v err=%v", addresses, err)
	}
	for name, resolver := range map[string]staticResolver{
		"loopback": {{IP: net.ParseIP("127.0.0.1")}},
		"private":  {{IP: net.ParseIP("10.0.0.8")}},
		"shared":   {{IP: net.ParseIP("100.64.0.8")}},
		"link-local": {
			{IP: net.ParseIP("169.254.169.254")},
		},
		"mixed": {{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("192.168.1.5")}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := resolvePublicAddresses(context.Background(), resolver, "api.example.com"); err == nil {
				t.Fatal("non-public DNS result accepted")
			}
		})
	}
}

func TestResolvePrivateAddressesAllowsPrivateButRejectsLinkLocal(t *testing.T) {
	private := staticResolver{{IP: net.ParseIP("192.168.1.20")}}
	if _, err := resolveEndpointAddresses(context.Background(), private, "models.internal", catalog.NetworkScopePrivate); err != nil {
		t.Fatalf("private DNS result rejected: %v", err)
	}
	metadata := staticResolver{{IP: net.ParseIP("169.254.169.254")}}
	if _, err := resolveEndpointAddresses(context.Background(), metadata, "metadata.internal", catalog.NetworkScopePrivate); err == nil {
		t.Fatal("link-local metadata address accepted")
	}
}
