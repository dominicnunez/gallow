# Product Direction

Gallow is a Go repository entropy analyzer.

Its core scope is:

- dead and unused code;
- unused or unlisted dependencies;
- duplicate code;
- tidy and tool/module drift;
- branch-relative audit;
- baselines and narrow suppressions;
- human, JSON, and SARIF output;
- evidence and removal-first remediation.

The primary product question is:

> What did this change leave behind that the repository probably does not need?

## Boundaries

Gallow does not expand into correctness, security, architecture governance, style, generic complexity scoring, runtime coverage, release hygiene, or editor/LSP tooling.

Those concerns have different evidence models and are better served by dedicated tools.

## Roadmap

1. Improve dead-code confidence and explanations.
2. Improve generated-code, reflection, build-tag, interface, and workspace precision.
3. Strengthen duplicate confidence and branch attribution.
4. Make `audit --base` the center of CI and review workflows.
5. Explore a narrow MCP interface for AI coding agents after the JSON contract is stable.

Autofix remains deferred until findings are precise enough to support an explicitly reviewed mode. Arbitrary source deletion is out of scope.
