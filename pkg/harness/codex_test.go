// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package harness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ptone/scion-agent/pkg/api"
)

func TestCodexAuthPropagation(t *testing.T) {
	// Setup a temporary home directory
	tmpHome, err := os.MkdirTemp("", "scion-home-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpHome)

	// Mock HOME environment variable
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", origHome)

	// Create ~/.codex/auth.json
	codexDir := filepath.Join(tmpHome, ".codex")
	if err := os.MkdirAll(codexDir, 0755); err != nil {
		t.Fatal(err)
	}
	authPath := filepath.Join(codexDir, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"token":"secret"}`), 0644); err != nil {
		t.Fatal(err)
	}

	c := &Codex{}
	agentHome := filepath.Join(tmpHome, "agent-home")
	
	// Discover Auth
	auth := c.DiscoverAuth(agentHome)
	if auth.CodexAuthFile != authPath {
		t.Errorf("expected CodexAuthFile to be %s, got %s", authPath, auth.CodexAuthFile)
	}

	// Propagate Files
	if err := c.PropagateFiles(agentHome, "user", auth); err != nil {
		t.Fatalf("PropagateFiles failed: %v", err)
	}

	// Verify file was copied
	dstPath := filepath.Join(agentHome, ".codex", "auth.json")
	if _, err := os.Stat(dstPath); err != nil {
		t.Errorf("expected auth file to be copied to %s", dstPath)
	}
	data, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"token":"secret"}` {
		t.Errorf("unexpected content in copied auth file")
	}
}

func TestCodexGetEnv(t *testing.T) {
	c := &Codex{}

	// Test OPENAI_API_KEY passthrough
	os.Setenv("OPENAI_API_KEY", "test-key")
	defer os.Unsetenv("OPENAI_API_KEY")

	env := c.GetEnv("test-agent", "/tmp", "user", api.AuthConfig{})
	if env["OPENAI_API_KEY"] != "test-key" {
		t.Errorf("expected OPENAI_API_KEY to be 'test-key', got '%s'", env["OPENAI_API_KEY"])
	}
}

func TestCodexGetCommand(t *testing.T) {
	c := &Codex{}

	// Test standard command
	cmd := c.GetCommand("do something", false, []string{})
	if len(cmd) < 3 || cmd[0] != "codex" || cmd[1] != "--yolo" || cmd[2] != "do something" {
		t.Errorf("unexpected command structure: %v", cmd)
	}

	// Test resume
	cmd = c.GetCommand("", true, []string{})
	if len(cmd) < 3 || cmd[1] != "--yolo" || cmd[2] != "resume" {
		t.Errorf("unexpected resume command: %v", cmd)
	}
}

func TestCodexInjectAgentInstructions(t *testing.T) {
	agentHome := t.TempDir()
	c := &Codex{}
	content := []byte("# Agent Instructions\nDo good work.")

	if err := c.InjectAgentInstructions(agentHome, content); err != nil {
		t.Fatalf("InjectAgentInstructions failed: %v", err)
	}

	target := filepath.Join(agentHome, ".codex", "AGENTS.md")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected file at %s: %v", target, err)
	}
	if string(data) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", string(data), string(content))
	}
}

func TestCodexRequiredEnvKeys(t *testing.T) {
	c := &Codex{}

	got := c.RequiredEnvKeys("")
	if got != nil {
		t.Errorf("RequiredEnvKeys() = %v, want nil", got)
	}
}

func TestCodexInjectSystemPrompt_NoOp(t *testing.T) {
	agentHome := t.TempDir()
	c := &Codex{}

	// First inject agent instructions
	agentContent := []byte("# Existing Instructions\nDo things.")
	if err := c.InjectAgentInstructions(agentHome, agentContent); err != nil {
		t.Fatalf("InjectAgentInstructions failed: %v", err)
	}

	// System prompt injection should be a no-op (not yet supported)
	sysContent := []byte("You are a helpful assistant.")
	if err := c.InjectSystemPrompt(agentHome, sysContent); err != nil {
		t.Fatalf("InjectSystemPrompt failed: %v", err)
	}

	// AGENTS.md should remain unchanged — no system prompt prepended
	target := filepath.Join(agentHome, ".codex", "AGENTS.md")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected file at %s: %v", target, err)
	}

	if string(data) != string(agentContent) {
		t.Errorf("AGENTS.md was modified by InjectSystemPrompt; got %q, want %q", string(data), string(agentContent))
	}
}
