---
id: "12-reserved-launch-authority"
title: "Persist and classify reserved launch authority"
status: done
wave: 12
depends_on:
  - "06-executor-role-guard"
  - "09-teardown-intent-storage"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-001
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-004
  - REQ-TASKS-ERRANDS-008
acceptance_criteria:
  - AC-TASKS-ERRANDS-001.1
  - AC-TASKS-ERRANDS-001.2
  - AC-TASKS-ERRANDS-002.1
  - AC-TASKS-ERRANDS-004.8
  - AC-TASKS-ERRANDS-008.2
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 12: Persist and classify reserved launch authority

## Scope and authority

Repairs review finding(s) 7 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. Implementation receipts and the Task13 handoff are recorded in Results.

## Low-level implementation and regression plan

**Owned:** new `internal/task/models/errand_launch_authority.go` and test; `internal/orchestrator/executor/errand_session_context.go:applyErrandSessionIdentity` and `executor_execute.go` session creation metadata; new `internal/orchestrator/errand_launch_recovery.go` pure classifier and corresponding test. This foundation must not start or resume a runtime yet.

**Private metadata (no new SQL column):** At initial errand session insertion, copy the complete trusted reserved identity from context into a versioned `errand_launch_authority` metadata record containing task/session/parent, reserved first turn/submission and initial profile. Write it atomically with role/parent and workspace binding, before runtime allocation. Reject partial/mismatched context. It is immutable initial provenance, not a mutable current-turn pointer. Don't put role/parent authority only in metadata; existing indexed columns stay canonical. Full metadata overlays must preserve it. Do not stamp an old unknown session from a fresh request.

**Classifier inputs:** Durable errand reservation; session role/task/parent/queue-incarnation; stored launch authority; exact reserved turn loaded by ID and its session/task; normalized reserved submission (`normalizeDeliverySubmissionID`) and its session/incarnation, dispatch state and terminal turn; runtime inventory/execution + current prompt/submission evidence from the already restored runtime. Use read-only metadata getters (`GetAgentDeliverySubmissionMetadata` when available) so large retained payloads are not loaded unnecessarily. InitialPromptSubmission metadata alone is insufficient: it does not carry the reserved Errands submission ID. SQL submission metadata alone may lack a first-turn link until terminal; require launch authority + exact turn plus current/terminal evidence as applicable.

| Evidence | Classifier result | Allowed next operation in Task 13 |
| --- | --- | --- |
| No session, no turn/submission/runtime under reservation; owner live | `absent` | Normal prepare/create once under guard |
| Exact session/authority; reserved turn absent or provably unattempted; confirmed no runtime/submission effect | `prepared` | Continue same session, create reserved turn only if absent; never PrepareSession again |
| Exact tuple + matching runtime or accepted/dispatching/uncertain durable submission | `admitted` | Adopt bookkeeping; never LaunchAgent/StartAgentProcess/Prompt again |
| Exact tuple + completed/failed/interrupted terminal proof | `settled` | Adopt bookkeeping, then exact result/abandonment reconciliation |
| Any mismatch, missing required authority, unreadable store/inventory, conflicting successor/unproved no-runtime | `blocked` | Existing recovery block/log; no create, no prompt, no false failure result, no unverified stop |

A submission found for another incarnation is not proof for this row. A live runtime found for a different turn is not proof that the reservation was never admitted; it must not be stopped or replayed. Legacy sessions without new authority may be adopted only if existing durable evidence independently proves the same complete session+reserved turn+submission tuple and owner. Otherwise block, retaining the original evidence; do not claim every legacy queued row can be auto-recovered.

**Tests:** `TestErrandLaunchAuthorityPersistsWithPreparedSession` exercises actual executor preparation and SQLite reread/reopen; new classifier table `TestErrandLaunchRecoveryRequiresCompleteAuthority`: absent, prepared, accepted, uncertain, completed, wrong task/parent/role/incarnation/turn/submission, store read error, existing runtime with unreadable receipt, and legacy exact/missing evidence. Positive same-session-ID-only fixture must be rejected. Seed each competing source with distinct sentinels to prove the selected evidence.

```bash
(cd apps/backend && go test -race -trimpath ./internal/task/models -run 'ErrandLaunchAuthority|ReservedLaunchIdentity' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/orchestrator/executor -run 'PrepareSession.*Errand|ErrandSessionContext' -count=1)
(cd apps/backend && go test -race -trimpath ./internal/orchestrator -run 'ErrandLaunchAuthority|ErrandLaunchRecoveryRequiresCompleteAuthority|LaunchErrandSessionCreates' -count=1)
bash scripts/lint-go-changed apps/backend/internal/task/models/errand_launch_authority.go apps/backend/internal/orchestrator/executor/errand_session_context.go apps/backend/internal/orchestrator/executor/executor_execute.go apps/backend/internal/orchestrator/errand_launch_recovery.go
git diff --check
```

