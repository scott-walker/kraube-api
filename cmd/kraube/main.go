package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/scott-walker/kraube-api"
)

// cliProxy holds the proxy URL parsed from the --proxy flag (if any). When
// empty, the client still picks up HTTPS_PROXY / ALL_PROXY from the
// environment automatically inside NewClient.
var cliProxy string

// Version is the release version, injected by GoReleaser via
// `-ldflags "-X main.Version={{.Version}}"`. A plain `go build` leaves it as
// "dev"; versionString then falls back to the module version recorded in
// build info, so `go install ...@vX.Y.Z` binaries still report themselves.
var Version = "dev"

// versionString renders the one-line `kraube --version` output. Preference
// order: linker-injected Version, then the module version from build info
// (covers `go install ...@vX.Y.Z`), then plain "dev". The version is
// normalized to a leading "v" ("kraube v0.6.1") to match release tags.
func versionString() string {
	v := Version
	if v == "" || v == "dev" {
		if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			v = bi.Main.Version
		} else {
			v = "dev"
		}
	}
	if v != "dev" && !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return "kraube " + v
}

func main() {
	// Dev logging: kraube --debug ... or KRAUBE_DEBUG=1
	if hasFlag("--debug") || os.Getenv("KRAUBE_DEBUG") == "1" {
		kraube.EnableDevLog()
	}

	// Global proxy flag — applies to all subcommands (login, usage, query,
	// stream). Parsed before command dispatch so it is stripped from os.Args.
	cliProxy = flagValue("--proxy")

	// If --proxy is set, route OAuth calls (login / refresh / FetchProfile)
	// through the same chrome-fingerprinted transport + proxy as the message
	// traffic. Without this, `kraube login` would go direct even with --proxy.
	if cliProxy != "" {
		if err := applyAuthProxy(cliProxy); err != nil {
			fmt.Fprintf(os.Stderr, "proxy: %v\n", err)
			os.Exit(1)
		}
	}

	// Generation flags — apply to query/stream/default (the "send a message" path).
	// Parsed before command dispatch so they are stripped from os.Args.
	genOpts := parseGenFlags()

	// Informational flags — resolved locally, must never reach the API.
	if hasFlag("--version") || hasFlag("-v") {
		fmt.Println(versionString())
		return
	}
	if hasFlag("--help") || hasFlag("-h") {
		printUsage(os.Stdout)
		return
	}

	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(1)
	}

	// Every known flag has been stripped from os.Args by now (command-specific
	// flags like --out / --listen are the exception: they are parsed after
	// dispatch and skipped by the check). Anything flag-like that remains is a
	// typo — refuse it loudly, because the default dispatch below sends
	// positional arguments to the API as a prompt, and a mistyped flag must
	// not become a paid request.
	if msg := residualFlagError(os.Args[1:]); msg != "" {
		fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
		fmt.Fprintln(os.Stderr, "Run `kraube --help` for usage.")
		os.Exit(1)
	}

	ctx := context.Background()

	switch os.Args[1] {
	case "login":
		cmdLogin(ctx)
	case "usage":
		cmdUsage(ctx)
	case "serve":
		cmdServe(ctx)
	case "version":
		fmt.Println(versionString())
	case "help":
		printUsage(os.Stdout)
	case "stream":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: kraube stream \"prompt\"")
			os.Exit(1)
		}
		cmdStream(ctx, strings.Join(os.Args[2:], " "), genOpts)
	default:
		cmdQuery(ctx, strings.Join(os.Args[1:], " "), genOpts)
	}
}

