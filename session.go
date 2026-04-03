package konnektor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// Session represents an active connection to a claude CLI process.
type Session struct {
	transport *transport
	opts      *Options

	sessionID string
	initMsg   *SystemInitMessage

	reqCounter atomic.Int64
	pendingMu  sync.Mutex
	pending    map[string]chan *ControlResponse

	ctx    context.Context
	cancel context.CancelFunc
}

// newSession creates a Session, spawns the process, and runs the initialize handshake.
func newSession(ctx context.Context, opts *Options) (*Session, error) {
	ctx, cancel := context.WithCancel(ctx)

	t, err := newTransport(ctx, opts)
	if err != nil {
		cancel()
		return nil, err
	}

	s := &Session{
		transport: t,
		opts:      opts,
		pending:   make(map[string]chan *ControlResponse),
		ctx:       ctx,
		cancel:    cancel,
	}

	// Send initialize control request
	reqID := s.nextRequestID()
	initReq := ControlRequest{
		Type:      MessageTypeControlRequest,
		RequestID: reqID,
		Request: ControlRequestInner{
			Subtype: ControlSubtypeInitialize,
		},
	}

	if err := t.send(initReq); err != nil {
		t.kill()
		cancel()
		return nil, fmt.Errorf("send initialize: %w", err)
	}

	// Wait for initialize control_response
	for {
		line, err := t.readLine()
		if err != nil {
			t.kill()
			cancel()
			return nil, fmt.Errorf("read initialize response: %w", err)
		}

		var raw struct {
			Type    MessageType `json:"type"`
			Subtype string     `json:"subtype,omitempty"`
		}
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		if raw.Type == MessageTypeControlResponse {
			var resp ControlResponse
			if err := json.Unmarshal(line, &resp); err != nil {
				t.kill()
				cancel()
				return nil, fmt.Errorf("parse control response: %w", err)
			}
			if resp.Response.RequestID == reqID {
				if resp.Response.Subtype == ControlSubtypeError {
					t.kill()
					cancel()
					return nil, fmt.Errorf("initialize error: %s", resp.Response.Error)
				}
				break
			}
		}

		// May receive system init before control_response
		if raw.Type == MessageTypeSystem && raw.Subtype == string(SystemSubtypeInit) {
			var init SystemInitMessage
			if err := json.Unmarshal(line, &init); err == nil {
				s.initMsg = &init
				s.sessionID = init.SessionID
			}
		}
	}

	return s, nil
}

// SessionID returns the CLI-assigned session ID.
func (s *Session) SessionID() string {
	return s.sessionID
}

// InitMessage returns the system init message received during handshake.
func (s *Session) InitMessage() *SystemInitMessage {
	return s.initMsg
}

// Query sends a prompt and returns a channel of Messages.
// The channel is closed when the turn completes (result message received) or on error.
func (s *Session) Query(prompt string) (<-chan *Message, error) {
	msg := UserMessage{
		Type:      MessageTypeUser,
		SessionID: s.sessionID,
		Message: MessageContent{
			Role:    "user",
			Content: prompt,
		},
		ParentToolUseID: nil,
	}

	if err := s.transport.send(msg); err != nil {
		return nil, fmt.Errorf("send query: %w", err)
	}

	ch := make(chan *Message, 64)
	go s.readLoop(ch)
	return ch, nil
}

// QueryMultiContent sends a multi-content message (e.g. with images) and returns a channel.
func (s *Session) QueryMultiContent(content []ContentBlock) (<-chan *Message, error) {
	msg := UserMessage{
		Type:      MessageTypeUser,
		SessionID: s.sessionID,
		Message: MessageContent{
			Role:    "user",
			Content: content,
		},
		ParentToolUseID: nil,
	}

	if err := s.transport.send(msg); err != nil {
		return nil, fmt.Errorf("send query: %w", err)
	}

	ch := make(chan *Message, 64)
	go s.readLoop(ch)
	return ch, nil
}

