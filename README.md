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

### Command aliasing

The config key is the local command name (the shim's symlink name). By default that same name is sent to the server. Set `command` to send a different name — for example, symlink the shim as `kubectl-prod` locally but run `kubectl` on the server. This lets two symlinks share one server-side command while each keeps its own target and policy:

```toml
[command.kubectl-prod]
target = 'user@server'
command = 'kubectl'
```

Local lookups (target, ssh_args, run_local, local_bin, upload) still key off the symlink name (`kubectl-prod`); only the command executed and policy-matched server-side becomes `kubectl`.

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

String and positional-array rules match case-insensitively. Structured rules
match case-sensitively, including option names and paths.

Stdin is controlled by server policy. For compatibility, stdin is allowed by default. Set top-level `stdin = false` in a server profile to deny stdin by default, then opt individual commands back in with `stdin = true`:

```toml
stdin = false

[command.kubectl]
allow = [['apply', '-f', '-']]
stdin = true

[command.az]
allow = [['account', 'show']]
# stdin remains false
```

The shim always forwards stdin when invoking the server. The server-side `stdin` policy is the enforcement point: in socket mode denied stdin is discarded, and in SSH mode denied stdin is replaced with `/dev/null` before the command is executed.

File transfer is controlled separately from command stdin. The shim can be configured to package file or directory arguments into a tar.gz bundle before command stdin; the server extracts the bundle into a temporary working directory before running the command. Commands must opt in server-side with `allow_file_transfer = true` or any request carrying upload metadata is rejected.

```toml
# shim.toml
[command.kubectl]
target = 'user@server'

[[command.kubectl.upload]]
match = ['apply', '*:*']
file_args = ['-f', '--filename']
```

```toml
# profiles/default.toml
[command.kubectl]
allow = [['apply', '*:*']]
allow_file_transfer = true
```

Relative upload paths keep their original argv value and work because the command runs from the temporary workspace. Absolute upload paths are rewritten to a visible staged path such as `./.cmdproxy-upload/0/file.yaml`. If a configured file argument points to a missing local path, the shim sends upload metadata without file bytes; the server still runs from an empty temporary workspace so the command fails with its normal “file not found” behavior. Upload bundles are validated during extraction and cannot write outside the temporary workspace.

### Rule syntax

Both server `allow` and `deny` lists support three formats that can be mixed freely:

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

Compatibility note: quantified segments now check every consumed argument. A
previous bug could skip earlier values when the minimum count was at least two
(for example, `:2` or `:2+`). Such nonmatching arguments are now rejected in
server and shim rules alike.

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

#### Structured rules with named options

Use a structured rule when the command/subcommand order matters but its options
may appear in any order. For this example, `tool` is a fictional CLI:

```toml
[[command.tool.allow]]
prefix = ['group', '{create,remove}']
options = [
    ['--workspace', 'sandbox'],
    ['{--name,-n}', 'scratch-*'],
]
```

This allows both `tool group remove --workspace sandbox -n scratch-one` and
`tool group remove --name=scratch-one --workspace=sandbox`. Each compact option
list declares its name (or brace-separated literal aliases), followed by its
value patterns. Each option is required **exactly once**, counting all its
aliases together. `['--confirm']` declares a required switch with no value.

For optional, forbidden, or repeatable options, use explicit objects. Both forms
can appear in the same `options` list. This is equivalent to the compact rule
above, with an additional optional switch:

```toml
[[command.tool.allow]]
prefix = ['group', '{create,remove}']
options = [
    { names = ['--workspace'], values = ['sandbox'] },
    { names = ['--name', '-n'], values = ['scratch-*'], occurs = '1' },
    { names = ['--confirm'], values = [], occurs = '?' },
]
```

`occurs` counts option occurrences across aliases, independently of the number
of values each occurrence consumes. Its default is `'1'` in both forms. It
accepts the positional quantifiers without the colon: `'?'` (zero or one),
`'0'` (forbidden), `'N'`, `'N-M'`, `'N+'`, `'*'`, or `'+'`. Every occurrence's
values must match, even when repetition is explicitly allowed. Duplicate
selectors fail by default, including identical values and mixed spellings such
as `-n scratch-one --name=scratch-one`.

The complete matching rules are:

- `prefix` is a nonempty, ordered list of one-argument globs anchored at argv
  zero. Quantifiers and option arguments are not allowed in the prefix. Options
  before or between prefix arguments do not match. This format currently requires
  an initial non-option argument, typically a subcommand; use existing string or
  positional rules for flag-only invocations.
- After the prefix, declared options can appear in any order, interspersed with
  bare positional arguments. **Unknown options fail the rule by default.** Names
  are exact, case-sensitive spellings: long names such as `--workspace` or single-character
  short names such as `-n`. Wildcard names, abbreviations, joined short values
  (`-nNAME`), and short clusters (`-it`) are not supported. Write `-n NAME` and
  `-i -t` instead.
- One-value options accept `--name VALUE`, `--name=VALUE`, and `-n VALUE` for
  declared aliases. Short assignments such as `-n=VALUE` are rejected because
  some programs interpret their value as `=VALUE`. Long assignments split at
  the first `=` only. Empty and dash-leading option values are rejected,
  including assignment forms. Switches
  have `values = []` and cannot take assignments such as `--confirm=true`.
- `values` uses the positional segment grammar, including escaped literal
  colons. Use exact-one segments for fixed tuples, or a single repeated segment
  such as `'scratch-*:2'` or `'scratch-*:+'`. All values must match. Optional-value
  segments and mixtures of quantified segments are rejected at config load.
- Fixed-count options consume exactly that many following arguments.
  Variable-count options consume the entire consecutive run of non-option
  values up to the next option or `--`. They cannot be combined with
  `positionals`, and do not accept assignment syntax. Excess or disallowed
  values fail the rule; they are never reassigned to positionals.
- Remaining bare arguments match the ordered `positionals` list using the usual
  segment grammar. Omitted means no positionals in strict mode. In strict mode,
  even `positionals = ['*:*']` cannot absorb undeclared options. Declared option
  values cannot satisfy positionals or other required options. A standalone `-`
  is also rejected before the `--`
  separator, including as a positional or stdin marker.
- All prefix, value, positional, and tail patterns in this format are
  case-sensitive. Globs retain their usual semantics: `*` can span `/` and
  does not validate or canonicalize path components.

**Optional passthrough:** set `allow_extra_args = true` to constrain only the
declared options while permitting unrelated arguments:

```toml
[[command.tool.allow]]
prefix = ['group', 'remove']
allow_extra_args = true
options = [
    ['--workspace', 'sandbox'],
    ['{--name,-n}', 'scratch-*'],
]
```

This also permits `tool group remove --region test --workspace sandbox
-n scratch-one --confirm`. Both compact and explicit option declarations work
with this setting. The default is `false`; existing rules stay strict.

Declared options still enforce required occurrences, alias counts, and every
value constraint. A bad or repeated declared option cannot escape as an extra
argument. The prefix and `--` boundary are unchanged, and deny rules still take
precedence. Joined/clustered short options, short assignments, malformed option
names, and a standalone `-` before the separator remain rejected.

When `positionals` is omitted in this mode, leftover bare arguments pass through
too. If `positionals` is explicitly provided (including `[]`), it must match all
leftover bare arguments in order. This includes values of undeclared options,
because cmdproxy does not know which tokens those options consume. Variable-count
declared options still consume their complete value run; a bad value is never
reclassified as an extra argument.

**This mode is intentionally weaker.** Cmdproxy scans undeclared options as raw
tokens without inferring their value arities. For example, `tool group remove
--description --name scratch-one --workspace sandbox` matches this rule even if
the actual program consumes `--name` as the value of `--description`. Unknown
options may also introduce alternative selectors or overrides. Keep appropriate
deny guards for conflicting selectors; they do not remove this parsing
uncertainty. Declare options with explicit value arities when that distinction
matters. Text inside a single argv value is never split into additional options,
and arguments after `--` never satisfy outer option requirements.

For example, a batch command can allow one occurrence with several values:

```toml
[[command.tool.allow]]
prefix = ['batch', 'remove']
options = [['--items', 'scratch-*:+']]
```

`tool batch remove --items scratch-one scratch-two` matches. An unrelated item,
a repeated `--items`, or `--items=scratch-one` fails this rule. Use a separate
closed rule for a different selector form; combining the selectors then fails
both rules. The matcher assigns no built-in meaning to any option name.

**The `--` boundary:** by default a standalone `--` fails a structured rule. To
require it, add `after_double_dash`, an ordered pattern for the complete raw tail:

```toml
[[command.tool.allow]]
prefix = ['exec']
positionals = ['worker-*']
options = [
    ['--config', '/tmp/sandbox/*.conf'],
    { names = ['--pool', '-p'], values = ['test'], occurs = '?' },
]
after_double_dash = ['*:+']
```

This permits `tool exec worker-one --config /tmp/sandbox/client.conf -- echo hello`.
It rejects `tool exec worker-one -- echo --config /tmp/sandbox/client.conf`:
arguments after the first separator cannot satisfy outer options. The tail is
matched literally as argv; later separators and option-like tokens are ordinary
tail arguments. `after_double_dash = []` requires a separator with an empty
tail. Use a narrower tail such as `['echo', 'hello']` to restrict the executed
command, rather than allowing arbitrary tail arguments.

**Reusing long value patterns:** define flat, named lists at the profile's top
level and use `values_ref` instead of `values` in an explicit option:

```toml
[value_sets]
owned_names = ['{scratch-*,experiment-*}']

[[command.tool.allow]]
prefix = ['item', 'remove']
options = [
    { names = ['--name', '-n'], values_ref = 'owned_names' },
]
```

Value sets use the same value-segment syntax. They have no interpolation,
inheritance, or nested references. Referenced value sets must not be empty;
declare switches explicitly with `values = []`. Unknown references, overlapping
aliases, invalid patterns/counts, and unknown rule/option fields fail config loading.

Each `[[command.tool.allow]]` block adds an alternative rule. To mix formats,
put structured rules in an ordinary array as inline tables:

```toml
[command.tool]
allow = [
    'version *',
    ['status'],
    { prefix = ['item', 'show'], options = [['--name', 'scratch-*']] },
]
deny = [
    { prefix = ['item', 'show'], options = [['--name', 'scratch-protected']] },
]
```

Do not define both `allow = [...]` and `[[command.tool.allow]]` for the same
command; TOML treats that as redefining the same key. Structured rules are
server-only; shim `run_local` and upload rules keep their existing syntax.

Evaluation is unchanged: matching deny rules win, then any matching allow wins.
A structured rule failing on an unknown option or a duplicate is a **non-match**,
not a global veto. Another broad allow can still authorize that invocation, so
retain existing deny guards until overlapping allows have been audited. In strict
mode, replacing wildcard gaps with a closed option vocabulary requires declaring
each additional permitted option. Use `allow_extra_args = true` to retain a
permissive remainder with the weaker guarantees described above. Old binaries
reject structured rule entries.

By default, structured deny rules also match only their complete declared shape.
In the example above, adding an undeclared `--verbose`, a positional argument, or `--`
makes the structured deny a non-match. If a broad allow such as `'item *'` were
added, it would then authorize those arguments. Keep suitably scoped string or
positional deny globs for guards against broad allows; a structured deny is not
an unconditional ban on a selector wherever it appears.

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
