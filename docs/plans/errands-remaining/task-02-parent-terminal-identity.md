---
id: "02-parent-terminal-identity"
title: "Repair ordinary parent terminal identity after Errands integration"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-TASKS-ERRANDS-004
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.2
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3
  - AC-TASKS-ERRANDS-004.9
  - AC-TASKS-ERRANDS-004.11
system_design:
  - ../../specs/platform/system-design/durable-agent-delivery.md
  - ../../specs/tasks/system-design/errands.md
---

# Task 02: Restore exact ordinary-session terminal identity

## Outcome and existing authority

Correct the compatibility regression introduced by the already integrated Errands recovery change. This is remediation within the authorized integration, preserving existing durable-delivery and Errands notification contracts; it adds no new product behavior, UI, recovery action or relaxed safety policy. The [incident evidence](queue-notification-incident.md) records the exact live sequence and commit provenance. Recovery integration is blocked until this correction and executable evidence are included.

`9db45d3e75` removed the missing-turn fallback for all submission-bearing durable events. Its retained/canonical admission resolver only binds Errands, so ordinary parent terminal records can retain an empty turn ID. Existing upstream guarded workflow settlement rejects that incomplete identity, leaving an Auto-run notification queued. Existing prompt admission evidence identifies the original turn; latest/current turn guessing is not an acceptable repair.

## Implementation boundary

Use existing exact submission/admission ownership to resolve the original turn for ordinary sessions as well as Errands when producer terminal events lack it. Preserve explicit producer identity, missing/conflicting evidence rejection, native asynchronous activity without submission, and Errand-only policy distinctions. Do not turn the resolver's existing Errand-role boolean into a generic success flag if callers use it to gate Errand behavior. Keep received payloads immutable; resolved attribution belongs in derived projection/callback/effect state.

Already stored incomplete terminal effects need idempotent reconciliation from their own retained terminal and exact admission evidence. Wire it into the existing settlement/reconciliation path, with existing writer locks and conditional writes. A completed submission alone is insufficient. Validate session/task, incarnation, harness generation, stream/submission, original turn association and any supplied prompt/native ownership evidence. Repair only an empty derived identity that is unambiguously proven; never overwrite a conflicting identity, advance a stale successor, synthesize provider completion, mutate raw inbox bytes or resend a prompt. Keep the guarded workflow predicate intact. This packet does not authorize editing the live database.

## Acceptance

1. A deterministic ordinary-parent regression fails on the current branch: a terminal with a delivery submission but no producer turn ID reaches durable projection and prevents notification dispatch. The corrected path records the original identity, commits the normal workflow no-op and dispatches one notification with Auto-run enabled, without Send now or another user prompt. The user independently confirmed that manual Send now succeeds where automatic dispatch stalls; manual success is a control, not acceptance of the repair. Include an ordinary queued user-message control and preserve prior transcript output before successor release.
2. Existing incomplete effects with matching exact admission/terminal evidence converge through normal reconciliation, remain stable on repeated restart/replay, and release eligible queued work once. Mixed inventory includes a valid repair and a conflicting/missing-evidence record; the latter remains protected. Test SQL failure/rollback and no duplicate provider dispatch.
3. Delayed historical terminals cannot inherit a newer turn, cancel or complete its execution, or overwrite its result. Preserve Errand ownership tests, cancellation behavior, native asynchronous no-submission behavior, immutable payloads and workflow ownership. Do not replace strict checks with a missing-ID bypass.

## Ownership and dependencies

One Lunaldinho implementation writer, then Era Sol review. Parent owns specifications, this work order, integration and final delivery. The worker is not alone: recovery subtask owns attention/refusal frontend work in another checkout; duration implementation and cleanup regression files are excluded. Do not revert other changes, launch children, stage, commit, push or change the live instance.

Likely owned code: lifecycle `durable_delivery_stream.go`, `retained_event_ownership.go`, `received_delivery_projection.go`, `streams_events.go` and adjacent identity tests; SQLite `agent_delivery_admission.go`, `agent_delivery_projection.go`, `agent_delivery_settlement.go` and focused adjacent tests; existing orchestrator delivery settlement/reconciliation integration and a new focused parent identity regression. Use a small adjacent helper only when needed to preserve lock/transaction boundaries. Avoid unrelated lifecycle changes and do not edit the shared recovery/frontend work. Return a `parent-terminal-identity-handoff.md` beside this file.

No code dependency on cleanup or duration. Integration acceptance of the recovery feature depends on this remediation. If exact reconciliation requires a new durable authority or a materially different contract, report that boundary before expanding the patch.

## TDD and verification

