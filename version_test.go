package zentrola

import "testing"

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "release", content: "1.3.0\n", want: "1.3.0"},
		{name: "pre-release with CRLF", content: "2.0.0-rc.1\r\n", want: "2.0.0-rc.1"},
		{name: "missing", content: "", want: ""},
		{name: "not semantic", content: "dev\n", want: ""},
		{name: "extra line", content: "1.3.0\n1.3.1\n", want: ""},
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
		t.Fatal("VERSION must declare a semantic version")
	}
}
