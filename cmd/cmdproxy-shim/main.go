package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mmazur/cmdproxy/internal/config"
	"github.com/mmazur/cmdproxy/internal/protocol"
)

func main() {
	cmdName := resolveCommandName()
	if cmdName == "cmdproxy-shim" {
		fmt.Fprintln(os.Stderr, "usage: symlink this binary as the command you want to proxy")
		fmt.Fprintln(os.Stderr, "  ln -s cmdproxy-shim kubectl")
		fmt.Fprintln(os.Stderr, "  ./kubectl get pods")
		os.Exit(127)
	}

	cfg, err := config.LoadShimConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cmdproxy-shim: %v\n", err)
		os.Exit(127)
	}

	target := cfg.TargetForCommand(cmdName)
	if target == "" {
		fmt.Fprintf(os.Stderr, "cmdproxy-shim: no target configured for %q\n", cmdName)
		os.Exit(127)
	}

	req := protocol.Request{
		Cmd:  cmdName,
		Args: os.Args[1:],
	}
	encoded, err := protocol.Encode(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cmdproxy-shim: encode: %v\n", err)
		os.Exit(127)
	}

	sshCmd := exec.Command("ssh", target, encoded)
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr
	if cfg.StdinEnabled(cmdName) {
		sshCmd.Stdin = os.Stdin
	}

	if err := sshCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "cmdproxy-shim: ssh: %v\n", err)
		os.Exit(127)
	}
}

func resolveCommandName() string {
	exe, err := os.Executable()
	if err != nil {
		return filepath.Base(os.Args[0])
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return filepath.Base(exe)
	}
	// If the resolved path differs from the original, the original is a symlink.
	// Use the original name (the symlink name), not the resolved target.
	if resolved != exe {
		return filepath.Base(exe)
	}
	return filepath.Base(os.Args[0])
}
