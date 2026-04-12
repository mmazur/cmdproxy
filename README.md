# cmdproxy

A security boundary for giving AI agents scoped, read-only access to CLI tools on a remote system.

## How it works

cmdproxy has two components:

- **cmdproxy-shim** — a client binary you symlink as local commands (`kubectl`, `az`, `jq`, etc.). When invoked, it forwards the command and arguments over SSH to a remote server.
- **cmdproxy-server** — a server binary that runs as an SSH forced command. It evaluates the incoming request against a glob-based allow/deny policy, logs the decision, and either executes the command or rejects it.

```
local machine                          remote server
─────────────                          ─────────────
./kubectl get pods
  → cmdproxy-shim
    → ssh user@server
                                       → cmdproxy-server
                                         → policy check
                                           → allowed → exec kubectl get pods
                                           → denied  → exit 126
  ← stdout/stderr + exit code
```

## Policy

The server config defines per-command allow/deny glob patterns matched against the joined argument string. Evaluation order:

1. Command not in config → **deny**
2. Args match any `deny` glob → **deny**
3. Args match any `allow` glob → **allow**
4. No match → **deny**

## Building

```
make              # build both binaries
make install      # install to ~/.local/bin
make test         # run tests
```

## Configuration

See `examples/` for annotated config files.

- Client: `~/.config/cmdproxy/shim.toml`
- Server: `~/.config/cmdproxy/profiles/default.toml`