// printUsage writes the CLI usage text to w: os.Stderr with exit 1 when the
// invocation was wrong, os.Stdout with exit 0 for an explicit help request.
func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  kraube login [--out PATH]  — authenticate via browser")
	fmt.Fprintln(w, "  kraube usage               — show plan usage limits")
	fmt.Fprintln(w, "  kraube \"your prompt\"       — send a message")
	fmt.Fprintln(w, "  kraube stream \"prompt\"     — stream response")
	fmt.Fprintln(w, "  kraube serve               — local HTTP daemon (proxy + token keepalive)")
	fmt.Fprintln(w, "  kraube version             — print the binary version")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Serve flags:")
	fmt.Fprintln(w, "  --listen ADDR              — listen address (default 127.0.0.1:8787)")
	fmt.Fprintln(w, "  --auth-key KEY             — require Bearer/x-api-key auth (or KRAUBE_SERVE_KEY)")
	fmt.Fprintln(w, "                               mandatory when ADDR is not loopback")
	fmt.Fprintln(w, "  --refresh-margin DURATION  — refresh token this long before expiry (default 10m)")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Global flags:")
	fmt.Fprintln(w, "  --debug                    — verbose logging (or KRAUBE_DEBUG=1)")
	fmt.Fprintln(w, "  --out PATH                 — credentials file path (login only)")
	fmt.Fprintln(w, "  --proxy URL                — route all traffic through proxy")
	fmt.Fprintln(w, "                               schemes: http, https, socks5, socks5h")
	fmt.Fprintln(w, "                               (falls back to HTTPS_PROXY/ALL_PROXY env)")
	fmt.Fprintln(w, "  --version, -v              — print the binary version")
	fmt.Fprintln(w, "  --help, -h                 — show this help")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Generation flags (query / stream / default):")
	fmt.Fprintln(w, "  --system TEXT              — system prompt as inline text")
	fmt.Fprintln(w, "  --system-file PATH         — system prompt read from a file")
	fmt.Fprintln(w, "  --history PATH|-           — prior messages as JSON array")
	fmt.Fprintln(w, "                               (file path, or \"-\" to read from stdin)")
	fmt.Fprintln(w, "                               format: [{\"role\":\"user|assistant\",")
	fmt.Fprintln(w, "                                         \"content\":\"...\"}, ...]")
	fmt.Fprintln(w, "  --model NAME               — model id (default claude-sonnet-4-6)")
	fmt.Fprintln(w, "  --max-tokens N             — response cap in tokens (default 4096)")
	fmt.Fprintln(w, "  --temperature F            — sampling temperature 0.0..1.0")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Env:")
	fmt.Fprintln(w, "  KRAUBE_CREDENTIALS_PATH    — override credentials file path globally")
	fmt.Fprintln(w, "  HTTPS_PROXY / ALL_PROXY    — proxy URL (used when --proxy not given)")
}

// cmdValueFlags lists the command-specific value flags that are parsed after
// dispatch (cmdLogin / cmdServe read them via flagValue), so they
// legitimately survive until the unknown-flag check and must be skipped
// together with their value.
var cmdValueFlags = map[string][]string{
	"login": {"--out"},
	"serve": {"--listen", "--auth-key", "--refresh-margin"},
}

// preDispatchValueFlags are the value flags stripped before dispatch (by
// flagValue in main and parseGenFlags). flagValue only strips a flag together
// with its value, so one still present at check time means either the value
// is missing (flag is the last argument) or the flag was given twice.
var preDispatchValueFlags = map[string]bool{
	"--proxy":       true,
	"--system":      true,
	"--system-file": true,
	"--history":     true,
	"--model":       true,
	"--max-tokens":  true,
	"--temperature": true,
}

// residualFlagError inspects the arguments left over after all pre-dispatch
// flag stripping and returns a user-facing error for the first flag-like
// token that nothing will ever consume, or "" when the residual arguments
// are clean. A bare "-" is not a flag: it is a valid flag value (`--history -`)
// and never survives stripping in that role, so a leftover one is treated as
// positional text.
func residualFlagError(args []string) string {
	allowed := map[string]bool{}
	if len(args) > 0 {
		for _, f := range cmdValueFlags[args[0]] {
			allowed[f] = true
		}
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case allowed[a]:
			i++ // skip the flag's value too
		case a == "-" || !strings.HasPrefix(a, "-"):
			// Positional argument (prompt text) — the declared interface.
		case preDispatchValueFlags[a]:
			if i == len(args)-1 {
				return fmt.Sprintf("flag %s requires a value", a)
			}
			return fmt.Sprintf("flag %s given more than once", a)
		default:
			return fmt.Sprintf("unknown flag: %s", a)
		}
	}
	return ""
}

// genFlags holds optional generation parameters parsed from the CLI.
// Zero values mean "use library defaults".
type genFlags struct {
	system      string
	systemSet   bool // true if --system or --system-file was provided
	history     []kraube.Message
	model       string   // empty = ModelSonnet4_6
	maxTokens   int      // 0 = 4096
	temperature *float64 // nil = library default
}