**ACs:** `001.1`, `001.2`, `002.1`, `004.8`, `008.2`. Private metadata/read seams only. Authority expansion and pure classifier independently testable; dependency on exact runtime/submission evidence must not be replaced with `session != nil`.

## Dependencies and ownership

[Task 06](task-06-executor-role-guard.md), [Task 09](task-09-teardown-intent-storage.md)

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

`sequential`. The integrating session may overlap disjoint read-only preparation, but grants one writer for each shared schema, repository, orchestration or browser file.

## Results

Accepted 2026-11-01. Immutable version-1 `errand_launch_authority` now records task/session/parent, reserved first turn/submission and initial profile at the actual initial session insert, with canonical indexed role/parent and workspace binding, before runtime allocation. Preparation rejects missing/partial/mismatched reserved context, discards untrusted task-level launch metadata, preserves the new map through preparation overlays, and cannot stamp an existing unknown session from a new request. The compatibility reserved-identity getter and external launch APIs remain unchanged.

The pure `classifyErrandLaunchRecovery(errandLaunchRecoveryEvidence) (errandLaunchRecoveryState, error)` returns `absent`, `prepared`, `admitted`, `settled`, or `blocked`. It validates every supplied source and never starts, resumes, prompts, stops, writes bookkeeping or mutates its input. The 92-case table covers modern and independently proved legacy admission, all supported active/terminal submission states, uncertainty, missing/unreadable evidence, wrong task/parent/role/incarnation/turn/submission, same-session-ID-only claims, and runtime/durable successor conflicts. A matching runtime cannot override a different durable terminal turn or incarnation.

Before-state inventory: `/tmp/errands-task12-before-status.z`, `/tmp/errands-task12-before-hashes.json`; original executor snapshots: `/tmp/errands-task12-before-{errand_session_context,executor_execute}.go`. Progress checkpoint: `/tmp/errands-task12-progress.md`. Final file hashes and preservation check: `/tmp/errands-task12-final-receipt.json`. Only the seven owned Go files and this work order changed during the packet; prior repairs were preserved. Changes remain uncommitted.

Behavioral RED was observed before minimal GREEN: actual executor preparation/SQLite reread lacked launch metadata and accepted six incomplete contexts (`/tmp/errands-task12-red-preparation.log`); model scaffolding failed immutable round-trip and strict malformed-context assertions (`/tmp/errands-task12-red-models.log`); the blocked-only classifier scaffold failed the expected positive dispositions (`/tmp/errands-task12-red-classifier.log`). The first classifier fixture run used the wrong session-model field name and failed compilation (`/tmp/errands-task12-classifier-fixture-compile.log`); that run is not counted as RED. Initial authority GREEN is retained at `/tmp/errands-task12-green-authority.log`.

All Go commands below ran with explicit shared `GOCACHE=/home/luancm/.cache/go-build`, `GOMODCACHE=/home/luancm/go/pkg/mod`, `GOFLAGS=-p=1`, and the exclusive `/tmp/kandev-errands-repair-build.lock` for the whole command lifetime. Build/test/export warming used `GOMAXPROCS=2 GOMEMLIMIT=8GiB`; lint analysis used `GOMAXPROCS=1 GOMEMLIMIT=3GiB` and `--concurrency=1`. RAM and disk were checked before commands. Go commands ran from `apps/backend`; the lint helper and whitespace checks ran from the repository root.

