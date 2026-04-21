# cmdproxy

A tool for giving AI agents scoped, read-only access to CLI tools — either hosted on a remote system over SSH or on your own desktop via a reverse-forwarded-over-ssh Unix socket.

## Use cases

- access to a select subset kubectl / az / aws / etc commands (LLM does not have the ability to just read the raw access token and issue its own commands)
- ssh into a server, run Claude Code (or equivalent), have ctrl+v actually paste your local images (Linux-only; for now)

## Components

- **cmdproxy-shim** — a client binary you symlink as local commands (`kubectl`, `az`, `wl-paste`, etc.). When invoked, it forwards the command and arguments to a server over SSH or a Unix socket.
- **cmdproxy-server** — a server that evaluates the incoming request against a allow/deny policy, logs the decision, and either executes the command or rejects it.

## Transport modes

### SSH

The primary use case: give an AI agent access to a subset of CLI tools' functionality without giving it access to the underlying auth tokens.

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

The server runs as an SSH forced command. Configure the shim target as an SSH destination:

```toml
[command.kubectl]
target = 'user@server'
```

### Unix socket

Forward commands back to your local machine over an existing SSH connection, without opening a reverse SSH server. Useful for clipboard access, local tool execution, or any case where the tool must run on the desktop side.

```
remote server                         local desktop
─────────────                         ─────────────
./wl-paste
  → cmdproxy-shim
    → connect to forwarded Unix socket
                                      → cmdproxy-server --socket
                                        → policy check
                                          → allowed → exec wl-paste
                                          → denied  → error in response
  ← JSON response (stdout, stderr, exit code)
```

Configure the shim target with the `socket:` prefix:

```toml
[command.wl-paste]
target = 'socket:~/.cmdproxy.sock'
```

Socket paths must be absolute. `~` and `$HOME` are expanded. Relative paths are rejected.

The server runs in socket mode with `--socket`. It reads a JSON request from stdin, evaluates policy, executes the command, and writes a JSON response to stdout. This is designed for systemd socket activation with `Accept=yes`, where each connection spawns a fresh server process.

See [`examples/wl-paste/`](examples/wl-paste/) for a complete worked example with systemd units, SSH config, and shim/server configuration.

## Policy

The server config defines per-command allow/deny patterns matched against arguments. Evaluation order:

1. Command not in config → **deny**
2. Args match any `deny` glob → **deny**
3. Args match any `allow` rule → **allow**
4. No match → **deny**

All matching is case-insensitive.

### Rule syntax

Both `allow` and `deny` lists support two formats that can be mixed freely:

#### String globs

A glob pattern matched against all arguments joined with spaces:

```toml
allow = [
    'account show *',
    'acr list *',
]
deny = [
    '* --delete*',
    '* --force*',
]
```

#### Positional argument lists

An array where each element is matched against individual arguments, giving precise control over argument positions:

```toml
allow = [
    ['account', 'show'],                    # exactly these two args
    ['acr', 'show', '*'],                   # acr show <anything>
    ['[a-z]*:+', '--help'],                 # one or more subcommands, then --help
    ['[a-z]*:*', 'list*', '*:*'],           # optional subcommands, a list* arg, then anything
]
deny = [
    ['[a-z]*:*', 'delete', '*:*'],          # deny "delete" as a positional subcommand
]
```

Each element has the form `glob_pattern[:quantifier]`.

**Glob patterns** use standard glob syntax (`*`, `?`, `[a-z]`, `{a,b}`). A bare string with no wildcards is an exact match.

**Quantifiers** control how many arguments a single element can consume:

| Quantifier | Meaning |
|---|---|
| *(none)* | exactly 1 (default) |
| `:*` | zero or more |
| `:+` | one or more |
| `:?` | zero or one |
| `:N` | exactly N (e.g. `:3`) |
| `:N+` | N or more (e.g. `:2+`) |
| `:N-M` | between N and M inclusive (e.g. `:2-5`) |

Literal colons in glob patterns must be escaped as `\:`. This works directly in single-quoted TOML strings (`'https\://*.example.com'`). In double-quoted strings, backslashes are TOML escape characters, so you need `"https\\://*.example.com"` instead. Prefer single-quoted strings to avoid this issue. Only one unescaped colon is allowed per element. Invalid quantifiers are rejected at config load time.

**Examples:**

```toml
[command.az]
allow = [
    # Legacy glob: account show with any trailing args
    'account show *',

    # Any subcommand path ending in --help
    ['[a-z]*:+', '--help'],

    # Any subcommand path, then a list* command, then any trailing args
    ['[a-z]*:*', 'list*', '*:*'],

    # Exact: acr login with one argument
    ['acr', 'login', '*'],

    # Exact: account get-access-token with specific flags
    ['account', 'get-access-token', '-o', 'json', '--resource', '*'],
]
deny = [
    # Glob: deny --delete anywhere in args
    '* --delete*',

    # Positional: deny "delete" as a subcommand
    ['[a-z]*:*', 'delete', '*:*'],
]
```

## Building

```
make              # build both binaries
make install      # install to ~/.local/bin
make test         # run tests
```

## Configuration

See `examples/` for annotated config files and `examples/wl-paste/` for a complete socket mode setup.

- Client: `~/.config/cmdproxy/shim.toml`
- Server: `~/.config/cmdproxy/profiles/default.toml`
