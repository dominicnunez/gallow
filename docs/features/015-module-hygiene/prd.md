# PRD 015: Module Hygiene

Status: Implemented

## Requirements

- Detect tidy drift without changing `go.mod` or `go.sum`.
- Treat valid tool dependencies as used.
- Report unresolved tool directives.
- Emit human, JSON, and SARIF findings with removal-first remediation.
- Do not perform release-hygiene checks for local replacements.
