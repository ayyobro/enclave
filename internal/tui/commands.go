package tui

import "strings"

// SlashCommand defines a slash command.
type SlashCommand struct {
	Name        string
	Description string
	HasArgs     bool
}

// Commands is the registry of all available slash commands.
var Commands = []SlashCommand{
	{Name: "/help", Description: "List all available commands"},
	{Name: "/clear", Description: "Clear the chat view"},
	{Name: "/whoami", Description: "Show your identity and fingerprint"},
	{Name: "/verify", Description: "Verify contact keys (or /verify <name>)", HasArgs: true},
	{Name: "/users", Description: "List all users and online status"},
	{Name: "/copy", Description: "Copy code block to clipboard", HasArgs: true},
	{Name: "/search", Description: "Search message history", HasArgs: true},
	{Name: "/send", Description: "Send an encrypted file", HasArgs: true},
	{Name: "/react", Description: "React to last message with emoji", HasArgs: true},
	{Name: "/group", Description: "Create a group: /group <name> <user1> <user2>", HasArgs: true},
	{Name: "/invite", Description: "Invite user to current group", HasArgs: true},
	{Name: "/pin", Description: "Pin the last message or code block"},
	{Name: "/pins", Description: "List all pinned messages"},
	{Name: "/ephemeral", Description: "Toggle disappearing messages", HasArgs: true},
	{Name: "/vibe2gether", Description: "Start collaborative Claude Code session", HasArgs: true},
	{Name: "/endvibe", Description: "End the collaborative coding session"},
	{Name: "/export", Description: "Save conversation to a file"},
	{Name: "/quit", Description: "Exit Enclave"},
}

// FilterCommands returns commands matching a prefix.
func FilterCommands(prefix string) []SlashCommand {
	if prefix == "/" {
		return Commands
	}

	prefix = strings.ToLower(prefix)
	var matches []SlashCommand
	for _, cmd := range Commands {
		if strings.HasPrefix(cmd.Name, prefix) {
			matches = append(matches, cmd)
		}
	}
	return matches
}
