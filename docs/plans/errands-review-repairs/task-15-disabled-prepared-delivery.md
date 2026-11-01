---
id: "15-disabled-prepared-delivery"
title: "Gate prepared results without losing user input"
status: pending
wave: 15
depends_on:
  - "14-send-now-source-identity"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-003
  - REQ-TASKS-ERRANDS-004
  - REQ-TASKS-ERRANDS-012
acceptance_criteria:
  - AC-TASKS-ERRANDS-003.5
  - AC-TASKS-ERRANDS-004.7
  - AC-TASKS-ERRANDS-004.8
  - AC-TASKS-ERRANDS-004.9
  - AC-TASKS-ERRANDS-004.10
  - AC-TASKS-ERRANDS-004.11
  - AC-TASKS-ERRANDS-012.2
  - AC-TASKS-ERRANDS-012.3
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 15: Gate prepared results without losing user input

## Scope and authority

Repairs review finding(s) 19 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. No production changes have been made for this packet.

## Low-level implementation and regression plan

Own `apps/backend/internal/orchestrator/{errand_parent_deliverer.go,errand_turn_composition.go,queue_send_now_errand.go,queue_send_now.go,event_handlers_agent.go}`, `internal/errands/delivery.go`, source-selection helpers in `internal/orchestrator/messagequeue/` and adjacent integration tests. Coordinate queue JSON changes with Task 14; no second result outbox is introduced.

Use the existing durable server notification queue row, not a second result outbox. At enabled deletion, `PreserveCancellationNotice` idempotently creates one server-owned source keyed by `(task, parent incarnation, errand ID, cancellation revision)`, containing the complete bounded prepared block and immutable display snapshot, typed provenance, and the original queue source ID/generation. Confirm durable admission before deleting; queue-full/error prevents deletion. Retry after a crash reuses the same source. The source owns the full notice independently of the errand row; its preview is insufficient.

The source stays in the queue while pending. Dispatch may remove it only in a durable queue claim containing the full original source, ID/generation and exact dispatch/receipts. The claim owns it: definite refusal restores the source; acceptance settles with admission/transcript evidence; uncertainty retains it without replay. Every crash leaves source or claim recoverable. Terminal parent/task cleanup may discard it under existing rules.

With Errands disabled, drain and Send Now bypass untouched typed server-only sources before reservation or dispatch, preserving ordinary FIFO. A notification-only Send Now does not interrupt a live turn. For existing mixed claims, definite refusal restores the whole exact claim before ordinary-only reselection; accepted/uncertain claims remain owned, without replay. On enabled restart normal drain delivers the retained source once. Preserve capacity, auto-run, CREATED/fallback and incarnation fences.

Retain legacy prepared sources with proved provenance and full content; normalize metadata without changing their IDs. For legacy mixed claims use the persisted complete original `Sources` to rebuild only after definite refusal and full restoration. Missing or contradictory provenance, full content, source IDs or admission blocks the source/claim for inspection. Never strip text, fabricate a notice, drop user input or replay uncertainty; lookalike user text stays ordinary.

Implement source-aware bypass in both normal FIFO drain and Send Now before claiming or interrupting an active turn. The preserved server sources continue to count against existing queue capacity; never evict them to manufacture free capacity. Ordinary sources retain their relative FIFO order. If all candidates are disabled server-only sources, return the existing feature-disabled/no-work outcome and leave them durable. Queue cleanup is task/parent-owned and preserves the existing terminal cleanup policy.

RED: `TestPreparedErrandCancellationDoesNotDeliverAfterDisabledRestart`, `TestDisabledErrandSendNowPreservesFoldedUserInput`, `TestDisabledErrandPreparedHeadDoesNotBlockOrdinaryQueue`, `TestDisabledErrandMixedClaimRecoveryRestoresBeforeReselection`, `TestLegacyPreparedErrandUnknownProvenanceIsRetainedNotStripped`. Use real SQLite/queue persistence, enabled enqueue/delete then reopen disabled and enabled, both source orders, full bounded notice versus preview, literal system tags in user text, attachments/references, capacity, auto-run, CREATED/fallback and accepted/uncertain/refused claims. Positive enabled controls must prove actual provider delivery is reachable.

```bash
(cd apps/backend && go test -trimpath -race ./internal/orchestrator -run 'PreparedErrand|DisabledErrand|LegacyPreparedErrand|SendNow.*Errand|Delete.*Errand|Errand.*Delet' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/errands -run 'PreserveCancellation|Delivery|Reconcile|Feature|Disabled' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/orchestrator/messagequeue -run 'SendNow|Restore|Admission|Errand' -count=1)
git diff --check
```

Run changed-file Go lint. The flag remains restart-required and off in shipped profiles. Retained child sessions remain viewable and isolated.

## Dependencies and ownership

[Task 14](task-14-send-now-source-identity.md)

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

`sequential`. The integrating session may overlap disjoint read-only preparation, but grants one writer for each shared schema, repository, orchestration or browser file.

## Results

Pending. Record each final command, observed result, test count and log path; distinguish skips, infrastructure failures and reproduced defects from passes.
