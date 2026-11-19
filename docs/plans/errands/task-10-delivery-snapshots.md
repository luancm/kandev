---
id: "10-delivery-snapshots"
title: "Persist result delivery snapshots and recovery evidence"
status: done
wave: 9
depends_on:
  - "00-upstream-baseline"
  - "05-errand-results-and-delivery"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-003
  - REQ-TASKS-ERRANDS-004
  - REQ-TASKS-ERRANDS-010
acceptance_criteria:
  - AC-TASKS-ERRANDS-003.5
  - AC-TASKS-ERRANDS-004.4
  - AC-TASKS-ERRANDS-004.8
  - AC-TASKS-ERRANDS-004.9
  - AC-TASKS-ERRANDS-004.10
  - AC-TASKS-ERRANDS-004.12
  - AC-TASKS-ERRANDS-010.7
system_design:
  - ../../specs/tasks/system-design/errands.md
---

# Task 10: Persist result delivery snapshots and recovery evidence

## Summary

Connect real result delivery to the already-built finished-card renderer and
make direct-prompt delivery recoverable. Use one immutable delivery envelope
in persisted messages and live events, with durable acceptance/readback
evidence that distinguishes a delivered result from an abandoned claim.

## In scope

- Serialize `Composition.Items` as the documented `errand_delivery` object
  with `token`, `route`, and snake-case item fields. Keep the separate
  `errand_delivery_token` used by recovery. Cover direct prompts with no
  queued entry, queued turns, notifications, and preserved deletion notices.
- Attach the envelope at the message persistence/admission boundary and
  include it in the live message event. Keep ordinary visible user text and
  strip system framing; do not derive historical cards from mutable rows.
- Reconcile both notification and turn-context claims using durable
  evidence. The current blanket exclusion of `turn_context` is insufficient.
  Test crash boundaries around claim, message persistence, provider
  acceptance, and receipt, including a real repository reload.
- Replace the misleading direct-prompt fixture that supplies a queued entry
  with a true nil-entry test. Retain independent frontend parser tests and
  add a wire-to-persisted-message contract test.

## Out of scope

- Queue wake/Send now dispatch and fresh-runtime retry integration (task 11).
- New card layouts, unbounded result payloads, and new delivery routes.

## Acceptance

1. Live and reloaded direct/queued/notification/deletion delivery messages
   contain the same bounded envelope, render finished cards, preserve user
   text, and remain readable after the errand row is removed.
2. Restart or definite admission failure neither loses a claim nor repeats
   accepted result text. Unknown acceptance is preserved for reconciliation;
   it is not treated as a definite refusal or inferred from launch success.
3. First-wins races among await, parent context, and notification preserve
   at-most-once text; results settling during a running turn wait for a later
   turn unless that turn explicitly awaits them.

## Verification

Use TDD and the repository work order's targeted checks. Run from root after
task 00 installs dependencies. Add cases in the named suites; every test
selector below must discover the new cases.

```bash
(cd apps/backend && go test -race -trimpath ./internal/errands/... -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator -run 'Errand|PromptTask|DeliveryToken' -count=1)
(cd apps/backend && go test -trimpath ./internal/backendapp -run 'Errand|DeliveryToken' -count=1)
(cd apps/backend && go test -trimpath ./internal/task/repository/sqlite -run 'Errand|MessageMetadata' -count=1)
(cd apps/web && pnpm exec vitest run components/task/chat/errands/errand-parse.test.ts components/task/chat/errands/errand-delivery-message.test.tsx)
git diff --check
```

Run `bash scripts/lint-go-changed` with the exact changed Go paths. Task 09
supplies the production-browser and reload proof; component fixtures alone
cannot complete the delivery acceptance.

## Files likely touched

- `apps/backend/internal/errands/{compose.go,delivery.go,reconcile.go}` and
  compose/delivery/reconcile/restart tests.
- `apps/backend/internal/orchestrator/{errand_turn_composition.go,task_operations.go}`
  and prompt/message persistence helpers; `errand_turn_composition_test.go`.
- `apps/backend/internal/backendapp/errands_transcript.go` and tests.
- Related message metadata repository tests and
  `apps/web/components/task/chat/errands/{errand-parse.test.ts,errand-delivery-message.test.tsx}`.

## Dependencies

Task 00 and historical task 05. Task 11 builds on this envelope and claim
contract; task 09 validates the complete rendered flow.

## Risks

- A late best-effort message update reproduces the crash window.
- Missing JSON tags or using a route string still passes isolated renderer
  fixtures while breaking real cards.
- Do not create a second delivery ledger or infer acceptance from a runtime
  being present. Reuse existing durable prompt/submission evidence.

## Parallelism

`sequential`

## Inputs

- System-design [contracts](../../specs/tasks/system-design/errands.md#websocket-contract),
  [parent delivery](../../specs/tasks/system-design/errands.md#parent-turns-and-delivery),
  and [recovery](../../specs/tasks/system-design/errands.md#failure-and-recovery).
- Existing UI-04/UI-09 structure in the plan; no layout change is required.

## Results

Primary accepted implementation and the recorded post-merge targeted checks on 2026-10-28. Later 12A-E checks preserve delivery, retry, cancellation and recovery integration. Browser/reload proof and provider/PostgreSQL readiness remain explicit downstream gates; this status does not claim those environments were certified.

Completed. Delivery messages now persist the immutable `errand_delivery`
envelope with snake-case item fields, a separate claim token, and a link to the
durable prompt submission. Direct prompts update the existing user message
before admission; queued prompts persist their snapshot with the transcript;
preserved cancellation notices keep their snapshot after the errand row is
deleted. Reconciliation now checks notification and turn-context claims and
releases only explicit failed or cancelled submissions, preserving unknown
outcomes.

Verification passed:

- `(cd apps/backend && GOFLAGS=-p=2 GOGC=50 go test -race -trimpath ./internal/errands/... -count=1)`
- `(cd apps/backend && GOFLAGS=-p=2 GOGC=50 go test -trimpath ./internal/orchestrator -run 'Errand|PromptTask|DeliveryToken' -count=1)`
- `(cd apps/backend && GOFLAGS=-p=2 GOGC=50 go test -trimpath ./internal/backendapp -run 'Errand|DeliveryToken' -count=1)`
- `(cd apps/backend && GOFLAGS=-p=2 GOGC=50 go test -trimpath ./internal/task/repository/sqlite -run 'Errand|MessageMetadata' -count=1)`
- `(cd apps/web && pnpm exec vitest run components/task/chat/errands/errand-parse.test.ts components/task/chat/errands/errand-delivery-message.test.tsx)`
- `bash scripts/lint-go-changed` with the exact changed Go paths
- `git diff --check`
