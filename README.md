# Gallow

[![CI](https://github.com/dominicnunez/gallow/actions/workflows/ci.yml/badge.svg)](https://github.com/dominicnunez/gallow/actions/workflows/ci.yml)
![Go version](https://img.shields.io/github/go-mod/go-version/dominicnunez/gallow)
![License](https://img.shields.io/github/license/dominicnunez/gallow)

Gallow is a conservative **repository entropy analyzer for Go**.

It answers four questions:

1. What code is no longer needed?
2. What dependencies are no longer needed or incorrectly represented?
3. What implementation has been duplicated?
4. What new entropy did this branch introduce?

Gallow complements `go vet`, Staticcheck, `go mod tidy`, security scanners, and architecture tools. It is not a general-purpose linter.

## Install

Source builds require Go 1.26 or newer. The analysis dependency now reads
compiler export data from Go 1.27; CI exercises Go 1.26 and the current stable
Go 1.27.3. Earlier compiler support was dropped because the compatible
`golang.org/x/tools` release requires Go 1.26.

```bash
go install github.com/dominicnunez/gallow/cmd/gallow@latest
```

## Primary workflow

Audit a branch against its base:

```bash
gallow audit --base origin/main
```

This keeps existing repository debt from blocking adoption and focuses review on entropy introduced or worsened by the current change.

Run a full repository check:

```bash
gallow --root .
```

Use Gallow in CI:

```bash
gallow audit --base origin/main --format sarif --ci
```

## Analysis families

### Code entropy

Gallow reports unused packages, files, functions, methods, structs, interfaces, types, variables, constants, and fields.

### Dependency entropy

Gallow reports:

- unused direct requirements;
- external imports without matching direct requirements;
- `go mod tidy -diff` drift;
- unresolved Go `tool` directives.

Dependencies used only by tests are treated as used when tests are included. Local `replace` directives are outside Gallow's repository-entropy scope.

### Duplication entropy

Gallow reports substantial repeated Go code with all related locations. By default it requires a larger repeated block (12 meaningful lines or 100 normalized tokens across at least 12 token-bearing lines) and at least three substantive statements. Repeated single-return guard clauses—common Go error handling boilerplate—are not clone evidence.

Projects can tune the duplicate thresholds in `.gallowrc.json` when they deliberately want tighter reporting. The thresholds never accept values below the conservative safety floor: six lines, 50 tokens, eight token-lines, and one substantive statement.

Its remediation order is:

1. remove an obsolete or redundant copy;
2. extract shared logic when both copies are required and should evolve together;
3. narrowly suppress intentional local duplication with a `gallow-ignore` comment that explains why the duplication is intentional.

## Removal-first remediation

Every supported finding includes structured remediation metadata in JSON and SARIF. When removal is a valid remedy, Gallow recommends it first.

Example:

```json
{
  "remediation": {
    "preferred": "remove",
    "summary": "Remove the unused direct requirement."
  }
}
```

Gallow does not automatically delete source code.

## Commands and flags

```text
gallow [check|dead-code|audit] [flags] [root]
```

Important flags:

- `--base`, `--changed-since`: base ref for audit mode; default `HEAD~1`;
- `--root`, `-r`: repository root;
- `--format`, `-f`: `human`, `json`, or `sarif`;
- `--baseline`: suppress findings recorded in a baseline;
- `--save-baseline`: write the current findings to a baseline;
- `--workspace`: comma-separated module path or directory filters;
- `--tags`: comma-separated Go build tags;
- `--production`: exclude `*_test.go` files;
- `--all-requires`: also check indirect requirements;
- `--ignore-generated`: skip generated Go files;
- `--summary`: print only summary counts in human output;
- `--fail-on-issues`, `--ci`: exit with status 1 when findings exist.

## Configuration

Gallow reads `.gallowrc.json` from the selected root unless `--config` specifies another file.

```json
{
  "format": "human",
  "rules": {
    "duplicate-code": "warn"
  },
  "duplicateCode": {
    "minLines": 12,
    "minTokens": 100,
    "minTokenLines": 12,
    "minStatements": 3
  },
  "ignorePatterns": ["internal/generated/**"],
  "ignoreFindings": [
    {
      "type": "duplicate-code",
      "file": "**/*_test.go"
    }
  ],
  "workspace": ["example.com/app"],
  "buildTags": ["integration"]
}
```

Inline suppressions and saved baselines remain available for narrow, intentional exceptions.

## Finding types

| Type | Meaning | Preferred first action |
|---|---|---|
| `unused-dependency` | Direct requirement is not used | Remove the requirement |
| `unlisted-dependency` | Import lacks a direct requirement | Remove the import if unnecessary |
| `unused-package` | Package is unreachable | Remove the package |
| `unused-file` | File declarations are unused | Remove the file |
| `unused-function` | Function is unreachable | Remove the function |
| `unused-method` | Method is unreachable | Remove the method |
| `unused-struct` | Struct is unreachable | Remove the struct |
| `unused-interface` | Interface is unreachable | Remove the interface |
| `unused-type` | Type is unreachable | Remove the type |
| `unused-var` | Package variable is unreachable | Remove the variable |
| `unused-const` | Constant is unreachable | Remove the constant |
| `unused-field` | Field appears unused | Remove after checking dynamic use |
| `duplicate-code` | Code window appears in multiple places | Remove an obsolete copy |
| `tidy-drift` | `go mod tidy -diff` reports drift | Run tidy and inspect the diff |
| `tool-dependency` | Tool directive has no matching requirement | Remove the directive if unnecessary |

## Output contracts

- Human output is the developer contract.
- JSON is the stable automation contract.
- SARIF is the standardized CI and code-scanning contract.

The machine-readable report schema is versioned through `schema_version`.

## Development

```bash
go fmt ./...
go test ./...
go vet ./...
go run ./cmd/gallow --root . --summary
go run ./cmd/gallow audit --root . --base origin/main --summary
```

See [the product direction](docs/product-direction.md) and [the 0.2 migration guide](docs/migration-0.2.md).
