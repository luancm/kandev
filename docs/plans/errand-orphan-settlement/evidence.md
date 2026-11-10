# Orphan Errand incident, 2026-11-08

Read-only inspection of `/home/luancm/.kandev/data/kandev.db` and `/home/luancm/.kandev/logs/backend-logs.log`. The instance discovery helper reported backend PID 1295821, port 10101, checkout `/home/luancm/projects/kandev`. No live state was changed.

Errand/session `1a1282fd-702c-4e74-a77e-54fa890e8e70` was created at 12:03:54 UTC. Its original turn is `23d5cc8f-547d-4097-b763-839aa727c0e3`, submission `prompt:errand:676e27de-7711-4d94-beca-4dddb0fcd025`.

- At 12:13:29.101 the backend logged abandonment of the original turn. Its completion equals its start, consistent with `AbandonTurn`.
- At 12:13:29.181 the session was reconciled for lazy recovery, previous state RUNNING, `retracked=false`.
- At 12:13:29.692 Errand reconciliation logged `preceding errand turn has no committed exact settlement decision` and skipped the launched row.
- Recovery-created turn `agent-delivery-turn-b48b51e4-65a0-4ac3-a913-b0a87aa45fed:g1-502` was also abandoned at 12:13:30.655.
- A generation-two turn ending in `g2-14` completed at 12:13:42.443. Its persisted message is a model-resume warning, not a work-completion report. Neither generated turn has lifecycle-only metadata or an Errand settlement receipt in the inspected state.
- The original settlement effect remains pending with unsealed outcome `parent`. It is the session's only persisted delivery effect. The submission is `interrupted_unknown`, with no terminal stream/sequence/turn binding.
- At inspection the child session is WAITING_FOR_INPUT and its native thread metadata is idle, while the Errand is `launched`, `awaiting_result=1`, `result_state=none`, and has no settlement time.

`AbandonTurn` updates timestamps only. `ReconcileTerminalDecisions` requires a sealed exact outcome. `PendingErrandTurnSettlements` also rejects any completed non-lifecycle turn without an exact receipt, except the reserved first-turn identity. The terminal-evidence resolver accepts matching projected terminal effects; no such effect exists here. Thus recovery can close the session's turns without leaving evidence that the Errand's exact-settlement consumer accepts. The running presentation is downstream of this durable mismatch.

No raw ACP files for the session were found in the inspected log directory. The original runtime's precise provider outcome remains unverified. Session recovery metadata records reconstruction and termination of the old process, but a fix must validate that evidence through the owning recovery contract rather than interpret a metadata blob as unconditional terminal authority.

The workspace still has HEAD `11c2b68f86` and the staged upstream merge. This investigation did not commit, push, restart, cancel, or alter the Errand.
# Recurrence during implementation, 2026-11-09 22:05 UTC

Read-only SQLite (`mode=ro`, `query_only=ON`) and retained backend log inspection confirmed current implementation Errand/session `c6663ad7-973c-45e1-9f38-ce0a9a67a152` hit the same missing-settlement failure class. At 21:58:38.641 UTC, `backend-logs.log:9191` records abandonment of original turn `d6fc6ca2-c224-48ce-93f3-3624bc28f9a7`; line 9192 records lazy recovery from RUNNING with retracked=false. Line 9288 records reconciliation skipping the launched Errand because its preceding turn lacks a committed exact settlement decision.

At inspection, the Errand remains launched, awaiting_result=1, result_state=none and unsettled; its session is WAITING_FOR_INPUT. Its sole settlement effect is pending with unsealed parent outcome. Original submission `prompt:errand:c765963f-c255-4265-8e47-d78a9447ddbd` remains dispatching without terminal coordinates. Last retained agent activity is 21:57:30. The user's 22:03:22 continue attempt created open turn `915a07e0-974d-459d-b5d7-34ce3cc2f810`, but forwarding failed with the same exact-settlement error (`backend-logs.log:9514`); the transcript contains Failed to send message to agent. Thus projected running does not establish active work, and continue did not resume dispatch. Provider outcome and original-process death are not established by this inspection. No live state was changed. Preserve partial source/tests and independently establish writer quiescence before replacement.
