# Parent notification dispatch refusal, 2026-11-10

Read-only live-instance triage. Parent session `1f764ecc-824e-4065-a31d-96b2db9ee9b0`, task `29d30fd5-4243-4b3a-99b5-2a76931fa7ef`, deployed core `b2bbe9d5b7`. Instance discovery reports PID 1047962, port 10101, repository `/home/luancm/projects/kandev`. Evidence is in `/home/luancm/.kandev/logs/backend-logs.log` and read-only `/home/luancm/.kandev/data/kandev.db`.

## Observed timeline

- At 23:18:54.720 UTC the server queued empty entry `5edff3ad-ce99-4fdf-9f5a-33223b6db520`. At 23:18:54.854 the completion drain logged `failed to settle delivery before queued successor`, with `guarded non-workflow delivery outcome was not committed`, and requeued that entry at the head with `queued_by=server`.
- At 23:18:56.542 the outstanding-Errands workflow no-op logged the same failure. Similar refusals occurred at 22:41, 22:44, 22:48 and 23:15.
- At 23:33:53 the user's next direct prompt delivered the two reviewer results by `turn_context`. At 23:33:54.266 the server removed that queued entry. The read-only queue query in this investigation returned no remaining entries for the parent.

## Exact evidence and current hypothesis

The completed parent turn is `8e5d9118-9ee6-4a97-96ad-806ab646ce84`; submission `prompt:queue:182e257c-6585-4ec8-bd2f-74a243b4493f` is recorded completed. Its terminal effect `agent_delivery.terminal:976e2cb9-cf79-4458-b86e-70c8669070dc:g1:74030` is `block_resolved`, outcome completed, but has empty `turn_id`. The submission's terminal stream/sequence/turn fields are likewise empty/zero. The prompt-admission effect does contain the real turn ID. Session and submission incarnation IDs agree.

`settleQueuedDeliveryPredecessor` requires a guarded workflow no-op. `RecordAgentDeliveryWorkflowOutcome` calls `agentDeliveryWorkflowCursorMatchesTx`, whose query requires the stored terminal effect's `turn_id` to equal the nonempty workflow effect turn. The observed terminal cannot satisfy that predicate. This explains a concrete refusal boundary; it does not yet establish why terminal identity was lost or authorize weakening the check. Review the producer/binding path and exact evidence recovery before a fix.

The historical stranded child remains outstanding and can route the parent through Errands workflow deferral, but active Errands are not themselves supposed to disable queue auto-run. The blank server notification is normal deferred composition; its missing visible explanation is separate from the dispatch failure. No claim of another guard deadlock is supported by this evidence. No live state, runtime or configuration was changed. No diagnostic processes or scratch were created.

## Next step

### Completed worker with uncertain result delivery

On 2026-11-11 the recovery coordinator independently inspected frontend worker `31e441fc-8c9e-47d2-be87-2b4ac183890e`: its conversation records final work/ownership release at 10:05:07 and `frontend-recovery-handoff.md`, followed by a prompt-delivery-uncertain error at 10:05:12. The Errand projection still reports running. This evidence is reported by the coordinator; the parent has not independently reconstructed that delivery transition. It is distinct from the 01:18 abandoned-worker incident. Do not infer that frontend implementation remains active or restart it as a writer; the coordinator is reviewing its saved patch with Era Sol. Reported frontend checks passed, while exact-settlement browser continuation remains unproven. Do not assume this completion-delivery uncertainty has the same root cause as the parent's terminal-identity refusal without tracing its receipts.

### Provenance established on 2026-11-11

Git comparison with merged upstream `52309e613f` identifies an Errands integration regression. Commit `9db45d3e75` changed `durableAgentEvent` in `internal/agent/runtime/lifecycle/durable_delivery_stream.go` so missing `TurnID` is filled from execution only when `DeliverySubmissionID` is also empty. This guard applies to ordinary parent sessions as well as Errands. The replacement `resolveRetainedEventTurn` and canonical resolver bind turn identity only for Errand sessions, leaving an ordinary submission-bearing terminal without the previous fallback. That matches the live terminal's empty turn ID.

The rejecting cursor/terminal match originated upstream in `e6692746b0` and remains unchanged; `agent_delivery_settlement.go` in the orchestrator also matches merged upstream. The Errands change to workflow-outcome validation only admits the additional `errands_outstanding` outcome and does not cause this identity predicate to fail. Therefore the safety check is pre-existing, but our broader identity change introduced the observed compatibility gap. This conclusion is based on source/history and live records; a before/after executable regression is still required to prove the full path.

Repair must preserve exact admission ownership and stale-event protection, restore ordinary parent identity handling, and address already persisted incomplete effects only with exact evidence. Do not simply remove the terminal identity check or assign the current turn to historical events. No production fix is implemented yet.

Read-only source/evidence review of the identity loss and minimal regression boundary, then a scoped fix packet coordinated with the recovery owner. Preserve exact terminal authority, queue FIFO and no-duplicate delivery. This incident remains unresolved even though the specific entry was consumed by the user's prompt.

The recovery subtask confirmed that its pending attention/continuation patch does not repair ordinary parent-session terminal-effect binding and changes no workflow-outcome or terminal-identity persistence files over `b2bbe9d5b7`. Its remaining backend writer owns only the recovery-state observer notification and focused tests. Era Sol Errand `e0c9a24e-8509-4c60-8d27-703f74dba321` owns this separate read-only diagnosis review; no production owner has been assigned for the incident.
