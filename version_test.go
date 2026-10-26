package zentrola

import "testing"

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "release", content: "APP_ENV=prod\nAPP_VERSION=1.3.0\n", want: "1.3.0"},
		{name: "pre-release with CRLF", content: "APP_VERSION=2.0.0-rc.1\r\n", want: "2.0.0-rc.1"},
		{name: "missing", content: "APP_ENV=prod\n", want: ""},
		{name: "not semantic", content: "APP_VERSION=dev\n", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseVersion(tt.content); got != tt.want {
				t.Fatalf("parseVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEmbeddedVersionIsValid(t *testing.T) {
	if Version() == "" {
		t.Fatal(".env.example must declare a semantic APP_VERSION")
	}
}
