---
id: "08-created-errand-isolation"
title: "Keep created and recovered errands detached"
status: done
wave: 8
depends_on:
  - "05-parent-settlement-guard"
  - "07-settle-before-successor"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-001
  - REQ-TASKS-ERRANDS-005
  - REQ-TASKS-ERRANDS-012
acceptance_criteria:
  - AC-TASKS-ERRANDS-001.2
  - AC-TASKS-ERRANDS-001.3
  - AC-TASKS-ERRANDS-005.1
  - AC-TASKS-ERRANDS-005.2
  - AC-TASKS-ERRANDS-005.3
  - AC-TASKS-ERRANDS-005.5
  - AC-TASKS-ERRANDS-012.3
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 08: Keep created and recovered errands detached

## Scope and authority

Repairs review finding(s) 9 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. Implementation is complete in the owned orchestrator launch path and its real-entry regressions; no API or schema changes were made.

## Low-level implementation and regression plan

**Owned:** `internal/orchestrator/task_operations.go:startCreatedSession,scheduleTaskForSession`, new helper `internal/orchestrator/errand_created_launch.go`, `errand_session_prompt.go` only if extending trusted row reconstruction; new `errand_created_launch_test.go`. Fallback tests in `prompt_launch_fallback_test.go` may add one real entry-path case. Do not rewrite generic fallback transport/acceptance logic.

**Algorithm:** Resolve the loaded session's immutable role once; a read failure aborts. For `Role=errand`, before any workflow/profile/task-state side effect:

- Use the retained session profile (for a prepared launch that is the requested profile persisted at Errand admission); validate compatibility through existing profile/executor validation. Do not call `resolveEffectiveAgentProfile` or dynamic workflow profile routing. Explicit user runtime override semantics remain those of the retained session; no workflow profile may replace it.
- Use an empty deferred-launch claim, the existing errand counterpart to `deferredLaunchClaimForStart`. Do not consume, clear or rewrite the task's ordinary deferred slot.
- Skip task SCHEDULING. Also guard `scheduleTaskForSession` by a positive loaded errand role so future callers cannot write it accidentally. Preserve archive/task safety validation; bypass only the state write, not task eligibility.
- Skip workflow step prefix/suffix/template, completion-signal, fixed-profile route binding and generated-title effects. Retain user-requested plan mode, accepted entity-reference context (including an explicitly empty expansion), attachments and errand-only prompt framing through `wrapCreatedSessionPrompt`/`wrapTaskSessionPrompt`. `ClaimTaskTitleSession` is already role-guarded; do not weaken it.
- Set `McpModeErrand` unconditionally by role through the existing executor resolver, even when feature off. Preserve executor/environment/workspace attach-only recovery and all cancellation/ceiling/recovery guards.
- If a loaded errand became terminal, do not redirect it to an ordinary active task session; return existing session-superseded/refusal. Ordinary workflow redirection remains unchanged.
- Preserve precomposed prompt/retry snapshots and initial acceptance callbacks. This policy cannot treat `Launch` success as prompt acceptance.

**Regression:** `TestCreatedErrandSessionKeepsExplicitProfileAndTaskState` enters real `startCreatedSession` with `CREATED` errand profile `errand-profile`, workflow step profile `step-profile`, task TODO/REVIEW, sentinel workflow prefix, ordinary task deferred launch and canonical environment. Assert runtime request uses errand profile/mode and exact user/reference/plan context, no step prefix/completion signal, unchanged task state/step/deferred metadata/primary/title owner/binding. `TestErrandMissingExecutionFallbackKeepsDetachedPolicy` enters real `promptTask` and faults the first `executor.Prompt` with `ErrExecutionNotFound` after lazy resume; assert same properties at fallback request and original errand session identity. Feature-on/off table, ordinary-session positive control still uses workflow profile.

```bash
(cd apps/backend && go test -race -trimpath ./internal/orchestrator -run 'Test(CreatedErrand|ErrandMissingExecutionFallback|LaunchErrandSession|InitialSubmissionReplay.*Errand|HandlePromptDispatchFailure|PromptTask.*Fallback|StartCreatedSession)' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/orchestrator/executor -run 'Errand|TaskSessionMCPMode' -count=1)
bash scripts/lint-go-changed apps/backend/internal/orchestrator/task_operations.go apps/backend/internal/orchestrator/errand_created_launch.go apps/backend/internal/orchestrator/errand_session_prompt.go
git diff --check
```

**ACs:** `001.2`, `001.3`, `005.1`, `005.2`, `005.3`, `005.5`, `012.3`. Private helpers only; no API/schema change. Task 13 depends on this so recovery cannot route through the broken generic start.

## Dependencies and ownership

[Task 05](task-05-parent-settlement-guard.md), [Task 07](task-07-settle-before-successor.md)

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

`sequential`. The integrating session may overlap disjoint read-only preparation, but grants one writer for each shared schema, repository, orchestration or browser file.

## Results

Completed 2026-11-01. TDD RED reproduced the workflow-profile override on all four feature-flag/task-state combinations, terminal errand redirection, missing archive refusal and profile override during the real missing-execution fallback. The ordinary workflow positive control, role-read failure and task eligibility guard passed before implementation.

- Orchestrator race suite: `(cd apps/backend && go test -race -trimpath -p 1 -parallel 1 ./internal/orchestrator -run 'Test(CreatedErrand|ErrandMissingExecutionFallback|LaunchErrandSession|InitialSubmissionReplay.*Errand|HandlePromptDispatchFailure|PromptTask.*Fallback|StartCreatedSession)' -count=1)` → `ok`, 5.087s. The selector matched 49 top-level test functions; subtests ran under their parents. Log: `/tmp/errands-task08-green-orchestrator.log`.
- Executor race suite: `(cd apps/backend && go test -race -trimpath -p 1 -parallel 1 ./internal/orchestrator/executor -run 'Errand|TaskSessionMCPMode' -count=1)` → `ok`, 1.166s. The selector matched 25 top-level test functions; subtests ran under their parents. Log: `/tmp/errands-task08-green-executor.log`.
- Final changed-package/new-lines lint: `golangci-lint run ./internal/orchestrator --new-from-rev=74a91cfc161916f0d5264b86762adae14e43119e --timeout=5m --concurrency=1` → `0 issues`. Log: `/tmp/errands-task08-lint-final.log`. The repository helper's first concurrency-2 run found a `nestif` issue that was fixed; a GOMAXPROCS-1 helper run timed out during package loading and is recorded at `/tmp/errands-task08-lint-single.log`.
- `gofmt -l` on all four owned Go files returned no files; `git diff --check` passed. The pre-edit inventory, hashes and `task_operations.go` diff are retained at `/tmp/errands-task08-inventory-before.txt`, `/tmp/errands-task08-hashes-before.txt` and `/tmp/errands-task08-task_operations-before.diff`.
- No edits were made to `errand_session_prompt.go` or to the shared repair manifest. Changes remain uncommitted and undeployed.
