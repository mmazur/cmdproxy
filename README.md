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
    "account show *",
    "acr list *",
]
deny = [
    "* --delete*",
    "* --force*",
]
```

#### Positional argument lists

An array where each element is matched against individual arguments, giving precise control over argument positions:

```toml
allow = [
    ["account", "show"],                    # exactly these two args
    ["acr", "show", "*"],                   # acr show <anything>
    ["[a-z]*:+", "--help"],                 # one or more subcommands, then --help
    ["[a-z]*:*", "list*", "*:*"],           # optional subcommands, a list* arg, then anything
]
deny = [
    ["[a-z]*:*", "delete", "*:*"],          # deny "delete" as a positional subcommand
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

Literal colons in glob patterns must be escaped as `\:`. Only one unescaped colon is allowed per element. Invalid quantifiers are rejected at config load time.

**Examples:**

```toml
[command.az]
allow = [
    # Legacy glob: account show with any trailing args
    "account show *",

    # Any subcommand path ending in --help
    ["[a-z]*:+", "--help"],

    # Any subcommand path, then a list* command, then any trailing args
    ["[a-z]*:*", "list*", "*:*"],

    # Exact: acr login with one argument
    ["acr", "login", "*"],

    # Exact: account get-access-token with specific flags
    ["account", "get-access-token", "-o", "json", "--resource", "*"],
]
deny = [
    # Glob: deny --delete anywhere in args
    "* --delete*",

    # Positional: deny "delete" as a subcommand
    ["[a-z]*:*", "delete", "*:*"],
]
```

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
