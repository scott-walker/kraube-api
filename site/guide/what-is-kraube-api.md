# What is Kraube API?

Kraube API is a lightweight Go gateway for accessing the Anthropic Messages API through your Claude Pro/Max/Team subscription via OAuth.

## Why

The official Anthropic Go SDK requires a paid API key. If you have a Claude subscription, you're already paying — but there's no official library to use your subscription programmatically from Go.

Kraube API solves this by replicating the Claude Code CLI's HTTP protocol: billing headers, metadata, beta headers, and Chrome TLS fingerprinting.

## Key Principles

### OAuth-only

The only authentication method is OAuth Bearer token via claude.ai subscription. No API keys.

### Stateless

The client doesn't know where tokens come from. Everything goes through the `TokenProvider` interface — a single method. File, env variable, Vault, Redis, callback — any source works.

### Lightweight

Minimal wrapper over HTTP. Not a framework, not an SDK with abstractions — just a typed HTTP client. You bring your own architecture.

### Reverse-engineered

The protocol is taken from reverse engineering the Claude Code CLI binary. When something isn't documented in the Anthropic API docs, the CLI binary is the source of truth.

## What it supports

- Streaming and non-streaming requests
- Tool use (custom + built-in: web search, code execution, text editor, bash)
- Extended thinking (enabled, adaptive, disabled)
- Vision (images via URL and base64)
- Documents
- System prompts with caching
- Structured output (JSON Schema)
- Token counting
- Rate limit tracking
