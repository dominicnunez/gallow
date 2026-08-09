# Migrating to Gallow 0.2

Gallow 0.2 narrows the product to repository entropy analysis and increments the machine-readable report schema to version 2.

## Removed finding IDs

- `complexity`
- `test-only-dependency`
- `local-replace`

Remove matching rule and suppression entries from configuration and automation.

Dependencies used only by tests continue to count as used when tests are included; they no longer produce a finding.

## Removed flags

- `--max-cyclomatic`
- `--max-cognitive`
- `--coverage`

The CLI returns a migration-specific error when these flags are supplied.

## Removed configuration

Remove the `health` object and its `maxCyclomatic` and `maxCognitive` settings. Gallow returns a migration-specific error while that object is present.

## Removed output formats

- `codeclimate`
- `gitlab-codequality`
- `annotations`

Use `--format sarif` for supported CI and code-scanning systems. Use `--format json` with a small adapter elsewhere.

## Removed analysis data

Finding-level coverage annotations and complexity metrics are no longer present in JSON or SARIF. Consumers must stop reading those fields.

## Added remediation data

Supported findings now include a `remediation` object with a preferred action, summary, and optional alternatives. SARIF exposes the same object under result properties.
