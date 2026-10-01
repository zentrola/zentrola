package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type releaseOptions struct {
	targetOS       string
	architecture   string
	component      string
	skipNPMInstall bool
}

type releaseTarget struct {
	name   string
	goos   string
	goarch string
}

var releaseMetadataFiles = []string{
	".env.example",
	"VERSION",
	"LICENSE",
	"NOTICE",
	"THIRD_PARTY_NOTICES.md",
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "release build failed:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	options, err := parseOptions(args, stderr)
	if err != nil {
		return err
	}
	projectRoot, err := findProjectRoot()
	if err != nil {
		return err
	}
	target, err := resolveTarget(options)
	if err != nil {
		return err
	}
	component, err := resolveComponent(options.component)
	if err != nil {
		return err
	}
	goCommand, err := exec.LookPath("go")
	if err != nil {
		return errors.New("go executable not found in PATH")
	}
	npmCommand := ""
	if component != "backend" {
		npmName := "npm"
		if runtime.GOOS == "windows" {
			npmName = "npm.cmd"
		}
		npmCommand, err = exec.LookPath(npmName)
		if err != nil {
			return fmt.Errorf("%s not found in PATH", npmName)
		}
	}

	_, _ = fmt.Fprintf(stdout, "Building %s release for %s/%s\n", component, target.name, target.goarch)
	releaseBase := filepath.Join(projectRoot, "dist")
	if err := os.MkdirAll(releaseBase, 0o755); err != nil {
		return fmt.Errorf("create release root: %w", err)
	}
	stagingRoot, err := os.MkdirTemp(releaseBase, "."+target.name+"-"+target.goarch+"-")
	if err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(stagingRoot) }()

	for _, name := range releaseMetadataFiles {
		if err := copyFile(filepath.Join(projectRoot, name), filepath.Join(stagingRoot, name)); err != nil {
			return err
		}
	}
	if err := copyTree(filepath.Join(projectRoot, "third_party_licenses"), filepath.Join(stagingRoot, "third_party_licenses"), nil); err != nil {
		return fmt.Errorf("copy third-party licenses: %w", err)
	}

	webRoot := filepath.Join(projectRoot, "web")
	if component != "backend" {
		webBuildRoot := webRoot
		if !options.skipNPMInstall {
			webBuildRoot = filepath.Join(stagingRoot, ".web-build")
			excluded := map[string]bool{
				".env":              true,
				"dist":              true,
				"node_modules":      true,
				"playwright-report": true,
				"test-results":      true,
			}
			if err := copyTree(webRoot, webBuildRoot, excluded); err != nil {
				return fmt.Errorf("prepare isolated web build: %w", err)
			}
			npmEnv := map[string]string{
				"NPM_CONFIG_CACHE": filepath.Join(projectRoot, ".cache", "npm"),
			}
			if err := runCommand(webBuildRoot, npmEnv, stdout, stderr, npmCommand, "ci"); err != nil {
				return fmt.Errorf("npm install failed: %w", err)
			}
		}
		if err := runCommand(webBuildRoot, nil, stdout, stderr, npmCommand, "run", "build"); err != nil {
			return fmt.Errorf("admin web build failed: %w", err)
		}
		if err := copyTree(filepath.Join(webBuildRoot, "dist"), filepath.Join(stagingRoot, "dist"), nil); err != nil {
			return fmt.Errorf("copy admin web assets: %w", err)
		}
		if webBuildRoot != webRoot {
			if err := os.RemoveAll(webBuildRoot); err != nil {
				return fmt.Errorf("clean isolated web build: %w", err)
			}
		}
	}

	executableSuffix := ""
	if target.goos == "windows" {
		executableSuffix = ".exe"
	}
	buildEnv := map[string]string{
		"CGO_ENABLED": "0",
		"GOCACHE":     filepath.Join(projectRoot, ".cache", "go-build"),
		"GOARCH":      target.goarch,
		"GOOS":        target.goos,
	}
	if component != "web" {
		if err := runCommand(projectRoot, buildEnv, stdout, stderr, goCommand,
			"build", "-trimpath", "-ldflags=-s -w",
			"-o", filepath.Join(stagingRoot, "zentrola"+executableSuffix), "./cmd/server"); err != nil {
			return fmt.Errorf("backend build failed: %w", err)
		}
	}
	if component != "backend" {
		if err := runCommand(projectRoot, buildEnv, stdout, stderr, goCommand,
			"build", "-trimpath", "-ldflags=-s -w",
			"-o", filepath.Join(stagingRoot, "zentrola-web"+executableSuffix), "./cmd/web"); err != nil {
			return fmt.Errorf("admin web launcher build failed: %w", err)
		}
	}

	releaseRoot := filepath.Join(releaseBase, target.name, target.goarch)
	if err := ensureChildPath(releaseBase, releaseRoot); err != nil {
		return err
	}
	if err := publishRelease(stagingRoot, releaseRoot, component, executableSuffix); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stdout, "Release created at", releaseRoot)
	return nil
}

