package tui

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"enclave/internal/theme"
)

// ChatMessage represents a single rendered message in the chat.
type ChatMessage struct {
	DBMessageID int64  // local database ID for reactions/pins
	From        string
	FromName    string
	Text        string
	Timestamp   time.Time
	ExpiresAt   time.Time // zero means no expiry
	IsOwn       bool
	IsSystem    bool
	Read        bool
	Reactions   []MessageReaction
	Pinned      bool
}

// MessageReaction is an emoji reaction on a message.
type MessageReaction struct {
	FromName string
	Emoji    string
}

// CodeBlock stores a code block's raw content for clipboard copying.
type CodeBlock struct {
	Index int
	Lang  string
	Code  string
}

// ChatViewModel renders the scrollable message history.
type ChatViewModel struct {
	viewport    viewport.Model
	messages    []ChatMessage
	codeBlocks  []CodeBlock
	peerName    string
	peerKey     string
	focused     bool
	width       int
	height      int
	typing      bool
	typingFrom  string
	typingTimer time.Time
}

func NewChatViewModel() ChatViewModel {
	vp := viewport.New(0, 0)
	vp.SetContent("")
	return ChatViewModel{
		viewport: vp,
	}
}

func (m *ChatViewModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.viewport.Width = w - 2  // account for border
	m.viewport.Height = h - 3 // account for border + title
	m.refreshContent()
}

func (m *ChatViewModel) SetFocused(f bool) {
	m.focused = f
}

func (m *ChatViewModel) SetPeer(name, key string) {
	m.peerName = name
	m.peerKey = key
	m.messages = nil
	m.refreshContent()
}

func (m *ChatViewModel) AddMessage(msg ChatMessage) {
	m.messages = append(m.messages, msg)
	m.refreshContent()
	m.viewport.GotoBottom()
}

// SetLastMessageExpiry sets the ExpiresAt on the most recent non-system message.
func (m *ChatViewModel) SetLastMessageExpiry(expiresAt time.Time) {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if !m.messages[i].IsSystem {
			m.messages[i].ExpiresAt = expiresAt
			return
		}
	}
}

func (m *ChatViewModel) AddSystemMessage(text string) {
	m.messages = append(m.messages, ChatMessage{
		Text:      text,
		Timestamp: time.Now(),
		IsSystem:  true,
	})
	m.refreshContent()
	m.viewport.GotoBottom()
}

// MarkRead marks sent messages at or before the given timestamp as read.
func (m *ChatViewModel) MarkRead(messageTS int64) {
	ts := time.Unix(messageTS, 0)
	changed := false
	for i := range m.messages {
		if m.messages[i].IsOwn && !m.messages[i].Read && !m.messages[i].Timestamp.After(ts) {
			m.messages[i].Read = true
			changed = true
		}
	}
	if changed {
		m.refreshContent()
	}
}

// AddReaction adds a reaction to the message closest to the given timestamp.
// Returns the matched message's DB ID (0 if not found).
func (m *ChatViewModel) AddReaction(messageTS int64, fromName, emoji string) int64 {
	ts := time.Unix(messageTS, 0)
	bestIdx := -1
	bestDiff := time.Duration(1<<63 - 1)
	for i := range m.messages {
		if m.messages[i].IsSystem {
			continue
		}
		diff := m.messages[i].Timestamp.Sub(ts)
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			bestDiff = diff
			bestIdx = i
		}
	}
	if bestIdx >= 0 {
		m.messages[bestIdx].Reactions = append(m.messages[bestIdx].Reactions, MessageReaction{
			FromName: fromName,
			Emoji:    emoji,
		})
		m.refreshContent()
		return m.messages[bestIdx].DBMessageID
	}
	return 0
}

// GetLastMessage returns the most recent non-system message (for reactions/pins).
func (m *ChatViewModel) GetLastMessage() *ChatMessage {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if !m.messages[i].IsSystem {
			return &m.messages[i]
		}
	}
	return nil
}

