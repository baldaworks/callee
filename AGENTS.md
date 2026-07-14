# Callee agent guidance

- Preserve the two-tool MCP surface: `callee` starts conversations and `callee-reply` continues them.
- Preserve flat role frontmatter. Do not add nested provider configuration.
- Do not add Gemini support.
- Do not add MCP forwarding without an explicit product decision.
- Keep stdout clean in MCP mode; diagnostics belong on stderr.
- Use Norma Runtime for ACP process logic rather than duplicating it.
- Run `go test ./...` and `go test -race ./...` before completion.
