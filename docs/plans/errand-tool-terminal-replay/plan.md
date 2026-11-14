---
created: 2026-11-14
status: done
requirements:
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-003
system_design:
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
legacy_specs: []
---

# Repair completed Errand delivery blocked by late tool updates

The parent recovered the completed Task03 review from its transcript, but its successful terminal remains behind a rejected late tool update. The [incident evidence](../errands-remaining/verification-result-delivery-incident.md) records the exact SQL and journal sequence. No live record was edited. Implementation and bounded verification are complete in the primary session; [the handoff](implementation-handoff.md) records source and check receipts. Further Errand launches remain held until the running instance includes the repair.

The confirmed storage gap is that missing-submission tool updates can correlate only to unowned tool starts. The observed original tool start belonged to an admitted prompt, so no eligible source exists and replay rejects the update before accepting the following completion. The adapter RED also established a producer issue: generic tool calls are inserted into the active map even when their initial status is already terminal, and prompt-end cleanup emits cancellation without the original prompt identity. The final adapter regressions cover that sequence and legitimate pending cleanup.

Execute [one bounded work order](task-01-exact-tool-replay.md): reproduce the adapter sequence, correct active-tool tracking and exact provenance, then exercise replay and settlement together. Preserve the [recovery authority decision](../../decisions/2026-11-09-errand-recovery-settlement-authority.md): never assign unowned work by proximity or today's active turn. Retained proof may authorize exact tool attribution; it does not itself authorize terminal settlement. The successful terminal must still pass its existing admission and result guards.

| Boundary | Required behavior | Evidence |
| --- | --- | --- |
| ACP initial terminal tool call | Never becomes pending solely for prompt-end cancellation | Adapter regression with completed initial call and pending control |
| Late update of an admitted tool | Preserve original tool owner and immutable received bytes | Durable provenance and replay regression |
| Reused tool ID or conflicting owner | Fail closed, preserve both conversations | Successor, parent-tool, native-session and generation controls |
| Ordinary session and other ACP providers | Preserve existing tool rendering and legitimate cancellation | Shared adapter and ordinary persistence controls |
| Historical missing proof | Remain unresolved; no skipped journal entry or forged receipt | Negative replay tests after source removal/ambiguity |

No UI, new recovery override, database deletion, live runtime restart, automatic prompt replay, or publication is included. Desktop and phone continue using the existing result/status projections. Supported repair depends on exact retained evidence; this plan does not claim every historical Errand is recoverable. The existing recovery design now records admitted-source retention and historical reconstruction without changing terminal authority. No new public product behavior is proposed.

Acceptance requires reproduction of the observed owned completed tool → missing-identity cancellation → successful terminal sequence, exactly one result after replay and retry, and unchanged successor/ambiguity guards. Parent integration and delivery remain separate. The work order records the current implementation and verification state.