Use tests named `TestParentTerminalIdentity*` for every new regression. Prefer real SQLite plus existing controlled runtime/provider boundaries, exercising the actual intake/projection/settlement/dispatch path rather than only a helper. Capture RED before implementation; distinguish source-level evidence from executable proof. Use barriers or existing `testing/synctest`, never sleep-based race assertions.

Run from repository root, serialize heavy checks with the shared build lock and retain every process handle:

```bash
(cd apps/backend && flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -race -trimpath -p 1 -parallel 1 ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite ./internal/orchestrator -run 'TestParentTerminalIdentity' -count=1 -v)
(cd apps/backend && flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -race -trimpath -p 1 -parallel 1 ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite ./internal/orchestrator -run 'Errand.*(Owner|Ownership|Admission|Terminal)|RetainedTerminal|Durable.*Identity|QueuedDeliveryPredecessor' -count=1)
(cd apps/backend && flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB go build -trimpath -p 1 ./internal/backendapp)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Record matched test counts so a zero-match selector is not a pass. Supplement selectors for all changed existing tests, run exact-file `scripts/lint-go-changed`, formatting and the SQL guard/conformance checks required by backend guidance. PostgreSQL requires an available DSN; record explicit skips and limits. No browser changes are planned: provider-boundary queue dispatch is the acceptance observation. Check disk before and after heavy validation and remove owned scratch without deleting active shared caches.

## Results

Implementation is saved, but executable acceptance remains incomplete after Era Sol review `e4c08c5d-ac1d-4bdb-9600-43401ad5331d`. The ordinary parent path now resolves terminal identity from exact admission evidence, and reconciliation repairs only an empty projected turn identity when the retained terminal and immutable prompt admission agree. The workflow guard remains unchanged; missing or conflicting evidence keeps queued work held. See the [implementation handoff](parent-terminal-identity-handoff.md) for RED/GREEN evidence, exact test counts and hashes, validation limits, and source hashes.

The focused and broad SQLite-backed race suites pass, as do SQL guard, persistence store conformance, backend build, formatting, documentation catalog validation, and specification lint. PostgreSQL cases were skipped because `KANDEV_TEST_POSTGRES_DSN` is unset. The required changed-file lint script reached its five-minute timeout and reported one QF1008 finding in the separate cleanup worker's `errand_completion_cleanup_test.go`; the same affected-package diff scan with a twelve-minute timeout completed and reported only that external finding. No finding was reported in this packet's files.

Era Sol found no definite production defect, but raised one P1 coverage gap: the ordinary queued-user-message test bypasses empty server notification admission, dispatch-time result composition and delivery accounting, while the lifecycle test uses a canned resolver. The next bounded packet must connect real SQLite admission to lifecycle intake/projection for a submission-bearing terminal without TurnID, then dispatch an empty server-generated notification containing pending Errand results through Auto-run. Repeat for a repairable persisted incomplete effect through normal reconciliation, retaining missing/conflicting-evidence controls. Assert composed provider input exactly once, original-turn outcome, queue drain, result-delivery accounting and preserved transcript/successor state. Keep the ordinary user-message case as a control. No manual Send Now or follow-up prompt counts as acceptance. Review disposition, integration and delivery remain open; no Git or live-runtime mutation has occurred.

Remediation packet `05eb3360-6708-4c83-91d4-c1dd7fec4ab5` is ready for follow-up review: real SQLite lifecycle identity resolution and normal empty server-notification admission/Auto-run are exercised across their package boundaries. Nine focused top-level race tests and four notification/control tests passed; SQLite matched zero tests. No production code changed. See [notification acceptance handoff](notification-acceptance-handoff.md) for fixture boundaries, four nested notification cases, hashes and outstanding lint limits.

Era Sol follow-up `3b49fda5-565c-4157-b816-440f862b683a` accepted the P1 disposition and verified both revised test hashes and receipts, with no new blocking findings. The paired package-local SQLite tests are adequate without a production test-only API. PostgreSQL is not proven; active-successor protection remains separate control evidence. Parent removed the remaining redundant embedded selector from the cleanup test and is running scoped lint before committing this checkpoint. Integration and final delivery remain pending.

Parent integration lint attempt exited 4 after the script's five-minute package-loading timeout; `/tmp/kandev-parent-terminal-integration-lint.log` contains no new code finding. The process and compiler children exited and the lock passed to queued browser work. No clean lint or commit is claimed. Next: longer diagnostic lint after browser ownership release, then normal hook-gated checkpoint commit.

Accepted checkpoint committed as `a96adeec38` after an extended affected-package lint run reported zero issues and the normal commit hooks passed on a warm-cache retry. The first hook attempt reported zero issues but timed out; it produced no commit. Both hooks were active; bypass was false. The checkpoint contains the reviewed 12 Go files. Final recovery integration, upstream refresh and push remain in the parent plan.
