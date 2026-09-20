package provider

import (
	"context"
	"net"
	"testing"
)

type staticResolver []net.IPAddr

func (r staticResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return r, nil
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
