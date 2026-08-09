# RFC 015: Module Hygiene

Status: Implemented

Gallow reports read-only `go mod tidy -diff` drift and unresolved Go tool dependencies. It treats valid tool directives and conventional tool imports as intentional dependency usage.

Local `replace` directives are outside scope because release reproducibility is distinct from repository entropy.
