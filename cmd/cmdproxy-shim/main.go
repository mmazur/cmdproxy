package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/mmazur/cmdproxy/internal/config"
	"github.com/mmazur/cmdproxy/internal/filetransfer"
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

	if cfg.RunLocalMatch(cmdName, os.Args[1:]) {
		runLocal(cfg, cmdName, os.Args[1:])
	}

	rawTarget := cfg.TargetForCommand(cmdName)
	if rawTarget == "" {
		fmt.Fprintf(os.Stderr, "cmdproxy-shim: no target configured for %q\n", cmdName)
		os.Exit(127)
	}

	target, err := config.ParseTarget(rawTarget)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cmdproxy-shim: %v\n", err)
		os.Exit(127)
	}

	bundle, err := filetransfer.BuildRequest(cmdName, os.Args[1:], cfg.UploadRulesForCommand(cmdName))
	if err != nil {
		fmt.Fprintf(os.Stderr, "cmdproxy-shim: upload: %v\n", err)
		os.Exit(127)
	}
	req := bundle.Request

	switch target.Mode {
	case config.TargetSocket:
		runSocket(target.Addr, req, bundle.Data)
	case config.TargetSSH:
		runSSH(cfg, cmdName, target.Addr, req, bundle.Data)
	}
}

func runSocket(socketPath string, req protocol.Request, bundle []byte) {
	addr, err := net.ResolveUnixAddr("unix", socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cmdproxy-shim: resolve %s: %v\n", socketPath, err)
		os.Exit(127)
	}
	conn, err := net.DialUnix("unix", nil, addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cmdproxy-shim: connect %s: %v\n", socketPath, err)
		os.Exit(127)
	}
	defer conn.Close()

	data, err := json.Marshal(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cmdproxy-shim: encode request: %v\n", err)
		os.Exit(127)
	}
	if _, err := conn.Write(data); err != nil {
		fmt.Fprintf(os.Stderr, "cmdproxy-shim: write request: %v\n", err)
		os.Exit(127)
	}

	go func() {
		if len(bundle) > 0 {
			conn.Write(bundle)
		}
		io.Copy(conn, os.Stdin)
		conn.CloseWrite()
	}()

	var resp protocol.Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		fmt.Fprintf(os.Stderr, "cmdproxy-shim: read response: %v\n", err)
		os.Exit(127)
	}

	os.Stdout.Write(resp.Stdout)
	os.Stderr.Write(resp.Stderr)
	if resp.Error != "" {
		fmt.Fprintf(os.Stderr, "cmdproxy-shim: server: %s\n", resp.Error)
	}
	os.Exit(resp.ExitCode)
}

func runSSH(cfg config.ShimConfig, cmdName, target string, req protocol.Request, bundle []byte) {
	encoded, err := protocol.Encode(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cmdproxy-shim: encode: %v\n", err)
		os.Exit(127)
	}

	args := append(cfg.SSHArgsForCommand(cmdName), target, encoded)
	sshCmd := exec.Command("ssh", args...)
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr
	if len(bundle) == 0 {
		sshCmd.Stdin = os.Stdin
	} else {
		stdinPipe, err := sshCmd.StdinPipe()
		if err != nil {
			fmt.Fprintf(os.Stderr, "cmdproxy-shim: ssh stdin: %v\n", err)
			os.Exit(127)
		}
		go func() {
			stdinPipe.Write(bundle)
			io.Copy(stdinPipe, os.Stdin)
			stdinPipe.Close()
		}()
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

func runLocal(cfg config.ShimConfig, cmdName string, args []string) {
	bin, err := cfg.LocalBinPath(cmdName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cmdproxy-shim: %v\n", err)
		os.Exit(127)
	}
	argv := append([]string{cmdName}, args...)
	if err := syscall.Exec(bin, argv, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "cmdproxy-shim: exec %s: %v\n", bin, err)
		os.Exit(127)
	}
}
