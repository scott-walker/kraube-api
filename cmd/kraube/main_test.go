package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionString(t *testing.T) {
	orig := Version
	defer func() { Version = orig }()

	tests := []struct {
		injected string
		want     string
	}{
		{"0.6.1", "kraube v0.6.1"},  // GoReleaser injects without the "v"
		{"v0.9.0", "kraube v0.9.0"}, // already prefixed — not doubled
	}
	for _, tt := range tests {
		Version = tt.injected
		if got := versionString(); got != tt.want {
			t.Errorf("versionString() with Version=%q = %q, want %q", tt.injected, got, tt.want)
		}
	}

	// Plain `go build` in the work tree: no ldflags, and build info carries
	// no module version — must fall back to "dev", never panic or go blank.
	Version = "dev"
	if got := versionString(); !strings.HasPrefix(got, "kraube ") || strings.TrimPrefix(got, "kraube ") == "" {
		t.Errorf("versionString() fallback = %q, want non-empty 'kraube <ver>'", got)
	}
}

func TestResidualFlagError(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string // substring of the error, "" = no error
	}{
		{"plain prompt", []string{"hello world"}, ""},
		{"multi-word prompt", []string{"what", "is", "go"}, ""},
		{"typo flag alone", []string{"--typo"}, "unknown flag: --typo"},
		{"typo flag before prompt", []string{"--nonexistent-flag", "hi"}, "unknown flag: --nonexistent-flag"},
		{"typo flag after prompt", []string{"hi", "--vresion"}, "unknown flag: --vresion"},
		{"single-dash typo", []string{"-x"}, "unknown flag: -x"},
		{"bare dash is not a flag", []string{"-"}, ""},
		{"stream with prompt", []string{"stream", "tell me a story"}, ""},
		{"serve with its flags", []string{"serve", "--listen", "127.0.0.1:0", "--auth-key", "k", "--refresh-margin", "5m"}, ""},
		{"serve with typo", []string{"serve", "--liste", "127.0.0.1:0"}, "unknown flag: --liste"},
		{"login with --out", []string{"login", "--out", "/tmp/x.json"}, ""},
		{"login flag outside login", []string{"--out", "/tmp/x.json", "hi"}, "unknown flag: --out"},
		{"serve flag outside serve", []string{"--listen", "127.0.0.1:0"}, "unknown flag: --listen"},
		{"known flag missing value", []string{"--proxy"}, "flag --proxy requires a value"},
		{"gen flag missing value", []string{"--model"}, "flag --model requires a value"},
		{"duplicated value flag", []string{"--model", "claude-opus-4-1", "hi"}, "flag --model given more than once"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := residualFlagError(tt.args)
			if tt.want == "" && got != "" {
				t.Errorf("residualFlagError(%q) = %q, want no error", tt.args, got)
			}
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Errorf("residualFlagError(%q) = %q, want containing %q", tt.args, got, tt.want)
			}
		})
	}
}

// TestCLI_VersionAndTypo_NoNetwork is the end-to-end guarantee behind both
// guards: `kraube --version` and `kraube --typo` must resolve locally. The
// binary runs with a credentials path pointing into an empty temp dir — any
// code path that tried to build a client (the prerequisite for an API call)
// would fail with "Not authenticated" instead of the expected output.
func TestCLI_VersionAndTypo_NoNetwork(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "kraube-under-test")
	build := exec.Command("go", "build", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	env := append(os.Environ(),
		"KRAUBE_CREDENTIALS_PATH="+filepath.Join(tmp, "no-such-credentials.json"),
		"KRAUBE_DEBUG=",
	)
	run := func(args ...string) (stdout, stderr string, exitCode int) {
		t.Helper()
		var outBuf, errBuf bytes.Buffer
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf
		err := cmd.Run()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("run %v: %v", args, err)
		}
		return outBuf.String(), errBuf.String(), code
	}

	for _, args := range [][]string{{"--version"}, {"-v"}, {"version"}} {
		stdout, stderr, code := run(args...)
		if code != 0 {
			t.Errorf("%v: exit = %d, stderr = %q, want 0", args, code, stderr)
		}
		if !strings.HasPrefix(stdout, "kraube ") {
			t.Errorf("%v: stdout = %q, want 'kraube <version>'", args, stdout)
		}
		if strings.Contains(stderr, "Not authenticated") {
			t.Errorf("%v: tried to build an API client: %q", args, stderr)
		}
	}

	for _, args := range [][]string{
		{"--nonexistent-flag"},
		{"--vresion"},
		{"--typo", "some prompt"},
	} {
		stdout, stderr, code := run(args...)
		if code != 1 {
			t.Errorf("%v: exit = %d, want 1", args, code)
		}
		if !strings.Contains(stderr, "unknown flag") {
			t.Errorf("%v: stderr = %q, want 'unknown flag' error", args, stderr)
		}
		if !strings.Contains(stderr, "--help") {
			t.Errorf("%v: stderr = %q, want a usage hint", args, stderr)
		}
		if strings.Contains(stderr, "Not authenticated") || stdout != "" {
			t.Errorf("%v: reached the client path: stdout=%q stderr=%q", args, stdout, stderr)
		}
	}

	// Prompt-shaped positional text must still dispatch to the API path —
	// which, with no credentials, fails authentication (proving the guard
	// does not swallow the declared `kraube "prompt"` interface).
	_, stderr, code := run("plain prompt text")
	if code != 1 || !strings.Contains(stderr, "Not authenticated") {
		t.Errorf("prompt dispatch broken: exit=%d stderr=%q, want auth failure", code, stderr)
	}
}
