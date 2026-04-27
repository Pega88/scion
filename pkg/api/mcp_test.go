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

package api

import (
	"strings"
	"testing"
)

func TestValidateMCPServers(t *testing.T) {
	tests := []struct {
		name    string
		servers map[string]MCPServerConfig
		wantErr string
	}{
		{
			name:    "empty map",
			servers: nil,
			wantErr: "",
		},
		{
			name: "valid stdio",
			servers: map[string]MCPServerConfig{
				"chrome-devtools": {
					Transport: MCPTransportStdio,
					Command:   "chrome-devtools-mcp",
					Args:      []string{"--headless"},
					Env:       map[string]string{"DEBUG": "false"},
				},
			},
		},
		{
			name: "valid sse",
			servers: map[string]MCPServerConfig{
				"remote_api": {
					Transport: MCPTransportSSE,
					URL:       "http://localhost:8080/mcp/sse",
					Headers:   map[string]string{"Authorization": "Bearer xyz"},
				},
			},
		},
		{
			name: "valid streamable-http",
			servers: map[string]MCPServerConfig{
				"streaming-svc": {
					Transport: MCPTransportStreamableHTTP,
					URL:       "http://localhost:9090/mcp",
					Scope:     MCPScopeProject,
				},
			},
		},
		{
			name: "stdio missing command",
			servers: map[string]MCPServerConfig{
				"foo": {Transport: MCPTransportStdio},
			},
			wantErr: "requires command",
		},
		{
			name: "stdio with url",
			servers: map[string]MCPServerConfig{
				"foo": {Transport: MCPTransportStdio, Command: "x", URL: "http://x"},
			},
			wantErr: "does not allow url",
		},
		{
			name: "stdio with headers",
			servers: map[string]MCPServerConfig{
				"foo": {Transport: MCPTransportStdio, Command: "x", Headers: map[string]string{"a": "b"}},
			},
			wantErr: "does not allow headers",
		},
		{
			name: "sse missing url",
			servers: map[string]MCPServerConfig{
				"foo": {Transport: MCPTransportSSE},
			},
			wantErr: "requires url",
		},
		{
			name: "sse with command",
			servers: map[string]MCPServerConfig{
				"foo": {Transport: MCPTransportSSE, URL: "http://x", Command: "y"},
			},
			wantErr: "does not allow command",
		},
		{
			name: "sse with args",
			servers: map[string]MCPServerConfig{
				"foo": {Transport: MCPTransportSSE, URL: "http://x", Args: []string{"a"}},
			},
			wantErr: "does not allow args",
		},
		{
			name: "sse with env",
			servers: map[string]MCPServerConfig{
				"foo": {Transport: MCPTransportSSE, URL: "http://x", Env: map[string]string{"A": "B"}},
			},
			wantErr: "does not allow env",
		},
		{
			name: "missing transport",
			servers: map[string]MCPServerConfig{
				"foo": {Command: "x"},
			},
			wantErr: "missing required field: transport",
		},
		{
			name: "invalid transport",
			servers: map[string]MCPServerConfig{
				"foo": {Transport: MCPTransport("websocket"), Command: "x"},
			},
			wantErr: "invalid transport",
		},
		{
			name: "invalid scope",
			servers: map[string]MCPServerConfig{
				"foo": {Transport: MCPTransportStdio, Command: "x", Scope: MCPScope("private")},
			},
			wantErr: "invalid scope",
		},
		{
			name: "invalid name (leading hyphen)",
			servers: map[string]MCPServerConfig{
				"-bad": {Transport: MCPTransportStdio, Command: "x"},
			},
			wantErr: "invalid name",
		},
		{
			name: "invalid name (special chars)",
			servers: map[string]MCPServerConfig{
				"bad name": {Transport: MCPTransportStdio, Command: "x"},
			},
			wantErr: "invalid name",
		},
		{
			name: "valid name with underscore",
			servers: map[string]MCPServerConfig{
				"chrome_devtools": {Transport: MCPTransportStdio, Command: "x"},
			},
		},
		{
			name: "scope global explicit",
			servers: map[string]MCPServerConfig{
				"x": {Transport: MCPTransportStdio, Command: "y", Scope: MCPScopeGlobal},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMCPServers(tt.servers)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestIsValidMCPName(t *testing.T) {
	tests := map[string]bool{
		"":                     false,
		"chrome-devtools":      true,
		"filesystem":           true,
		"chrome_devtools":      true,
		"my_server_v2":         true,
		"My-Server":            true,
		"server123":            true,
		"-leading-hyphen":      false,
		"trailing-hyphen-":     false,
		"_leading-underscore":  false,
		"trailing-underscore_": false,
		"with space":           false,
		"with.dot":             false,
		"with/slash":           false,
	}
	for name, want := range tests {
		if got := isValidMCPName(name); got != want {
			t.Errorf("isValidMCPName(%q) = %v, want %v", name, got, want)
		}
	}
}
