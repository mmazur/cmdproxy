package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTarget(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name    string
		input   string
		mode    TargetMode
		addr    string
		wantErr string
	}{
		{
			name:  "ssh target",
			input: "user@host.example.com",
			mode:  TargetSSH,
			addr:  "user@host.example.com",
		},
		{
			name:  "socket absolute",
			input: "socket:/run/user/1000/cmdproxy.sock",
			mode:  TargetSocket,
			addr:  "/run/user/1000/cmdproxy.sock",
		},
		{
			name:  "socket with tilde",
			input: "socket:~/.local/state/cmdproxy/cmdproxy.sock",
			mode:  TargetSocket,
			addr:  filepath.Join(home, ".local/state/cmdproxy/cmdproxy.sock"),
		},
		{
			name:  "socket with $HOME",
			input: "socket:$HOME/.local/state/cmdproxy/cmdproxy.sock",
			mode:  TargetSocket,
			addr:  filepath.Join(home, ".local/state/cmdproxy/cmdproxy.sock"),
		},
		{
			name:    "socket relative path",
			input:   "socket:relative/path.sock",
			wantErr: "socket path must be absolute (start with / or ~)",
		},
		{
			name:    "socket empty path",
			input:   "socket:",
			wantErr: "socket path is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := ParseTarget(tt.input)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if got := err.Error(); got != tt.wantErr && !contains(got, tt.wantErr) {
					t.Fatalf("error = %q, want containing %q", got, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if target.Mode != tt.mode {
				t.Errorf("mode = %d, want %d", target.Mode, tt.mode)
			}
			if target.Addr != tt.addr {
				t.Errorf("addr = %q, want %q", target.Addr, tt.addr)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