func publishRelease(stagingRoot, releaseRoot, component, executableSuffix string) error {
	if err := os.MkdirAll(releaseRoot, 0o755); err != nil {
		return fmt.Errorf("create target release directory: %w", err)
	}
	for _, name := range releaseMetadataFiles {
		if err := copyFile(filepath.Join(stagingRoot, name), filepath.Join(releaseRoot, name)); err != nil {
			return err
		}
	}
	thirdPartyDestination := filepath.Join(releaseRoot, "third_party_licenses")
	if err := ensureChildPath(releaseRoot, thirdPartyDestination); err != nil {
		return err
	}
	if err := os.RemoveAll(thirdPartyDestination); err != nil {
		return fmt.Errorf("remove stale third-party licenses: %w", err)
	}
	if err := copyTree(filepath.Join(stagingRoot, "third_party_licenses"), thirdPartyDestination, nil); err != nil {
		return fmt.Errorf("publish third-party licenses: %w", err)
	}
	if component != "web" {
		if err := removeReleasePaths(releaseRoot, "zentrola", "zentrola.exe"); err != nil {
			return err
		}
		if err := copyFile(filepath.Join(stagingRoot, "zentrola"+executableSuffix), filepath.Join(releaseRoot, "zentrola"+executableSuffix)); err != nil {
			return fmt.Errorf("publish backend executable: %w", err)
		}
	}
	if component != "backend" {
		if err := removeReleasePaths(releaseRoot, "zentrola-web", "zentrola-web.exe"); err != nil {
			return err
		}
		if err := copyFile(filepath.Join(stagingRoot, "zentrola-web"+executableSuffix), filepath.Join(releaseRoot, "zentrola-web"+executableSuffix)); err != nil {
			return fmt.Errorf("publish admin web executable: %w", err)
		}
		webDestination := filepath.Join(releaseRoot, "dist")
		if err := clearDirectoryContents(webDestination); err != nil {
			return fmt.Errorf("clear existing admin web assets: %w", err)
		}
		if err := copyTree(filepath.Join(stagingRoot, "dist"), webDestination, nil); err != nil {
			return fmt.Errorf("publish admin web assets: %w", err)
		}
	}
	return nil
}

func removeReleasePaths(releaseRoot string, names ...string) error {
	for _, name := range names {
		path := filepath.Join(releaseRoot, name)
		if err := ensureChildPath(releaseRoot, path); err != nil {
			return err
		}
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove stale release path %s: %w", path, err)
		}
	}
	return nil
}

