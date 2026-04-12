package protocol

import (
	"testing"
)

func TestRoundtrip(t *testing.T) {
	tests := []struct {
		name string
		req  Request
	}{
		{
			name: "simple command",
			req:  Request{Cmd: "az", Args: []string{"group", "list", "--output", "table"}},
		},
		{
			name: "no args",
			req:  Request{Cmd: "whoami", Args: nil},
		},
		{
			name: "empty args",
			req:  Request{Cmd: "echo", Args: []string{}},
		},
		{
			name: "args with spaces and special chars",
			req:  Request{Cmd: "jq", Args: []string{".data[] | select(.name == \"foo\")", "input.json"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := Encode(tt.req)
			if err != nil {
				t.Fatalf("Encode: %v", err)
			}

			decoded, err := Decode(encoded)
			if err != nil {
				t.Fatalf("Decode: %v", err)
			}

			if decoded.Cmd != tt.req.Cmd {
				t.Errorf("Cmd = %q, want %q", decoded.Cmd, tt.req.Cmd)
			}
			if len(decoded.Args) != len(tt.req.Args) {
				t.Fatalf("len(Args) = %d, want %d", len(decoded.Args), len(tt.req.Args))
			}
			for i := range decoded.Args {
				if decoded.Args[i] != tt.req.Args[i] {
					t.Errorf("Args[%d] = %q, want %q", i, decoded.Args[i], tt.req.Args[i])
				}
			}
		})
	}
}

func TestDecodeInvalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"not base64", "!!!not-base64!!!"},
		{"not gzip", "aGVsbG8="},                     // base64("hello")
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decode(tt.input)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}
