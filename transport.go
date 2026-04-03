package konnektor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// transport manages the claude CLI subprocess.
type transport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	stderr io.ReadCloser

	mu     sync.Mutex
	closed bool
}

// newTransport spawns the claude CLI process with the given options.
func newTransport(ctx context.Context, opts *Options) (*transport, error) {
	cliPath := opts.CLIPath
	if cliPath == "" {
		var err error
		cliPath, err = findCLI()
		if err != nil {
			return nil, fmt.Errorf("claude CLI not found: %w", err)
		}
	}

	args := opts.buildArgs()
	cmd := exec.CommandContext(ctx, cliPath, args...)

	// Set up environment
	env := os.Environ()
	// Filter out CLAUDECODE key
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if len(e) > 10 && e[:10] == "CLAUDECODE" {
			continue
		}
		filtered = append(filtered, e)
	}
	filtered = append(filtered, "CLAUDE_CODE_ENTRYPOINT=sdk-go")
	for k, v := range opts.Env {
		filtered = append(filtered, k+"="+v)
	}
	cmd.Env = filtered

	if opts.CWD != "" {
		cmd.Dir = opts.CWD
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer

	return &transport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: scanner,
		stderr: stderr,
	}, nil
}

// send writes a JSON message to the CLI's stdin.
func (t *transport) send(msg any) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return fmt.Errorf("transport closed")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	data = append(data, '\n')
	_, err = t.stdin.Write(data)
	return err
}

// readLine reads the next JSON line from stdout. Returns nil, io.EOF at end.
func (t *transport) readLine() ([]byte, error) {
	if t.stdout.Scan() {
		line := t.stdout.Bytes()
		// Skip non-JSON lines
		if len(line) == 0 || line[0] != '{' {
			return t.readLine()
		}
		// Copy to avoid scanner buffer reuse
		cp := make([]byte, len(line))
		copy(cp, line)
		return cp, nil
	}
	if err := t.stdout.Err(); err != nil {
		return nil, err
	}
	return nil, io.EOF
}

// closeStdin closes only stdin, allowing the process to finish.
func (t *transport) closeStdin() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed {
		t.closed = true
		t.stdin.Close()
	}
}

// kill terminates the process.
func (t *transport) kill() {
	t.closeStdin()
	if t.cmd.Process != nil {
		t.cmd.Process.Kill()
	}
}

// wait waits for the process to exit.
func (t *transport) wait() error {
	return t.cmd.Wait()
}

// findCLI searches for the claude binary in common locations.
func findCLI() (string, error) {
	// Check PATH first
	if path, err := exec.LookPath("claude"); err == nil {
		return path, nil
	}

	// Check common locations
	home, _ := os.UserHomeDir()
	candidates := []string{
		home + "/.npm-global/bin/claude",
		home + "/.local/bin/claude",
		"/usr/local/bin/claude",
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}

	return "", fmt.Errorf("claude binary not found in PATH or common locations")
}
