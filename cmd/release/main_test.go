package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeArchitecture(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"AMD64":   "amd64",
		"x64":     "amd64",
		"386":     "386",
		"I386":    "386",
		"x86":     "386",
		"arm64":   "arm64",
		"AARCH64": "arm64",
		"mips":    "",
	}
	for input, expected := range tests {
		if actual := normalizeArchitecture(input); actual != expected {
			t.Errorf("normalizeArchitecture(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestResolveComponent(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"":        "all",
		"ALL":     "all",
		"backend": "backend",
		" Web ":   "web",
	}
	for input, expected := range tests {
		actual, err := resolveComponent(input)
		if err != nil {
			t.Fatalf("resolveComponent(%q) returned error: %v", input, err)
		}
		if actual != expected {
			t.Errorf("resolveComponent(%q) = %q, want %q", input, actual, expected)
		}
	}
	if _, err := resolveComponent("desktop"); err == nil {
		t.Fatal("unsupported component was accepted")
	}
}

func TestCopyTreeExcludesGeneratedDirectories(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	destination := t.TempDir()
	writeTestFile(t, filepath.Join(source, "src", "main.ts"), "source")
	writeTestFile(t, filepath.Join(source, "node_modules", "package", "index.js"), "dependency")
	writeTestFile(t, filepath.Join(source, "dist", "index.html"), "generated")
	writeTestFile(t, filepath.Join(source, ".env"), "SECRET=value")

	err := copyTree(source, destination, map[string]bool{
		".env":         true,
		"dist":         true,
		"node_modules": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(destination, "src", "main.ts")); err != nil || string(data) != "source" {
		t.Fatalf("source file was not copied: data=%q err=%v", data, err)
	}
	for _, path := range []string{"node_modules", "dist", ".env"} {
		if _, err := os.Stat(filepath.Join(destination, path)); !os.IsNotExist(err) {
			t.Errorf("excluded path %s exists or returned unexpected error: %v", path, err)
		}
	}
}

func TestEnsureChildPath(t *testing.T) {
	t.Parallel()
	parent := filepath.Join(t.TempDir(), "dist")
	if err := ensureChildPath(parent, filepath.Join(parent, "windows", "amd64")); err != nil {
		t.Fatalf("valid child rejected: %v", err)
	}
	if err := ensureChildPath(parent, filepath.Dir(parent)); err == nil {
		t.Fatal("path outside parent was accepted")
	}
}

func TestPublishBackendPreservesWebRelease(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	stagingRoot := filepath.Join(root, "staging")
	releaseRoot := filepath.Join(root, "release")
	writeTestFile(t, filepath.Join(stagingRoot, "zentrola.exe"), "new backend")
	writeTestFile(t, filepath.Join(stagingRoot, ".env.example"), "new config")
	writeTestFile(t, filepath.Join(releaseRoot, "zentrola.exe"), "old backend")
	writeTestFile(t, filepath.Join(releaseRoot, "zentrola-web.exe"), "keep web")
	writeTestFile(t, filepath.Join(releaseRoot, "dist", "index.html"), "keep assets")

	if err := publishRelease(stagingRoot, releaseRoot, "backend", ".exe"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(releaseRoot, "zentrola.exe")); err != nil || string(data) != "new backend" {
		t.Fatalf("new backend was not published: data=%q err=%v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(releaseRoot, "zentrola-web.exe")); err != nil || string(data) != "keep web" {
		t.Fatalf("web executable changed: data=%q err=%v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(releaseRoot, "dist", "index.html")); err != nil || string(data) != "keep assets" {
		t.Fatalf("web assets changed: data=%q err=%v", data, err)
	}
}

func TestPublishWebPreservesBackendRelease(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	stagingRoot := filepath.Join(root, "staging")
	releaseRoot := filepath.Join(root, "release")
	writeTestFile(t, filepath.Join(stagingRoot, "zentrola-web"), "new web")
	writeTestFile(t, filepath.Join(stagingRoot, ".env.example"), "new config")
	writeTestFile(t, filepath.Join(stagingRoot, "dist", "index.html"), "new assets")
	writeTestFile(t, filepath.Join(releaseRoot, "zentrola"), "keep backend")
	writeTestFile(t, filepath.Join(releaseRoot, "zentrola-web"), "old web")
	writeTestFile(t, filepath.Join(releaseRoot, "dist", "old.js"), "stale assets")

	if err := publishRelease(stagingRoot, releaseRoot, "web", ""); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(releaseRoot, "zentrola")); err != nil || string(data) != "keep backend" {
		t.Fatalf("backend executable changed: data=%q err=%v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(releaseRoot, "zentrola-web")); err != nil || string(data) != "new web" {
		t.Fatalf("new web executable was not published: data=%q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(releaseRoot, "dist", "old.js")); !os.IsNotExist(err) {
		t.Fatalf("stale web asset was not removed: %v", err)
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
