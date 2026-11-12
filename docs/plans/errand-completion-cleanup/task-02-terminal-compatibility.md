---
id: "02-terminal-compatibility"
title: "Reconcile cancelled and legacy Errand cleanup obligations"
status: done
wave: 1
depends_on: ["01-completion-cleanup"]
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRAND-COMPLETION-001
acceptance_criteria:
  - AC-TASKS-ERRAND-COMPLETION-001.1
  - AC-TASKS-ERRAND-COMPLETION-001.3
  - AC-TASKS-ERRAND-COMPLETION-001.4
system_design:
  - ../../specs/tasks/system-design/errand-completion-cleanup.md
---

# Task 02: Reconcile cancelled and legacy cleanup obligations

The user explicitly requested implementation through Errands after diagnosis on 2026-11-12. This is remediation of the existing reviewed completion-cleanup package: cancellation precedence, conservative legacy repair and independent retry remain authoritative. No new UI or recovery authority is authorized.

## Evidence and correction boundary

Six supplied warnings report `errand session reached incompatible terminal state CANCELLED`; another reports `legacy errand lacks positive initial admission authority`. `projectCompletedSession` refuses to overwrite cancellation, but leaves the completion intent retryable. `legacyCompletionSessionEligible` does not exclude cancelled or failed sessions before adopting a completion obligation. `captureLegacyTarget` refuses absent or mismatched initial admission evidence. `retryPendingTeardowns` repeats per-row warnings and the delivery caller logs an aggregate error even while independent delivery proceeds. The logs do not establish live runtime residency or which admission field is missing.

Reproduce both shapes with real SQLite and controlled runtime evidence before production edits. Prevent legacy success adoption for incompatible terminal sessions. Reconcile an existing completion obligation overtaken by cancellation without projecting success or losing an unconfirmed physical-stop obligation. Reuse exact-target ownership, cancellation precedence and existing CAS intent transitions; a terminal session label alone is not proof of runtime absence. For missing or conflicting legacy evidence, preserve the refusal and diagnostic context without repeatedly emitting identical warnings on every unchanged sweep. Preserve retry when evidence changes and for transient runtime/storage errors. Do not guess the current turn, clear evidence or certify unknown cleanup as complete. If this requires a new persistent authority or schema, report the boundary instead of introducing it.

## Ownership

One implementation Errand owns `apps/backend/internal/orchestrator/errand_completion_cleanup.go`, `errand_teardown.go`, adjacent teardown/completion regression tests, `apps/backend/internal/errands/teardown.go` and adjacent service tests. Small related changes in the existing sweep/logging owner are permitted only when required. The review Errand is read-only. Both are not alone in the codebase: preserve all other changes. The panel-controls task owns `docs/specs/tasks/requirements/errands.md`, `docs/specs/tasks/system-design/errand-status-and-continuation.md` and `docs/plans/errand-panel-controls/`; those and all frontend paths are excluded. Preserve the untracked loading-performance investigation. No commits, pushes, live DB/runtime changes, nested delegation or shared-plan rewrites by workers. Parent owns integration and delivery. Worker may write `terminal-compatibility-handoff.md` here.

## Acceptance and verification

Cover legacy cancelled/failed sessions with exact successful historical receipts; pending and closed completion intents encountering cancellation; live, absent and unknown exact runtime inventory; cancellation racing completion; successor/replacement ownership; missing and contradictory initial admission; repeated unchanged sweeps, changed-evidence recovery and transient-error retry. Assert preserved session outcome, result revision/claim/transcript, no replay or duplicate stop, and progress of healthy rows and result delivery in mixed inventory. Capture diagnostic output to establish repeated-warning behavior without hiding new failures.

Run the bounded package selectors with `-race`, count matched tests, and retain RED/GREEN receipts. Starting command: `go -C apps/backend test -race -trimpath -p 1 -parallel 1 ./internal/errands ./internal/orchestrator -run 'TestErrand.*(Completion|Teardown|Legacy|Sweep|Cancel)' -count=1 -json`; refine only to include the new cases, not to hide failures. Run affected-package lint through `bash scripts/lint-go-changed <owned Go paths>`, the backend application build, gofmt and whitespace checks. Run SQL guard/store conformance if persistence code changes; record PostgreSQL skips explicitly. No browser tests are needed for unchanged UI.

Serialize every heavy check under `/tmp/kandev-errands-repair-build.lock`, with GOMAXPROCS=2, bounded Go parallelism and a private cache. The earlier repair cache was removed; `/tmp/kandev-startup-repair-env.sh` describes the known bounded environment, but use an Errand-owned cache path and retain each process handle. Check disk before/after; remove only owned scratch/caches after checks exit. Report exact changed paths, commands, counts, hashes, resource cleanup and remaining limitations. Do not infer completion from a running label or elapsed time.

## Parent acceptance

Implemented and reviewed. Legacy adoption excludes cancelled and failed sessions; closed completion receipts retire without another stop or outcome overwrite; pending obligations still require exact stop evidence, authoritative absence or successor supersession. Diagnostic deduplication uses a bounded fingerprint so changed admission evidence is reported while identical refusals remain retryable. Final parent race selector passed 77 top-level tests without failures or skips. Scoped Go lint passed with zero issues after a cold package-loading timeout; the fresh managed desktop build and all 19 desktop cases passed against the combined frontend/backend source. No persistence schema changed, and no live Mac or PostgreSQL validation is claimed. Final phone acceptance and delivery evidence are tracked in [the parent integration handoff](../errand-panel-controls/final-integration-handoff.md).
