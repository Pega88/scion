package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/GoogleCloudPlatform/scion/pkg/config"
	"github.com/GoogleCloudPlatform/scion/pkg/util"
)

type CLIMode string

const (
	ModeHuman     CLIMode = "human"
	ModeAssistant CLIMode = "assistant"
	ModeAgent     CLIMode = "agent"
)

// assistantDenied lists commands removed in assistant mode (relative to human).
// Uses dot-separated command paths: "hub.auth", "config.migrate", etc.
var assistantDenied = map[string]bool{
	"hub.auth":         true,
	"hub.token":        true,
	"grove.reconnect":  true,
	"config.migrate":   true,
	"config.cd-config": true,
	"config.cd-grove":  true,
	"cdw":              true,
	"clean":            true,
}

// agentAllowed lists commands available in agent mode.
// Parent commands are implicitly allowed when any child is allowed.
var agentAllowed = map[string]bool{
	"create":                      true,
	"delete":                      true,
	"doctor":                      true,
	"list":                        true,
	"logs":                        true,
	"look":                        true,
	"message":                     true,
	"messages":                    true,
	"messages.read":               true,
	"resume":                      true,
	"start":                       true,
	"stop":                        true,
	"version":                     true,
	"whoami":                      true,
	"config":                      true,
	"config.list":                 true,
	"config.get":                  true,
	"config.dir":                  true,
	"config.schema":               true,
	"hub":                         true,
	"hub.status":                  true,
	"hub.notifications":           true,
	"notifications":               true,
	"notifications.ack":           true,
	"notifications.subscribe":     true,
	"notifications.unsubscribe":   true,
	"notifications.update":        true,
	"notifications.subscriptions": true,
	"schedule":                    true,
	"schedule.list":               true,
	"schedule.get":                true,
	"schedule.cancel":             true,
	"schedule.history":            true,
	"shared-dir":                  true,
	"shared-dir.list":             true,
	"shared-dir.info":             true,
}

// resolveMode determines the active CLI mode from environment and settings.
// Priority: SCION_CLI_MODE env var > cli.mode setting > default (human).
func resolveMode() CLIMode {
	if envMode := os.Getenv("SCION_CLI_MODE"); envMode != "" {
		switch CLIMode(envMode) {
		case ModeHuman, ModeAssistant, ModeAgent:
			return CLIMode(envMode)
		default:
			fmt.Fprintf(os.Stderr, "Warning: unrecognized SCION_CLI_MODE=%q, defaulting to %q\n", envMode, ModeHuman)
			return ModeHuman
		}
	}

	settings, err := config.LoadSettings("")
	if err == nil && settings != nil && settings.CLI != nil && settings.CLI.Mode != "" {
		switch CLIMode(settings.CLI.Mode) {
		case ModeHuman, ModeAssistant, ModeAgent:
			return CLIMode(settings.CLI.Mode)
		default:
			fmt.Fprintf(os.Stderr, "Warning: unrecognized cli.mode=%q in settings, defaulting to %q\n", settings.CLI.Mode, ModeHuman)
			return ModeHuman
		}
	}

	return ModeHuman
}

// applyModeRestrictions removes commands from the Cobra tree that are not
// permitted in the current CLI mode.
func applyModeRestrictions(root *cobra.Command) {
	mode := resolveMode()
	if mode == ModeHuman {
		return
	}

	removed := 0
	switch mode {
	case ModeAssistant:
		removed = applyAssistantMode(root)
	case ModeAgent:
		removed = applyAgentMode(root)
	}

	util.Debugf("CLI mode %q: removed %d commands from command tree", mode, removed)
}

// applyAssistantMode removes denied commands from the tree.
func applyAssistantMode(root *cobra.Command) int {
	return removeCommands(root, "", func(path string) bool {
		return assistantDenied[path]
	})
}

// applyAgentMode keeps only commands in the agent allow-list.
func applyAgentMode(root *cobra.Command) int {
	return removeCommands(root, "", func(path string) bool {
		return !agentAllowed[path]
	})
}

// removeCommands walks the command tree and removes commands where shouldRemove
// returns true. It processes children recursively before deciding whether to
// remove a parent. Returns the count of removed commands.
func removeCommands(parent *cobra.Command, prefix string, shouldRemove func(string) bool) int {
	removed := 0
	for _, child := range parent.Commands() {
		name := child.Name()
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}

		// Always keep built-in Cobra commands.
		if name == "help" || name == "completion" {
			continue
		}

		// Recurse into subcommands first.
		if child.HasSubCommands() {
			removed += removeCommands(child, path, shouldRemove)
		}

		if shouldRemove(path) {
			parent.RemoveCommand(child)
			removed++
		}
	}
	return removed
}
