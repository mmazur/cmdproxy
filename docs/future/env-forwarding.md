# Environment Variable Forwarding

Allow the shim to forward selected environment variables to the remote command.

## Open Questions

- Allowlist of env vars per command in shim config?
- Security implications of forwarding secrets
- Encoding: add `env` field to the wire protocol Request