// PurgeExpired removes messages past their ExpiresAt time. Returns true if any were removed.
func (m *ChatViewModel) PurgeExpired() bool {
	now := time.Now()
	var kept []ChatMessage
	removed := false
	for _, msg := range m.messages {
		if !msg.ExpiresAt.IsZero() && now.After(msg.ExpiresAt) {
			removed = true
			continue
		}
		kept = append(kept, msg)
	}
	if removed {
		m.messages = kept
		m.refreshContent()
	}
	return removed
}

func (m *ChatViewModel) SetTyping(fromName string) {
	m.typing = true
	m.typingFrom = fromName
	m.typingTimer = time.Now()
	m.refreshContent()
}

func (m *ChatViewModel) ClearTyping() {
	if m.typing {
		m.typing = false
		m.refreshContent()
	}
}

func (m ChatViewModel) Update(msg tea.Msg) (ChatViewModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.focused {
			m.viewport, cmd = m.viewport.Update(msg)
		}
	default:
		m.viewport, cmd = m.viewport.Update(msg)
	}

	// Clear typing indicator after 3 seconds
	if m.typing && time.Since(m.typingTimer) > 3*time.Second {
		m.typing = false
	}

	return m, cmd
}

// GetCodeBlock returns a code block by 1-based index.
func (m *ChatViewModel) GetCodeBlock(index int) *CodeBlock {
	for i := range m.codeBlocks {
		if m.codeBlocks[i].Index == index {
			return &m.codeBlocks[i]
		}
	}
	return nil
}

// CodeBlockCount returns the number of code blocks in the conversation.
func (m *ChatViewModel) CodeBlockCount() int {
	return len(m.codeBlocks)
}

func (m *ChatViewModel) refreshContent() {
	if m.viewport.Width <= 0 {
		return
	}

	s := theme.NewStyles(theme.Current)
	t := theme.Current
	var lines []string
	m.codeBlocks = nil // rebuild index
	blockNum := 0

	if len(m.messages) == 0 && m.peerName != "" {
		lines = append(lines,
			"",
			s.Subtle.Render("  Start of conversation with "+m.peerName),
			s.Subtle.Render("  Messages are end-to-end encrypted"),
			"",
		)
	}

	for _, msg := range m.messages {
		if msg.IsSystem {
			wrapped := wordWrap(msg.Text, m.viewport.Width-4)
			for _, wl := range strings.Split(wrapped, "\n") {
				lines = append(lines, "  "+s.ErrorMsg.Render("⚠ "+wl))
			}
			lines = append(lines, "")
			continue
		}

		var nameStyle lipgloss.Style
		var name string
		if msg.IsOwn {
			nameStyle = s.OwnNameStyle
			name = "you"
		} else {
			nameStyle = s.PeerNameStyle
			name = msg.FromName
		}

		ts := formatTimestamp(msg.Timestamp)
		// Read receipt indicator for own messages
		readIndicator := ""
		if msg.IsOwn {
			if msg.Read {
				readIndicator = s.OnlineIndicator.Render(" ✓✓")
			} else {
				readIndicator = s.Subtle.Render(" ✓")
			}
		}
		// Pin indicator
		pinIndicator := ""
		if msg.Pinned {
			pinIndicator = lipgloss.NewStyle().Foreground(t.Warning).Render(" 📌")
		}
		// Ephemeral indicator
		ephemeralIndicator := ""
		if !msg.ExpiresAt.IsZero() {
			remaining := time.Until(msg.ExpiresAt)
			if remaining > 0 {
				ephemeralIndicator = s.Subtle.Render(fmt.Sprintf(" ⏱ %s", remaining.Round(time.Second)))
			}
		}

		header := fmt.Sprintf("%s %s%s%s%s", nameStyle.Render(name), s.TimestampStyle.Render("· "+ts), readIndicator, pinIndicator, ephemeralIndicator)

		lines = append(lines, "  "+header)
		rendered := renderMessageText(msg.Text, m.viewport.Width-4, s, t, &blockNum, &m.codeBlocks)
		for _, rl := range rendered {
			lines = append(lines, "  "+rl)
		}

		// Reactions
		if len(msg.Reactions) > 0 {
			var reacts []string
			for _, r := range msg.Reactions {
				reacts = append(reacts, fmt.Sprintf("%s %s", r.Emoji, s.Subtle.Render(r.FromName)))
			}
			lines = append(lines, "  "+strings.Join(reacts, "  "))
		}

		lines = append(lines, "")
	}

	if m.typing {
		lines = append(lines, "  "+s.Subtle.Render(m.typingFrom+" is typing..."))
	}

	m.viewport.SetContent(strings.Join(lines, "\n"))
}

