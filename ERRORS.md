# Errors

This file opts the repository into the bento project-memory error log. It is
for durable agent notes about recurring tool, workflow, or environment failures
that future sessions should know about. It is not the user-facing `refute`
runtime error reference; see [docs/json-schema.md](docs/json-schema.md) for
the CLI JSON and exit-code contract.

Append entries under dated headings:

```markdown
## YYYY-MM-DD

- Context: what the agent was trying to do.
- Failure: exact command, tool, hook, or workflow that failed.
- Resolution: what fixed it, or the current human handoff if unresolved.
```

Keep entries concise, factual, and useful to future maintainers. Do not record
secrets, private credentials, or large command logs.
