# Integration checkpoints

## Terminal identity

Commit `a96adeec38f40a0e0a0621c8ee046847cf9dc731` contains the reviewed 12 Go files over `b2bbe9d5b764223c0dc100b9075a670159e1e9a8`. Era Sol accepted both production semantics and direct automatic-notification regression coverage. Focused race checks passed nine top-level tests plus four notification/control tests. The earlier broad selector passed 84 tests with 22 PostgreSQL skips; SQL guard, store conformance and build passed. See the linked implementation and notification handoffs for exact evidence and limitations.

Extended affected-package lint passed with zero issues (`/tmp/kandev-terminal-final-extended-lint.log`). The first normal hook attempt reached zero issues but exceeded its five-minute limit and created no commit. The warm-cache retry passed normally (`/tmp/kandev-terminal-checkpoint-commit-retry.log`).

Hook receipt: pre-commit active; commit-msg active; harness-lint passed; architecture-lint passed; gofmt passed; go-lint passed; public-copy guard passed; commitlint passed. Documentation/specification and frontend-only hooks skipped where no matching staged files existed. Bypass false; commit result pass. The worktree deliberately retains the separately reviewed duration packet and unfinished cleanup regression/docs.

Duration Go files were temporarily preserved in a path-scoped stash during the terminal-only commit, restored with SHA-256 equality for all four files, and the owned stash removed. No source or evidence was discarded.

## Remaining integration

Commit accepted duration separately, merge reviewed recovery checkpoint `f71b9e09d4`, validate combined seams, then implement the already-planned successful-completion cleanup. Upstream main was fetched at `b2376186ed`; recheck its current tip at final merge. Finish affected validation and verify the remote parent-branch SHA after push. No live runtime/database mutation or deployment has been performed.

## Duration and recovery merge

Duration checkpoint `8ed56d94ef6b9a2d99d9b415f4ed1f638141626b` passed normal pre-commit and commit-msg hooks without bypass, including Go/web lint, formatting, i18n, E2E sleep guard, docs/spec/catalog and public-copy checks. Log: `/tmp/kandev-duration-checkpoint-commit.log`. Both parent checkpoints were pushed to `origin/feature/plan-errands-first-d-424` and `git ls-remote` verified that exact SHA. The unfinished cleanup packet was temporarily stashed for the clean checkpoint push, restored with SHA-256 equality, and the owned stash removed.

Recovery merge from `f71b9e09d4` is prepared, with one source conflict resolved in `apps/backend/internal/errands/compose.go`: preserve both `AttentionReason` and `DurationMS` in the immutable item and its producer. Other changes merged automatically; no unresolved paths remain. The merge is not committed pending combined validation and focused integration review. The intentionally RED cleanup test remains outside the index and is not part of the accepted merge.

Combined acceptance passed and Era Sol `523eba58` found no new integration findings. Parent verified all nine receipt SHA-256 values from [combined acceptance](combined-acceptance-handoff.md) and all seven reviewed source Git blobs before committing. Results: 128 primary and 13 extra top-level backend race passes; 24 PostgreSQL cases/subcases skipped without a DSN and one standalone environment-gated browser fixture skipped; SQL guard and race store conformance passed; web typecheck and 162 Vitest tests passed; fresh managed desktop 3/3 and source-stable phone 3/3 passed. The strict codec overlay checks both optional metadata fields together. These checks exclude the deliberately unimplemented cleanup RED cases and do not certify external providers or live incident repair.

Recovery integration committed as `67b6b9ba9d105d16aacac7fa3c1c297783e129d8` over parent `8ed56d94ef` and merged parent `f71b9e09d4`. Normal merge hooks were active and passed: harness, architecture, gofmt, changed-package Go lint, public-copy and commitlint; other hooks skipped where the merge-conflict file scope had no matches. Bypass false. Receipt: `/tmp/kandev-recovery-integration-merge-commit.log`. Push to the parent feature branch succeeded and remote SHA matched. The unfinished cleanup packet remains uncommitted. A separately authored untracked `loading-performance-investigation.md` appeared during integration; it was preserved and excluded from the commit/push. No owned stash remains.

## Direct primary cleanup checkpoint, 2026-11-12

The primary completed cleanup checkpoint `4becbc47a1f38d869f5e8b90bce44cc019a9bd6b`, parent `67b6b9ba9d105d16aacac7fa3c1c297783e129d8`. Both commit hooks were active and bypass was false. Harness, architecture, gofmt, scoped Go lint, Prettier, web lint, i18n, E2E sleep guard, public copy guard and commitlint passed; documentation-catalog and specification hooks skipped their unselected paths, with standalone catalog/spec checks passed. The unrelated loading-performance note stayed untracked. This is a local checkpoint; final merged browser/race acceptance and delivery remain pending.

