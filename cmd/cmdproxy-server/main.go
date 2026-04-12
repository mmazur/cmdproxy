package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mmazur/cmdproxy/internal/config"
	"github.com/mmazur/cmdproxy/internal/policy"
	"github.com/mmazur/cmdproxy/internal/protocol"
)

type logEntry struct {
	Timestamp  string   `json:"timestamp"`
	Command    string   `json:"command"`
	Args       []string `json:"args"`
	Decision   string   `json:"decision"`
	Reason     string   `json:"reason"`
	DurationMs int64    `json:"duration_ms"`
}

func main() {
	start := time.Now()

	origCmd := os.Getenv("SSH_ORIGINAL_COMMAND")
	if origCmd == "" {
		fmt.Fprintln(os.Stderr, "cmdproxy-server: SSH_ORIGINAL_COMMAND not set")
		os.Exit(126)
	}

	req, err := protocol.Decode(origCmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cmdproxy-server: decode: %v\n", err)
		os.Exit(126)
	}

	cfg, err := config.LoadServerConfig("default")
	if err != nil {
		fmt.Fprintf(os.Stderr, "cmdproxy-server: config: %v\n", err)
		os.Exit(126)
	}

	decision := policy.Evaluate(cfg, req.Cmd, req.Args)

	writeLog(logEntry{
		Timestamp:  start.UTC().Format(time.RFC3339),
		Command:    req.Cmd,
		Args:       req.Args,
		Decision:   decision.Verdict.String(),
		Reason:     decision.Reason,
		DurationMs: time.Since(start).Milliseconds(),
	})

	if decision.Verdict == policy.Deny {
		fmt.Fprintf(os.Stderr, "cmdproxy: denied: %s\n", decision.Reason)
		os.Exit(126)
	}

	binPath, err := exec.LookPath(req.Cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cmdproxy-server: command not found: %s\n", req.Cmd)
		os.Exit(127)
	}

	argv := append([]string{req.Cmd}, req.Args...)
	if err := syscall.Exec(binPath, argv, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "cmdproxy-server: exec: %v\n", err)
		os.Exit(126)
	}
}

func writeLog(entry logEntry) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	logDir := filepath.Join(home, ".local", "log", "cmdproxy")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return
	}

	logPath := filepath.Join(logDir, "access.jsonl")
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()

	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f.Write(line)
	f.Write([]byte("\n"))
}