// parseGenFlags reads --system / --system-file / --history / --model /
// --max-tokens / --temperature out of os.Args and returns the resolved
// values. Flags are stripped from os.Args so command dispatch keeps working
// against just the positional arguments.
//
// Fails the process on parse errors (unreadable history file, malformed JSON,
// invalid number) so the user sees the problem immediately rather than a
// surprising API response.
func parseGenFlags() genFlags {
	var g genFlags

	if v := flagValue("--system"); v != "" {
		g.system = v
		g.systemSet = true
	}
	if v := flagValue("--system-file"); v != "" {
		data, err := os.ReadFile(v)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read --system-file: %v\n", err)
			os.Exit(1)
		}
		g.system = string(data)
		g.systemSet = true
	}

	if v := flagValue("--history"); v != "" {
		var raw []byte
		var err error
		if v == "-" {
			raw, err = io.ReadAll(os.Stdin)
		} else {
			raw, err = os.ReadFile(v)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read --history: %v\n", err)
			os.Exit(1)
		}
		if len(raw) > 0 {
			var msgs []kraube.Message
			if err := json.Unmarshal(raw, &msgs); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to parse --history JSON: %v\n", err)
				os.Exit(1)
			}
			g.history = msgs
		}
	}

	if v := flagValue("--model"); v != "" {
		g.model = v
	}
	if v := flagValue("--max-tokens"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			fmt.Fprintf(os.Stderr, "Invalid --max-tokens: %q (must be positive integer)\n", v)
			os.Exit(1)
		}
		g.maxTokens = n
	}
	if v := flagValue("--temperature"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid --temperature: %q\n", v)
			os.Exit(1)
		}
		g.temperature = kraube.Float64(f)
	}

	return g
}

// buildRequest assembles a MessageRequest from the user-supplied prompt and
// the resolved generation flags. The prompt is appended as the final user
// message; --history messages (if any) come first; --system populates
// MessageRequest.System.
func buildRequest(prompt string, g genFlags) *kraube.MessageRequest {
	model := g.model
	if model == "" {
		model = kraube.ModelSonnet4_6
	}
	maxTokens := g.maxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	messages := make([]kraube.Message, 0, len(g.history)+1)
	messages = append(messages, g.history...)
	messages = append(messages, kraube.UserMessage(prompt))

	req := &kraube.MessageRequest{
		Model:       model,
		MaxTokens:   maxTokens,
		Messages:    messages,
		Temperature: g.temperature,
	}
	if g.systemSet {
		req.System = kraube.SystemText(g.system)
	}
	return req
}