func (m ChatViewModel) View() string {
	t := theme.Current

	borderColor := t.Border
	if m.focused {
		borderColor = t.BorderActive
	}

	title := ""
	if m.peerName != "" {
		titleStyle := lipgloss.NewStyle().Foreground(t.Primary).Bold(true)
		title = titleStyle.Render(" # " + m.peerName)
	} else {
		titleStyle := lipgloss.NewStyle().Foreground(t.ForegroundDim)
		title = titleStyle.Render(" Select a contact to start chatting")
	}

	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(m.width).
		Height(m.height)

	return box.Render(title + "\n" + m.viewport.View())
}

// renderMessageText parses a message for fenced code blocks and inline code,
// then renders each segment with the appropriate style.
func renderMessageText(text string, width int, s theme.Styles, t theme.Theme, blockNum *int, blocks *[]CodeBlock) []string {
	var result []string
	remaining := text

	for len(remaining) > 0 {
		// Look for fenced code block: ```...```
		fenceStart := strings.Index(remaining, "```")
		if fenceStart != -1 {
			// Render text before the fence
			if fenceStart > 0 {
				before := remaining[:fenceStart]
				result = append(result, renderInlineCode(before, width, s, t)...)
			}

			// Find closing fence
			afterOpen := remaining[fenceStart+3:]
			fenceEnd := strings.Index(afterOpen, "```")
			if fenceEnd != -1 {
				codeContent := afterOpen[:fenceEnd]
				lang := ""
				// Extract optional language hint on first line
				if nl := strings.IndexByte(codeContent, '\n'); nl != -1 {
					firstLine := strings.TrimSpace(codeContent[:nl])
					if len(firstLine) <= 20 && !strings.Contains(firstLine, " ") && len(firstLine) > 0 {
						lang = firstLine
						codeContent = codeContent[nl+1:]
					}
				}

				// Register the code block
				*blockNum++
				*blocks = append(*blocks, CodeBlock{
					Index: *blockNum,
					Lang:  lang,
					Code:  codeContent,
				})

				// Render label + code block
				label := fmt.Sprintf("╭─ %s", blockLabel(lang, *blockNum))
				labelStyle := lipgloss.NewStyle().Foreground(t.ForegroundDim)
				result = append(result, labelStyle.Render(label))
				result = append(result, renderCodeBlock(codeContent, lang, width, s)...)
				copyHint := fmt.Sprintf("╰─ /copy %d", *blockNum)
				result = append(result, labelStyle.Render(copyHint))

				remaining = afterOpen[fenceEnd+3:]
				continue
			}
		}

		// No more fenced blocks — render rest with inline code support
		result = append(result, renderInlineCode(remaining, width, s, t)...)
		break
	}

	return result
}

func blockLabel(lang string, index int) string {
	if lang != "" {
		return fmt.Sprintf("%s [%d]", lang, index)
	}
	return fmt.Sprintf("code [%d]", index)
}

// renderCodeBlock renders a fenced code block with syntax highlighting.
func renderCodeBlock(code, lang string, width int, themeStyles theme.Styles) []string {
	highlighted := highlightCode(code, lang)
	var lines []string
	for _, hl := range strings.Split(strings.TrimRight(highlighted, "\n"), "\n") {
		// Apply background across the full width
		padded := hl + strings.Repeat(" ", max(0, width-lipgloss.Width(hl)))
		styled := themeStyles.CodeBlock.Render(padded)
		lines = append(lines, styled)
	}
	return lines
}

// highlightCode uses chroma to syntax-highlight code for terminal output.
func highlightCode(code, lang string) string {
	// Find lexer by language hint, or auto-detect
	var lexer chroma.Lexer
	if lang != "" {
		lexer = lexers.Get(lang)
	}
	if lexer == nil {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)

	// Use a dark terminal-friendly style
	style := styles.Get("dracula")
	if style == nil {
		style = styles.Fallback
	}

	formatter := formatters.Get("terminal256")
	if formatter == nil {
		formatter = formatters.Fallback
	}

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, iterator); err != nil {
		return code
	}

	return buf.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// renderInlineCode parses text for `code` segments, diffs, and URLs.
