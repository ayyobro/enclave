package vibe

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// OutputEvent represents a parsed event from Claude Code's stream-json output.
type OutputEvent struct {
	Type    string // "init", "assistant", "tool_use", "result", "error"
	Text    string // extracted text content
	IsError bool
	IsDone  bool
	Raw     map[string]interface{} // full JSON for advanced use
}

// Session manages a Claude Code subprocess for collaborative coding.
type Session struct {
	repoPath  string
	sessionID string // Claude Code session ID for --resume
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	active    bool
	mu        sync.Mutex

	// OutputCh receives parsed events from Claude Code
	OutputCh chan OutputEvent
}

// NewSession creates a new collaborative coding session.
func NewSession(repoPath string) *Session {
	return &Session{
		repoPath: repoPath,
		OutputCh: make(chan OutputEvent, 64),
	}
}

// IsActive returns whether a session is currently running.
func (s *Session) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

// RepoPath returns the session's working directory.
func (s *Session) RepoPath() string {
	return s.repoPath
}

// Activate marks the session as ready to accept prompts.
func (s *Session) Activate() {
	s.mu.Lock()
	s.active = true
	s.mu.Unlock()
}

// SendPrompt sends a prompt to the running Claude Code session.
// Each prompt is a separate `claude --print` invocation that continues the session.
func (s *Session) SendPrompt(prompt string, sender string) error {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return fmt.Errorf("no active session")
	}
	s.mu.Unlock()

	// Build the prompt with attribution
	fullPrompt := fmt.Sprintf("[%s]: %s", sender, prompt)

	// Build command args
	args := []string{
		"--print",
		"--output-format", "stream-json",
		"--verbose",
		"--dangerously-skip-permissions",
	}

	// Resume the session if we have a session ID
	s.mu.Lock()
	if s.sessionID != "" {
		args = append(args, "--resume", s.sessionID)
	}
	s.mu.Unlock()

	args = append(args, fullPrompt)

	cmd := exec.Command("claude", args...)
	cmd.Dir = s.repoPath

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting claude: %w", err)
	}

	// Parse output in background
	go func() {
		s.parseOutput(stdout)
		cmd.Wait()
	}()

	return nil
}

// Start initializes the session with a first prompt.
func (s *Session) Start(initialPrompt string, sender string) error {
	s.mu.Lock()
	s.active = true
	s.mu.Unlock()

	return s.SendPrompt(initialPrompt, sender)
}

// Stop ends the session.
func (s *Session) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = false
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
}

// parseOutput reads stream-json lines from Claude Code and emits OutputEvents.
func (s *Session) parseOutput(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer for large outputs

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		event := s.parseEvent(raw)
		if event != nil {
			s.OutputCh <- *event
		}
	}
}

func (s *Session) parseEvent(raw map[string]interface{}) *OutputEvent {
	eventType, _ := raw["type"].(string)

	switch eventType {
	case "system":
		// Extract session ID from init event
		subtype, _ := raw["subtype"].(string)
		if subtype == "init" {
			if sid, ok := raw["session_id"].(string); ok {
				s.mu.Lock()
				s.sessionID = sid
				s.mu.Unlock()
			}
			return &OutputEvent{
				Type: "init",
				Text: "Claude Code session initialized",
				Raw:  raw,
			}
		}
		return nil

	case "assistant":
		// Extract text content from the message
		text := extractAssistantText(raw)
		if text == "" {
			return nil
		}
		return &OutputEvent{
			Type: "assistant",
			Text: text,
			Raw:  raw,
		}

	case "result":
		subtype, _ := raw["subtype"].(string)
		isError := subtype != "success"

		// Don't include result text — it duplicates the assistant message.
		// Only surface errors.
		text := ""
		if isError {
			text, _ = raw["result"].(string)
		}

		return &OutputEvent{
			Type:    "result",
			Text:    text,
			IsError: isError,
			IsDone:  true,
			Raw:     raw,
		}

	default:
		return nil
	}
}

// extractAssistantText pulls the text content from an assistant message event.
func extractAssistantText(raw map[string]interface{}) string {
	msg, ok := raw["message"].(map[string]interface{})
	if !ok {
		return ""
	}
	content, ok := msg["content"].([]interface{})
	if !ok {
		return ""
	}

	var parts []string
	for _, c := range content {
		block, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		blockType, _ := block["type"].(string)
		switch blockType {
		case "text":
			if text, ok := block["text"].(string); ok && text != "" {
				parts = append(parts, text)
			}
		case "tool_use":
			name, _ := block["name"].(string)
			// Show tool usage as a brief status
			if name != "" {
				input, _ := block["input"].(map[string]interface{})
				desc := formatToolUse(name, input)
				parts = append(parts, desc)
			}
		}
	}

	return strings.Join(parts, "\n")
}

// formatToolUse creates a concise description of a tool call.
func formatToolUse(name string, input map[string]interface{}) string {
	switch name {
	case "Read":
		path, _ := input["file_path"].(string)
		return fmt.Sprintf("[reading %s]", path)
	case "Edit":
		path, _ := input["file_path"].(string)
		return fmt.Sprintf("[editing %s]", path)
	case "Write":
		path, _ := input["file_path"].(string)
		return fmt.Sprintf("[writing %s]", path)
	case "Bash":
		cmd, _ := input["command"].(string)
		if len(cmd) > 60 {
			cmd = cmd[:60] + "..."
		}
		return fmt.Sprintf("[running: %s]", cmd)
	case "Glob":
		pattern, _ := input["pattern"].(string)
		return fmt.Sprintf("[searching: %s]", pattern)
	case "Grep":
		pattern, _ := input["pattern"].(string)
		return fmt.Sprintf("[grep: %s]", pattern)
	default:
		return fmt.Sprintf("[%s]", name)
	}
}