| Final command | Observed result | Evidence |
| --- | --- | --- |
| `go test -race -trimpath -p 1 -parallel 1 ./internal/task/models -run 'ErrandLaunchAuthority\|ReservedLaunchIdentity' -count=1 -v` | PASS: 5 top-level tests, 20 subcases, no skips | `/tmp/errands-task12-final-models.log` |
| `go test -race -trimpath -p 1 -parallel 1 ./internal/orchestrator/executor -run 'PrepareSession.*Errand\|ErrandSessionContext\|ErrandLaunchAuthorityPersistsWithPreparedSession\|PrepareSessionUsesReservedLaunchIdentity' -count=1 -v` | PASS: 10 top-level tests, 12 subcases, no skips | `/tmp/errands-task12-final-executor.log` |
| `go test -race -trimpath -p 1 -parallel 1 ./internal/orchestrator -run 'ErrandLaunchAuthority\|ErrandLaunchRecoveryRequiresCompleteAuthority\|LaunchErrandSessionCreates' -count=1 -v` | PASS: 3 top-level tests, 92 subcases, no skips | `/tmp/errands-task12-final-orchestrator.log` |
| `go build -trimpath -p 1 ./internal/orchestrator/executor ./internal/orchestrator ./internal/backendapp` | PASS: changed callers compile | `/tmp/errands-task12-final-caller-build.log` |
| `go list -p 1 -export -deps -test -tags 'fts5 e2e' -buildvcs=false -pgo=off ./internal/task/models ./internal/orchestrator/executor ./internal/orchestrator` | PASS: one tag-specific export warm; no tests executed by this command | `/tmp/errands-task12-lint-export-warm.log` |
| `bash scripts/lint-go-changed apps/backend/internal/task/models/errand_launch_authority.go apps/backend/internal/task/models/errand_launch_authority_test.go apps/backend/internal/orchestrator/executor/errand_session_context.go apps/backend/internal/orchestrator/executor/executor_execute.go apps/backend/internal/orchestrator/executor/errand_launch_authority_test.go apps/backend/internal/orchestrator/errand_launch_recovery.go apps/backend/internal/orchestrator/errand_launch_recovery_test.go` | PASS: 0 issues, concurrency 1 | `/tmp/errands-task12-final-lint-retry.log` |
| `gofmt -l <the seven owned Go files>`; `git diff --check`; owned-file trailing-whitespace/newline and before-inventory comparison | PASS: formatting and whitespace clean; no unowned changes | `/tmp/errands-task12-final-format-whitespace.log`, `/tmp/errands-task12-final-receipt.json` |

The executor selector explicitly includes the work-order-named persistence/reopen regression in addition to the prescribed preparation/context selectors. Its real SQLite cases verify the insert boundary using a trigger, insertion rollback, keyed and full-map metadata preservation after file reopen, malformed-context rejection, preservation of a legacy row on duplicate preparation, and ordinary/full-reservation compatibility controls. The real orchestrator creation control preserves the existing external API. Full unchanged Task03/04/10/11 suites were not repeated.

The first scoped lint attempt timed out after five minutes in package loading (`/tmp/errands-task12-final-lint.log`, exit 4), without code findings. The normal caller build had not warmed lint's non-trimpath `fts5 e2e` exports. One serialized 8GiB export warm completed, then the unchanged source passed 3GiB lint analysis. The timeout is infrastructure evidence, not a passing check. No cache cleanup or repeated failed low-memory warming was performed.

Task13 handoff:

- Collect fresh evidence under the existing launch/admission/cancellation guard before `PrepareSession`: durable reservation, live task and canonical parent, canonical child session and queue incarnation, exact reserved turn loaded by ID, and independently read current/latest turn. `SessionAbsent`, `ReservedTurnAbsent`, `CurrentTurnAbsent`, `SubmissionAbsent`, and `RuntimeAbsent` are positive completed-read witnesses. Nil alone is unknown; both present and absent is contradictory. Feed any failed read through `ReadError`; the classifier preserves it with `%w`.
- Fetch only the reserved normalized submission with `GetAgentDeliverySubmissionMetadata(ctx, normalizeDeliverySubmissionID(row.SubmissionID))`; the classifier consumes state/session/incarnation/terminal-turn metadata and ignores payloads. `errandLaunchRuntimeEvidence` requires the already restored execution's nonempty execution ID plus exact task/session/incarnation/turn and normalized submission receipt. `RuntimeAbsent` must include authoritative persisted inventory and pending authenticated restore, not only a missing process-local session lookup.
- Modern provenance must equal the complete immutable reservation, including initial profile. Current session profile selection is not substituted for that history. Legacy sessions require canonical ownership, the exact reserved turn with matching initial `ExecutionProfileID`, and independent full runtime proof, exact terminal submission/turn proof, or a durable admitted submission bound to the exact parent/submission by the existing accepted/unknown turn-admission marker. Missing or malformed authority never falls back to session-ID-only adoption, and no recovery request stamps old sessions.
- `absent` requires confirmed absence of all effects plus live owners. `prepared` requires exact stored provenance, a CREATED session without allocation/native-conversation evidence, confirmed no runtime, absent or unattempted pending reserved turn, and no admitted/attempted submission or initial-prompt evidence. `admitted` includes accepted/dispatching/interrupted-unknown exact submissions or full matching runtime receipts and forbids resubmission. `settled` requires an exact terminal submission link plus the reserved turn's completion. Conflicts and missing proof return `blocked` with an error, including a live runtime for another turn. Task13 owns same-session continuation, guarded bookkeeping adoption and all physical side effects.

No new SQL column, schema, SQL query or dialect-dependent production path was added; PostgreSQL execution was not required or claimed. SQLite preparation and mocked runtime evidence establish this packet's boundary only; live-provider recovery/adoption is not claimed. Task10/11 teardown and ownership gates remain unchanged. No delegation, shared-manifest/public-doc edit, commit, push, deployment or live-instance operation occurred. All verification handles were consumed and the shared build lock was released.