func cmdLogin(ctx context.Context) {
	path := flagValue("--out")
	if path == "" {
		path = kraube.DefaultCredentialsPath()
	}

	// Manual OAuth flow: print URL, user pastes code
	creds, err := kraube.LoginManual(ctx, func(authURL string) (string, error) {
		fmt.Println("Open this URL in your browser:")
		fmt.Println()
		fmt.Println("  " + authURL)
		fmt.Println()
		fmt.Print("Paste the code here: ")
		reader := bufio.NewReader(os.Stdin)
		code, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return code, nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Login failed: %v\n", err)
		os.Exit(1)
	}

	if err := kraube.SaveCredentials(path, creds); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save credentials: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Authenticated! Credentials saved to %s\n", path)
}

func cmdUsage(ctx context.Context) {
	client := mustClient(ctx)

	// Try cached data first.
	rl, _ := kraube.LoadRateLimit(kraube.DefaultRateLimitPath())

	// If no cache or stale (>5 min), make a cheap API call to refresh.
	// Rate limit headers only come from /v1/messages, not /count_tokens.
	if rl == nil || time.Since(rl.UpdatedAt) > 5*time.Minute {
		_, _ = client.Messages.Create(ctx, &kraube.MessageRequest{
			Model:     kraube.ModelSonnet4_6,
			MaxTokens: 1,
			Messages:  []kraube.Message{kraube.UserMessage("hi")},
		})
		// Reload from client (set by do()) or from disk (set by parseAPIError).
		if client.RateLimit != nil && client.RateLimit.HasData() {
			rl = client.RateLimit
		}
	}

	if rl == nil || !rl.HasData() {
		fmt.Println("No usage data available.")
		fmt.Println("Usage data appears after a successful API call (subscription plans only).")
		return
	}

	// Show staleness
	ago := time.Since(rl.UpdatedAt).Truncate(time.Second)
	fmt.Printf("Plan Usage (updated %s ago)\n", ago)

	fmt.Println()

	if rl.FiveHour != nil {
		printUsageBar("Session (5h)", rl.FiveHour)
	}
	if rl.SevenDay != nil {
		printUsageBar("Weekly  (7d)", rl.SevenDay)
	}

	if rl.FiveHour == nil && rl.SevenDay == nil {
		fmt.Println("No usage data available. Usage is only available for subscription plans.")
		return
	}

	if rl.Status == "rejected" {
		fmt.Println()
		claim := rl.Claim
		if claim == "" {
			claim = "usage limit"
		}
		fmt.Printf("You've hit your %s.\n", formatClaim(claim))
	}
}

func printUsageBar(label string, w *kraube.RateLimitWindow) {
	pct := w.UsedPercent()
	barWidth := 30
	filled := int(pct / 100 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	reset := formatDuration(w.TimeUntilReset())
	fmt.Printf("  %s  [%s] %5.1f%%  resets in %s\n", label, bar, pct, reset)
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "now"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func formatClaim(claim string) string {
	switch claim {
	case "five_hour":
		return "session limit"
	case "seven_day":
		return "weekly limit"
	case "seven_day_opus":
		return "Opus limit"
	case "seven_day_sonnet":
		return "Sonnet limit"
	case "overage":
		return "extra usage limit"
	default:
		return claim
	}
}

// cmdServe runs the local HTTP daemon: a proxy to the Anthropic Messages
// API with a background token keepalive. Intended to run permanently (e.g.
// under systemd — see deploy/kraube-serve.service) as the single owner of
// the credentials file, so short-lived callers never race the single-use
// refresh token.
func cmdServe(ctx context.Context) {
	cfg := kraube.ServerConfig{
		Addr:    flagValue("--listen"),
		AuthKey: flagValue("--auth-key"),
	}
	if cfg.AuthKey == "" {
		cfg.AuthKey = os.Getenv("KRAUBE_SERVE_KEY")
	}
	if v := flagValue("--refresh-margin"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			fmt.Fprintf(os.Stderr, "Invalid --refresh-margin: %q (want a positive duration like 10m)\n", v)
			os.Exit(1)
		}
		cfg.RefreshMargin = d
	}

	client := mustClient(ctx)

	srv, err := kraube.NewServer(client, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Graceful shutdown on SIGINT/SIGTERM: stop the keepalive, drain
	// in-flight requests (10s window inside Run), then exit cleanly.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("kraube serve listening on http://%s\n", srv.Addr())
	if err := srv.Wait(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("kraube serve stopped")
}

func cmdQuery(ctx context.Context, prompt string, g genFlags) {
	client := mustClient(ctx)

	resp, err := client.Messages.Create(ctx, buildRequest(prompt, g))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(resp.Text())
}

func cmdStream(ctx context.Context, prompt string, g genFlags) {
	client := mustClient(ctx)

	stream, err := client.Messages.Stream(ctx, buildRequest(prompt, g))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = stream.Close() }()

	for stream.Next() {
		if evt, ok := stream.Event().(*kraube.ContentBlockDeltaEvent); ok {
			if evt.Delta.Type == "text_delta" {
				fmt.Print(evt.Delta.Text)
				// Flush так чтобы caller (например Python subprocess) видел
				// токены сразу, а не дожидался полного буфера stdout.
				_ = os.Stdout.Sync()
			}
		}
	}
	if err := stream.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "\nStream error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println()
}

func mustClient(ctx context.Context) *kraube.Client {
	opts := []kraube.Option{kraube.WithTokenFile("")}
	if cliProxy != "" {
		opts = append(opts, kraube.WithProxy(cliProxy))
	}
	client, err := kraube.NewClient(ctx, opts...)
	if err != nil {
		// "Run: kraube login" is only honest advice when the credentials are
		// actually absent/dead — a network hiccup or an unwritable file must
		// not push the user into a pointless re-login.
		fmt.Fprintf(os.Stderr, "Not authenticated: %v\n", err)
		fmt.Fprintf(os.Stderr, "If credentials are missing or revoked, run: kraube login\n")
		os.Exit(1)
	}
	return client
}

// applyAuthProxy installs a package-level HTTP client for kraube's OAuth
// endpoints, so that `kraube login`, token refresh and standalone
// FetchProfile calls all go through the requested proxy. The auth client
// uses the same chrome-fingerprinted transport as the main API client to
// keep behavior consistent across endpoints.
func applyAuthProxy(proxyURL string) error {
	hc, err := kraube.NewProxiedHTTPClient(proxyURL)
	if err != nil {
		return err
	}
	kraube.SetAuthHTTPClient(hc)
	return nil
}

// hasFlag reports whether a boolean flag is present, removing every
// occurrence from os.Args so it does not interfere with command dispatch or
// trip the residual unknown-flag check.
func hasFlag(flag string) bool {
	found := false
	kept := os.Args[:0]
	for _, arg := range os.Args {
		if arg == flag {
			found = true
			continue
		}
		kept = append(kept, arg)
	}
	os.Args = kept
	return found
}

// flagValue extracts and removes a "--key VALUE" pair from os.Args.
// Returns the value, or "" if the flag is not present.
func flagValue(flag string) string {
	for i, arg := range os.Args {
		if arg == flag && i+1 < len(os.Args) {
			value := os.Args[i+1]
			os.Args = append(os.Args[:i], os.Args[i+2:]...)
			return value
		}
	}
	return ""
}
