# Interactive Approval

When a command doesn't match any allow pattern but isn't explicitly denied,
prompt the user (via a side channel) to approve or reject the command in real time.

## Open Questions

- Approval channel: terminal prompt, Slack, webhook?
- Timeout behavior: deny after N seconds of no response?
- Audit trail: log who approved and when