func renderInlineCode(text string, width int, s theme.Styles, t theme.Theme) []string {
	var result []string

	for _, line := range strings.Split(text, "\n") {
		wrapped := wordWrap(line, width)
		for _, wl := range strings.Split(wrapped, "\n") {
			result = append(result, renderInlineCodeLine(wl, s, t))
		}
	}

	return result
}

// renderInlineCodeLine handles a single line, styling `code` spans, diffs, and URLs.
func renderInlineCodeLine(line string, s theme.Styles, t theme.Theme) string {
	// Check for diff lines first (they style the whole line)
	if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
		return lipgloss.NewStyle().Foreground(t.Success).Render(line)
	}
	if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
		return lipgloss.NewStyle().Foreground(t.Error).Render(line)
	}
	if strings.HasPrefix(line, "@@") {
		return lipgloss.NewStyle().Foreground(t.Info).Render(line)
	}
	if strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
		return lipgloss.NewStyle().Foreground(t.ForegroundDim).Bold(true).Render(line)
	}

	// Handle inline code + URLs
	var out strings.Builder
	remaining := line

	for {
		tick := strings.IndexByte(remaining, '`')
		if tick == -1 {
			out.WriteString(renderURLs(remaining, s, t))
			break
		}

		if tick > 0 {
			out.WriteString(renderURLs(remaining[:tick], s, t))
		}

		afterTick := remaining[tick+1:]
		closeTick := strings.IndexByte(afterTick, '`')
		if closeTick == -1 {
			out.WriteString(renderURLs(remaining[tick:], s, t))
			break
		}

		code := afterTick[:closeTick]
		out.WriteString(s.InlineCode.Render(" " + code + " "))
		remaining = afterTick[closeTick+1:]
	}

	return out.String()
}

// renderURLs detects URLs in a line and makes them styled (and clickable via OSC 8 in supporting terminals).
func renderURLs(line string, s theme.Styles, t theme.Theme) string {
	urlStyle := lipgloss.NewStyle().Foreground(t.Info).Underline(true)

	var result strings.Builder
	remaining := line
	for {
		// Simple URL detection
		idx := strings.Index(remaining, "http://")
		if i := strings.Index(remaining, "https://"); i >= 0 && (idx < 0 || i < idx) {
			idx = i
		}
		if idx < 0 {
			result.WriteString(s.MessageBody.Render(remaining))
			break
		}

		// Render text before URL
		if idx > 0 {
			result.WriteString(s.MessageBody.Render(remaining[:idx]))
		}

		// Find end of URL (space, newline, or end of string)
		urlPart := remaining[idx:]
		endIdx := strings.IndexAny(urlPart, " \t\n\r>)],;")
		if endIdx < 0 {
			endIdx = len(urlPart)
		}
		url := urlPart[:endIdx]

		// OSC 8 hyperlink: \033]8;;URL\033\\TEXT\033]8;;\033\\
		result.WriteString(fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\", url, urlStyle.Render(url)))

		remaining = urlPart[endIdx:]
	}
	return result.String()
}

func formatTimestamp(t time.Time) string {
	now := time.Now()
	if t.YearDay() == now.YearDay() && t.Year() == now.Year() {
		return t.Format("15:04")
	}
	if now.Sub(t) < 7*24*time.Hour {
		return t.Format("Mon 15:04")
	}
	return t.Format("Jan 2 15:04")
}

func wordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}

	var result strings.Builder
	for _, line := range strings.Split(text, "\n") {
		if len(line) <= width {
			if result.Len() > 0 {
				result.WriteByte('\n')
			}
			result.WriteString(line)
			continue
		}

		words := strings.Fields(line)
		currentLen := 0
		first := true
		for _, word := range words {
			if !first && currentLen+1+len(word) > width {
				result.WriteByte('\n')
				currentLen = 0
				first = true
			}
			if !first {
				result.WriteByte(' ')
				currentLen++
			}
			result.WriteString(word)
			currentLen += len(word)
			first = false
		}
	}
	return result.String()
}
