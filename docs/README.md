# Documentation Index

This index separates current contracts from historical artifacts. Historical
plans, logs, and handoffs are retained for context only; do not execute them
unless a current tracker issue explicitly reactivates them.

## Current Contracts

- [README](../README.md) -- user-facing install, support, and command overview.
- [AGENTS](../AGENTS.md) -- repository instructions for coding agents.
- [Current State](current-state.md) -- current product and implementation status.
- [Project Goals](project-goals.md) -- project direction and intended outcomes.
- [Roadmap](roadmap.md) -- planned work and sequencing.
- [Support Matrix](support-matrix.md) -- canonical language/backend support table.
- [JSON Output and Exit Codes](json-schema.md) -- normative `--json`, `doctor --json`, coordinate, versioning, and exit-code reference.
- [Release Process](release.md) -- repeatable v0.1 release commands and artifacts.
- [Position Encoding](position-encoding.md) -- CLI byte-column vs LSP UTF-16 column constraint.
- [LSP Landscape](lsp-landscape.md) -- language-server ecosystem notes.
- [Shatter LSP Client Parameters](shatter-lsp-client-parameters.md) -- guidance for LSP client parameter handling.
- [Refute Design Spec](specs/2026-04-15-refute-design.md) -- original architecture design reference.
- [Rust Parity Design Spec](specs/2026-04-22-rust-parity-design.md) -- Rust support design reference.
- [Stories README](stories/README.md) -- intent-story format and workflow.
- [Story Index](stories/INDEX.md) -- generated index of user-facing intent stories.
- [Doctor Backend Check Story](stories/doctor-backend-check.md) -- intent story for `refute doctor`.
- [Extract Function Story](stories/extract-function.md) -- intent story for function extraction.
- [Extract Variable Story](stories/extract-variable.md) -- intent story for variable extraction.
- [Inline Symbol Story](stories/inline-symbol.md) -- intent story for inline refactoring.
- [Rename Symbol Story](stories/rename-symbol.md) -- intent story for rename refactoring.
- [Plans Lifecycle](plans/README.md) -- required lifecycle status convention for plan files.
- [Issue Backlog](issue-backlog.md) -- tombstone; migrated into GitHub Issues (#84).

## Historical Artifacts

- [2026-05-02 Audit Report](audits/2026-05-02-audit-report.md) -- one-off audit findings.
- [Core + Go Rename Plan](plans/2026-04-15-refute-core-go-rename.md) -- executed implementation plan for the initial CLI and Go rename path.
- [Core + Go Rename Log](plans/2026-04-15-refute-core-go-rename-log.md) -- execution log for the initial CLI and Go rename path.
- [Go Code Actions Tier 1 v1 Plan](plans/2026-04-16-go-code-actions-tier1.md) -- abandoned first plan, superseded by v2.
- [Go Code Actions v2 Handoff](plans/2026-04-17-go-code-actions-tier1-v2-handoff.md) -- executed handoff protocol for the v2 code-actions plan.
- [Go Code Actions v2 Log](plans/2026-04-17-go-code-actions-tier1-v2-log.md) -- historical progress log for the v2 code-actions plan.
- [Go Code Actions v2 Plan](plans/2026-04-17-go-code-actions-tier1-v2.md) -- executed plan for Go extract, inline, and Tier 1 symbol resolution.
- [E2E Test Coverage Plan](plans/2026-04-20-e2e-test-coverage.md) -- executed E2E coverage plan.
- [Java E2E Test Plan](plans/2026-04-22-java-e2e-tests.md) -- executed Java E2E coverage plan.
- [2026-05-02 Audit Plan](plans/2026-05-02-audit-plan.md) -- executed audit playbook.
- [Rust Parity Completion Plan](plans/2026-05-09-rust-parity-completion.md) -- executed Rust parity completion plan.
- [Go Code Actions Expedition Plan](expeditions/go-code-actions/plan.md) -- expedition plan for the landed Go code-actions work.
- [Go Code Actions Expedition Log](expeditions/go-code-actions/log.md) -- expedition progress log for the landed Go code-actions work.
- [Go Code Actions Expedition Handoff](expeditions/go-code-actions/handoff.md) -- closed handoff retained to prevent stale relanding instructions.
- [Go Code Actions Expedition State](expeditions/go-code-actions/state.json) -- machine-readable state showing the expedition landed on `main`.