// Interrupt sends an interrupt signal to stop the current generation.
func (s *Session) Interrupt() error {
	reqID := s.nextRequestID()
	return s.transport.send(ControlRequest{
		Type:      MessageTypeControlRequest,
		RequestID: reqID,
		Request: ControlRequestInner{
			Subtype: ControlSubtypeInterrupt,
		},
	})
}

// Close terminates the session and kills the CLI process.
func (s *Session) Close() error {
	s.cancel()
	s.transport.kill()
	return s.transport.wait()
}

// readLoop reads messages from stdout and dispatches them.
func (s *Session) readLoop(ch chan<- *Message) {
	defer close(ch)

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		line, err := s.transport.readLine()
		if err != nil {
			if err == io.EOF {
				return
			}
			return
		}

		msg, err := parseMessage(line)
		if err != nil {
			continue
		}

		// Handle control protocol messages internally
		if msg.Type == MessageTypeControlRequest {
			s.handleControlRequest(msg)
			continue
		}

		if msg.Type == MessageTypeControlResponse {
			s.handleControlResponse(msg)
			continue
		}

		if msg.Type == MessageTypeControlCancel {
			// Cancel any pending request
			continue
		}

		// Capture session ID from messages
		if msg.SystemInit != nil && s.sessionID == "" {
			s.sessionID = msg.SystemInit.SessionID
			s.initMsg = msg.SystemInit
		}
		if msg.Result != nil && s.sessionID == "" {
			s.sessionID = msg.Result.SessionID
		}

		// Forward to consumer
		select {
		case ch <- msg:
		case <-s.ctx.Done():
			return
		}

		// Result message ends the turn
		if msg.Type == MessageTypeResult {
			return
		}
	}
}

// handleControlRequest processes control requests from the CLI (e.g. permission prompts).
func (s *Session) handleControlRequest(msg *Message) {
	if msg.ControlRequest == nil {
		return
	}

	req := msg.ControlRequest

	switch req.Request.Subtype {
	case ControlSubtypeCanUseTool:
		s.handlePermissionRequest(req)
	default:
		// Auto-respond with success for unhandled subtypes
		s.transport.send(ControlResponse{
			Type: MessageTypeControlResponse,
			Response: ControlResponseInner{
				Subtype:   ControlSubtypeSuccess,
				RequestID: req.RequestID,
			},
		})
	}
}

// handlePermissionRequest responds to tool permission requests.
func (s *Session) handlePermissionRequest(req *ControlRequest) {
	if s.opts.PermissionHandler != nil {
		// Parse the tool permission details from the raw request
		raw, _ := json.Marshal(req.Request)
		var toolReq ToolPermissionRequest
		json.Unmarshal(raw, &toolReq)

		resp := s.opts.PermissionHandler(toolReq)

		s.transport.send(ControlResponse{
			Type: MessageTypeControlResponse,
			Response: ControlResponseInner{
				Subtype:   ControlSubtypeSuccess,
				RequestID: req.RequestID,
				Response: map[string]any{
					"behavior": resp.Behavior,
					"message":  resp.Message,
				},
			},
		})
	} else {
		// Auto-allow if no handler
		s.transport.send(ControlResponse{
			Type: MessageTypeControlResponse,
			Response: ControlResponseInner{
				Subtype:   ControlSubtypeSuccess,
				RequestID: req.RequestID,
				Response: map[string]any{
					"behavior": PermissionAllow,
				},
			},
		})
	}
}

// handleControlResponse routes control responses to pending requests.
func (s *Session) handleControlResponse(msg *Message) {
	if msg.ControlResponse == nil {
		return
	}

	reqID := msg.ControlResponse.Response.RequestID
	s.pendingMu.Lock()
	ch, ok := s.pending[reqID]
	if ok {
		delete(s.pending, reqID)
	}
	s.pendingMu.Unlock()

	if ok {
		ch <- msg.ControlResponse
	}
}

// nextRequestID generates a unique request ID.
func (s *Session) nextRequestID() string {
	n := s.reqCounter.Add(1)
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("req_%d_%s", n, hex.EncodeToString(b))
}