func clearDirectoryContents(directory string) error {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return os.MkdirAll(directory, 0o755)
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(directory, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func parseOptions(args []string, output io.Writer) (releaseOptions, error) {
	var options releaseOptions
	flags := flag.NewFlagSet("release", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&options.targetOS, "target-os", "", "target operating system: windows, linux or macos")
	flags.StringVar(&options.architecture, "architecture", "", "target architecture: amd64, arm64 or 386")
	flags.StringVar(&options.component, "component", "all", "release component: backend, web or all")
	flags.BoolVar(&options.skipNPMInstall, "skip-npm-install", false, "use the existing web/node_modules")
	if err := flags.Parse(args); err != nil {
		return releaseOptions{}, err
	}
	if flags.NArg() != 0 {
		return releaseOptions{}, fmt.Errorf("unexpected arguments: %s", strings.Join(flags.Args(), " "))
	}
	return options, nil
}

func resolveComponent(value string) (string, error) {
	component := strings.ToLower(strings.TrimSpace(value))
	if component == "" {
		component = "all"
	}
	if component != "backend" && component != "web" && component != "all" {
		return "", fmt.Errorf("unsupported component %q; use backend, web or all", value)
	}
	return component, nil
}

func resolveTarget(options releaseOptions) (releaseTarget, error) {
	targetOS := strings.ToLower(strings.TrimSpace(options.targetOS))
	if targetOS == "" {
		targetOS = runtime.GOOS
	}
	target := releaseTarget{name: targetOS, goos: targetOS}
	if targetOS == "darwin" || targetOS == "macos" {
		target.name, target.goos = "macos", "darwin"
	}
	if target.goos != "windows" && target.goos != "linux" && target.goos != "darwin" {
		return releaseTarget{}, fmt.Errorf("unsupported target OS %q; use windows, linux or macos", options.targetOS)
	}

	architecture := options.architecture
	if strings.TrimSpace(architecture) == "" {
		architecture = runtime.GOARCH
		if runtime.GOOS == "windows" {
			if value := os.Getenv("PROCESSOR_ARCHITEW6432"); value != "" {
				architecture = value
			} else if value := os.Getenv("PROCESSOR_ARCHITECTURE"); value != "" {
				architecture = value
			}
		}
	}
	target.goarch = normalizeArchitecture(architecture)
	if target.goarch == "" {
		return releaseTarget{}, fmt.Errorf("unsupported architecture %q; use amd64, arm64 or 386", architecture)
	}
	if target.goos == "darwin" && target.goarch == "386" {
		return releaseTarget{}, errors.New("macOS does not support the 386 architecture")
	}
	return target, nil
}

func normalizeArchitecture(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "AMD64", "X64":
		return "amd64"
	case "386", "I386", "X86":
		return "386"
	case "ARM64", "AARCH64":
		return "arm64"
	default:
		return ""
	}
}

func findProjectRoot() (string, error) {
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if data, readErr := os.ReadFile(filepath.Join(current, "go.mod")); readErr == nil {
			moduleLine := strings.SplitN(string(data), "\n", 2)[0]
			if strings.TrimSpace(moduleLine) == "module github.com/zentrola/zentrola" {
				return current, nil
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("project root not found; run this command inside the Zentrola repository")
		}
		current = parent
	}
}

func runCommand(directory string, overrides map[string]string, stdout, stderr io.Writer, name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Dir = directory
	command.Stdout = stdout
	command.Stderr = stderr
	if len(overrides) != 0 {
		command.Env = overriddenEnvironment(os.Environ(), overrides)
	}
	_, _ = fmt.Fprintf(stdout, "> %s %s\n", filepath.Base(name), strings.Join(args, " "))
	if err := command.Run(); err != nil {
		return err
	}
	return nil
}

func overriddenEnvironment(current []string, overrides map[string]string) []string {
	result := make([]string, 0, len(current)+len(overrides))
	for _, entry := range current {
		key, _, found := strings.Cut(entry, "=")
		if !found {
			continue
		}
		overridden := false
		for override := range overrides {
			if strings.EqualFold(key, override) {
				overridden = true
				break
			}
		}
		if !overridden {
			result = append(result, entry)
		}
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}

func copyTree(source, destination string, excludedTopLevel map[string]bool) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(destination, 0o755)
		}
		first := strings.Split(relative, string(filepath.Separator))[0]
		if excludedTopLevel[first] {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic link is not supported in release input: %s", path)
		}
		return copyFile(path, target)
	})
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open %s: %w", source, err)
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", source, err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create %s: %w", destination, err)
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return fmt.Errorf("copy %s: %w", source, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", destination, closeErr)
	}
	return nil
}

func ensureChildPath(parent, child string) error {
	relative, err := filepath.Rel(parent, child)
	if err != nil {
		return err
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing to replace unsafe release path %s", child)
	}
	return nil
}
