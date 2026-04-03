package konnektor

// Options configures how a Session is created and how the CLI process is spawned.
type Options struct {
	// Model to use (e.g. "claude-sonnet-4-5", "claude-opus-4-5").
	Model string

	// FallbackModel to use if primary model fails.
	FallbackModel string

	// SystemPrompt overrides the system prompt.
	SystemPrompt string

	// AppendSystemPrompt appends to the default system prompt instead of replacing.
	AppendSystemPrompt string

	// MaxTurns limits the number of agentic turns.
	MaxTurns int

	// MaxBudgetUSD limits the total cost.
	MaxBudgetUSD float64

	// PermissionMode controls tool permission behavior.
	PermissionMode PermissionMode

	// Tools is the list of tools to enable. Use "default" for default set, "" for none.
	Tools []string

	// AllowedTools restricts which tools can be used.
	AllowedTools []string

	// DisallowedTools prevents specific tools from being used.
	DisallowedTools []string

	// CWD sets the working directory for the CLI process.
	CWD string

	// SessionID sets a specific session ID.
	SessionID string

	// ResumeSession resumes a specific session by ID.
	ResumeSession string

	// ContinueSession resumes the most recent session.
	ContinueSession bool

	// ForkSession creates a new session branched from the resumed session.
	ForkSession bool

	// MCPServers configures MCP servers. Can be a JSON string or map.
	MCPServers map[string]any

	// MCPConfigPath is a path to an MCP config file.
	MCPConfigPath string

	// AddDirs adds additional directories to the session.
	AddDirs []string

	// IncludePartialMessages enables streaming partial message events.
	IncludePartialMessages bool

	// Effort controls reasoning effort: "low", "medium", "high", "max".
	Effort string

	// MaxThinkingTokens sets the thinking token budget (0 = disabled).
	MaxThinkingTokens int

	// Betas enables beta features.
	Betas []string

	// JSONSchema enables structured output with a JSON schema.
	JSONSchema string

	// Env sets additional environment variables for the CLI process.
	Env map[string]string

	// CLIPath overrides the path to the claude binary.
	CLIPath string

	// PermissionHandler is called when the CLI requests tool permission.
	// Return PermissionAllow or PermissionDeny. If nil, all tools are auto-allowed.
	PermissionHandler func(req ToolPermissionRequest) PermissionResponse

	// SettingSources controls which setting sources to use (e.g. "user", "project", "local").
	SettingSources []string

	// NoSessionPersistence disables session file persistence.
	NoSessionPersistence bool

	// PluginDirs adds plugin directories.
	PluginDirs []string
}

// buildArgs constructs CLI arguments from Options.
func (o *Options) buildArgs() []string {
	args := []string{
		"--output-format", "stream-json",
		"--verbose",
	}

	if o.Model != "" {
		args = append(args, "--model", o.Model)
	}
	if o.FallbackModel != "" {
		args = append(args, "--fallback-model", o.FallbackModel)
	}
	if o.SystemPrompt != "" {
		args = append(args, "--system-prompt", o.SystemPrompt)
	}
	if o.AppendSystemPrompt != "" {
		args = append(args, "--append-system-prompt", o.AppendSystemPrompt)
	}
	if o.MaxTurns > 0 {
		args = append(args, "--max-turns", itoa(o.MaxTurns))
	}
	if o.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", ftoa(o.MaxBudgetUSD))
	}
	if o.PermissionMode != "" {
		args = append(args, "--permission-mode", string(o.PermissionMode))
	}
	if len(o.Tools) > 0 {
		args = append(args, "--tools", join(o.Tools, ","))
	}
	if len(o.AllowedTools) > 0 {
		args = append(args, "--allowedTools", join(o.AllowedTools, ","))
	}
	if len(o.DisallowedTools) > 0 {
		args = append(args, "--disallowedTools", join(o.DisallowedTools, ","))
	}
	if o.SessionID != "" {
		args = append(args, "--session-id", o.SessionID)
	}
	if o.ResumeSession != "" {
		args = append(args, "--resume", o.ResumeSession)
	}
	if o.ContinueSession {
		args = append(args, "--continue")
	}
	if o.ForkSession {
		args = append(args, "--fork-session")
	}
	if o.MCPConfigPath != "" {
		args = append(args, "--mcp-config", o.MCPConfigPath)
	}
	if len(o.MCPServers) > 0 {
		args = append(args, "--mcp-config", marshalJSON(map[string]any{
			"mcpServers": o.MCPServers,
		}))
	}
	for _, dir := range o.AddDirs {
		args = append(args, "--add-dir", dir)
	}
	if o.IncludePartialMessages {
		args = append(args, "--include-partial-messages")
	}
	if o.Effort != "" {
		args = append(args, "--effort", o.Effort)
	}
	if o.MaxThinkingTokens > 0 {
		args = append(args, "--max-thinking-tokens", itoa(o.MaxThinkingTokens))
	}
	if len(o.Betas) > 0 {
		args = append(args, "--betas", join(o.Betas, ","))
	}
	if o.JSONSchema != "" {
		args = append(args, "--json-schema", o.JSONSchema)
	}
	if len(o.SettingSources) > 0 {
		args = append(args, "--setting-sources", join(o.SettingSources, ","))
	}
	if o.NoSessionPersistence {
		args = append(args, "--no-session-persistence")
	}
	for _, dir := range o.PluginDirs {
		args = append(args, "--plugin-dir", dir)
	}
	if o.PermissionHandler != nil {
		args = append(args, "--permission-prompt-tool", "stdio")
	}

	// Always last
	args = append(args, "--input-format", "stream-json")

	return args
}