Fetched upstream main is `dab0d02e4571c147adf8b9c193db595dbafdd190`. Its 17 conflicts are resolved and the merged backend builds. Final managed acceptance uses one shared build lock, two Go workers and a 3 GiB soft Go memory target. Inlining is disabled only for the large orchestrator test package; a separate optimized production build validates normal optimization. No live instance or database was changed. Final acceptance and push receipts will replace this pending state below.

## Final upstream integration acceptance, 2026-11-12

The upstream merge preserves exact parent terminal repair, immutable historical elapsed time, recovery attention/refusal and explicit continuation, and durable successful runtime cleanup. Integration also corrected cancellation re-entry during terminal convergence, session recovery publication ordering, Send Now claim-write failure fallthrough, and retention of unbound opaque ordinary-session payloads without granting turn authority. Runtime admission fixtures use the existing executor actor helper; prompt/signature fixtures match upstream contracts. No production safety gate was removed.

Backend race acceptance in `/tmp/kandev-upstream-backend-acceptance.log` passed the full Errands, models, status-summary, system-prompt and MCP-profile suites, then the affected orchestrator, SQLite, executor, task-service, agentctl API and lifecycle selectors. The verbose selector recorded 875 top-level passes and 98 top-level skips; the earlier five package suites were not verbose and are not included in that count. Skips include PostgreSQL without a DSN and environment-gated browser/storage cases. SQL guard and race store conformance passed. Scoped lint passed with zero issues across 93 changed Go packages (`/tmp/kandev-upstream-lint.log`); its first cold package-loading attempt timed out, then the extended retry passed.

Web acceptance passed typecheck, 418 tests, i18n checks and the E2E sleep guard (`/tmp/kandev-upstream-web-acceptance.log`). The first real browser run exposed completed-child follow-up rejection introduced by upstream's general terminal-session policy. Frontend and WebSocket admission now permit a completed Errand explicitly, with exact-settlement refusal still authoritative; ordinary completed, failed and cancelled sessions remain blocked. Frontend RED/GREEN evidence is `/tmp/kandev-completed-errand-submit-{red,green}.log` (119 affected tests pass). Backend admission RED evidence is `/tmp/kandev-completed-errand-handler-red.log`. Final backend admission, browser, optimized-build and delivery receipts follow after completion; none is inferred from the earlier browser results.

No live runtime/database repair or deployment was performed. The unrelated untracked `loading-performance-investigation.md` is excluded. Heavy builds use the shared lock and bounded resources; cache cleanup removes only unused scratch or older cache entries between builds.

## User-requested delivery checkpoint

The user requested committing and pushing the implemented work before final verification, then merging the newer upstream main and fixing any integration failures. This checkpoint completes the existing `dab0d02e4571c147adf8b9c193db595dbafdd190` merge and preserves the current source; it does not claim final browser acceptance. The newer fetched upstream snapshot is `9b2b8e98e2b7a510560c65ac572d412fa3394eb0`.

The full handler race suite passed 786 top-level tests with no skips (`/tmp/kandev-completed-errand-handlers-final.log`), using `SHELL=/bin/bash` to avoid this host's zsh auto-tmux startup in the unrelated authentication-terminal test. Frontend typecheck and 175 tests passed (`/tmp/kandev-final-web-integration.log`). Browser admission now preserves the child target, but the follow-up exposed a separate missing cold-resume permission. The latest implementation grants that permission only for a task-bound explicit manual request after exact Errand continuation validation; passive/foreign-task/ordinary/failed/cancelled/unsettled controls are included. Its race run is `/tmp/kandev-completed-errand-cold-resume2.log`; collect its terminal result and rerun browser acceptance after the checkpoint. Earlier failed browser runs remain RED evidence, not final acceptance.

Final upstream integration, full desktop/phone Errand checks, affected native-recovery/storage checks, optimized build and final delivery records remain required. The unrelated loading-performance investigation stays untracked and excluded.

## Checkpoint delivery and latest upstream

Checkpoint `0067adc210e28ee242f507a211c4a7ca6b96d508` was pushed to `origin/feature/plan-errands-first-d-424` and its remote SHA was verified. Its parents are cleanup `4becbc47a1` and upstream `dab0d02e45`. Both normal hooks were active and bypass was false. The first hook attempt reported zero Go lint issues but exceeded five minutes; the warm-cache retry passed all applicable hooks, including commitlint (`/tmp/kandev-errands-before-refresh-commit-retry.log`). The latest completed-authority race selector passed 15 top-level tests (`/tmp/kandev-completed-errand-cold-resume2.log`). Browser follow-up acceptance remains pending.

The subsequent upstream fetch returned `37f081a976526de1a6967f8714e72e247ec0b386`: five additional commits covering repeated native Resume, recovery-block migration and quarantine triggers, terminal keepalive/render lifetime and PR conflict badges. The merge applied cleanly, preserving the Errand fixture freshness entry and both migration sets. Per the user's requested order, this updated branch is checkpointed before final verification. Acceptance of the combined source, any necessary fixes and final delivery records remain open. No live runtime/database changes were made.
