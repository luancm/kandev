# Core recovery delivery checkpoint

The user requested commit and push on 2026-11-10 so they can rebuild and restart Kandev with the completed core fix. Branch: `feature/fix-stranded-errands-mo6`; parent HEAD: `11c2b68f869bc11eab8142764655a0a91415bbc6`. No parent-checkout/index or pending-merge changes are authorized or included.

## Included and deferred

Included: exact interruption evidence before destructive recovery, historical receipts without overwriting newer results, terminal supersession, immutable producer/consumer ownership, shell transcript isolation, typed warning turns, real parent-provider delivery regression, and guarded conversation-receipt turn creation. Prior per-slice race, lint and integration evidence is in the sibling handoffs. Final checks below cover the current combined snapshot and recovered tests.

Deferred: evidence-aware recovery attention, reason propagation and capacity parity, stable refusal presentation, supported explicit-resume eligibility adjustments, translations and desktop/phone acceptance. The requirements/design package records these intended contracts; it is not evidence they shipped. Task 01 remains in progress. PostgreSQL runtime parity and historical incident repair eligibility remain unproven.

## Current runtime incident

The latest implementation Errand `9c484042-a522-48b4-88d3-5a880b776f37` stopped producing retained activity at 07:06:15 UTC. Logs show SIGTERM at 07:07:09.096, a second SIGTERM forcing exit at 07:07:09.207, and startup at 07:07:15.500. Recovery abandoned turn `a90adb6a-a3b8-476f-b976-64c60f2d036c` at 07:07:25.942 and rejected reconciliation for missing exact settlement at 07:07:26.176. This explains the observed recurrence, but does not identify the signal sender or prove every Errand fails. Many earlier Errands completed successfully.

Supported cancellation during this checkpoint returned `errand submission admission remains unresolved`; read-only SQL confirmed the Errand row was unchanged. The session remains waiting, native thread idle, and no worker checks remain running. No replacement writer is launched. The running binary is under `/home/luancm/projects/kandev/apps/backend/bin`, separate from this fix worktree. No deployment or live database repair is performed.

## Final verification

In progress. Record exact terminal results before commit. Normal hooks must run; no bypass is authorized. Public explanation updated in `docs/public/coordination.md` to distinguish interrupted delegation from unknown provider outcomes and avoid promising historical repair.

## Abrupt loss and memory pressure

Systemd's user journal confirms OOM-triggered service restarts at 2026-11-09 21:58:21, 2026-11-10 01:46:22 and 07:07:08 UTC. Agent-launched builds share `kandev.service` with the backend (`OOMPolicy=stop`, `Restart=on-failure`). The kernel victim could not be verified with available journal permissions. During final verification a Go compiler used roughly 6.5 GiB despite GOMEMLIMIT=3GiB; abandoned task-owned tmpfs scratch occupied about 3.7 GiB. This establishes memory pressure as a recurrence trigger, not a thread-specific model failure.

OOM is an abrupt-loss case under the existing recovery contract: no shutdown callback or terminal event is assumed; original-owner death, exact admission evidence, authenticated reconciliation and durable retries remain required. Interruption does not prove external effects or permit replay. Historical records without proof remain blocked. Process-loss tests cover the recovery mechanism; no deliberate OOM of the live service is authorized. Whole-service OOM survival and build cgroup containment are not claimed by this patch.

The final check was lost during another confirmed service OOM at08:35:30UTC (systemd restart counter4); handle77249 became unavailable and no test/compiler remained. Captured passes: SQLite34.179s, lifecycle10.240s, process1.241s, Errands1.510s. Orchestrator/backendapp completion is unverified and must be rerun. Primary rechecked process references and removed only the five abandoned go-build directories inside the task-owned RAM scratch. Subsequent checks use disk-backed `/tmp/kandev-errand-final-checks`.

The provider-boundary regression already kills its owned process with `Process.Kill` before recovery, exercising ungraceful loss without a normal terminal event. It does not induce host OOM or prove whole-service survival. Removing abandoned task scratch reduced tmpfs usage to about45MiB; subsequent checks use disk-backed scratch.

Final combined rerun handle75776: backendapp10.429s, producer1.588s and storeconformance28.029s PASS. Orchestrator failed only `TestErrandLaunchRecoveryAttemptedSuccessorBlocksAdoption`: its fixture used the now-guarded public reservation path to construct legacy invalid state. Primary corrected that test to assert public refusal, seed the legacy state explicitly, and retain the original adoption/successor protection assertions. No production guard was weakened. Orchestrator rerun handle81577 PASS12.062s, including real warning-handler coverage.

Documentation checks passed:436 decisions/1939 specifications, all specification lint; public-doc validator63 tests/55 pages. Dependencies installed from apps with the frozen lockfile for normal commit hooks. The inherited hook path points to an absent macOS directory; command-scoped private worktree hooks invoke the normal pre-commit framework, without changing shared Git configuration or bypassing checks.

Current combined snapshot targeted race checks pass, with the fixture correction above. PostgreSQL remains skipped without a DSN; the previously documented broader Send Now baseline failures remain outside these selections. Normal commit hooks and push are next.
