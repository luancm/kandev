---
id: 05-definite-non-delivery
title: Preserve proven non-delivery through Errand admission
status: done
wave: 5
depends_on:
  - 03-owned-await
plan: plan.md
requirements:
  - REQ-PLATFORM-UNDELIVERED-PROMPT-001
  - REQ-PLATFORM-DELIVERY-RETENTION-001
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-004
acceptance_criteria:
  - AC-PLATFORM-UNDELIVERED-PROMPT-001.1
  - AC-PLATFORM-UNDELIVERED-PROMPT-001.3
  - AC-PLATFORM-UNDELIVERED-PROMPT-001.4
  - AC-PLATFORM-DELIVERY-RETENTION-001.3
  - AC-TASKS-ERRANDS-002.7
  - AC-TASKS-ERRANDS-002.8
  - AC-TASKS-ERRANDS-004.9
  - AC-TASKS-ERRANDS-004.10
system_design:
  - ../../specs/platform/system-design/undelivered-prompt-recovery.md
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task05: retain exact non-delivery evidence at Errand admission

Upstream `f52fbebd9b` adds conditional settlement for prompts that were definitely never sent. Its clean merge over delivered `859e2c7fcb` exposes an Errand integration gap: the prepared parent-follow-up path records unknown admission and wraps a typed unsent error before the new settlement seam. The submission can then become failed while the work turn remains open. Initial admission, initial Send Now and restart readers also need to preserve this existing proof instead of inferring either refusal or uncertainty from a generic failure callback or failed row alone.

This is a compatibility repair within the existing approved non-delivery and Errand admission contracts. It does not expand recovery authority or change the performance budget. Parent owns the merge and delivery; one existing native writer owns this packet. No persistent task is created.

## Selected boundary

Reuse upstream's existing local non-delivery receipt: failed state and outcome, a committed settlement timestamp, and no terminal identifiers. Match the captured submission, session, incarnation, positive harness and owner generations, hash and dispatch attempt, plus the existing Errand turn/execution/reservation/claim ownership. Conflicting acceptance, terminal inbox/effect evidence or successor ownership prevents refusal. The receipt proves non-delivery, not provider completion or an Errand terminal decision.

Propagate definite refusal only after the exact conditional settlement succeeds. Existing admission/turn/result/claim owners then perform their usual refused-attempt cleanup. Initial failure handling must carry or resolve that committed result through captured ownership; generic callback failure or `RequestNotSentError` alone is insufficient. Prefer the existing callbacks and durable receipt over a parallel coordinator or new authority mechanism. No new receipt type, schema, journal state, agentctl endpoint or force-clear control is planned.

Retain uncertainty when a write was attempted, a reply was lost, acceptance was observed, storage failed, the conditional update lost ownership, or evidence conflicts. Do not strip an uncertainty guard merely because a nested error has the unsent type. Do not fabricate a provider terminal event. Restart and payload retention must retain the same exact receipt evidence; arbitrary failed rows remain fenced. A distinct later instruction may be admitted after normal restrictions clear; the original instruction is never replayed.

## Ownership and proving checks

The implementation owner may change the affected backend prepared/initial admission, failure propagation, Errand recovery and initial Send Now paths, their exact repository receipt reads and focused tests. Existing runtime/executor callback plumbing may change only where needed to transport this result; coordinate any wider API, schema or production web change. Own this work order's Results and one concise handoff. Preserve the 25 staged upstream paths, prior documentation, raw measurements and all other changes; do not stage, commit or push. You are not alone in the checkout. Parent owns the package manifest and integration decision.

1. Reproduce the follow-up failure through parent-origin `PromptTask` with real SQLite and a controlled typed unsent provider boundary. Assert committed exact refusal, correct turn/reservation cleanup, preserved prior-result and claim semantics, no false unknown block, and later distinct admission without replay. A test calling only ordinary `finishPromptExecutorDispatch` is insufficient.
2. Exercise initial caller/runtime admission and an initial Send Now notification through their actual boundaries. Release only the owned reservation/claim after proof; preserve unrelated pending results and message identity.
3. Cover observed acceptance, attempted-write/lost-reply errors, changed owner/hash/incarnation, failed conditional settlement, cancellation and successor interleavings. These remain uncertain or preserve the newer owner as required; no fabricated result or terminal event.
4. Reopen the repository/service and prune redundant payload through the production retention path. Exact receipt metadata still supports the permitted refusal; arbitrary failed rows and conflicting terminal evidence do not. Run actual SQLite and disposable PostgreSQL cases with explicit counts/skips, SQL guard and store conformance.
5. After source review, run the upstream managed desktop and phone unsent-prompt scenarios and the affected Errand controls. Production web loading paths are unchanged; do not repeat the frozen navigation/heap matrix without a concrete changed boundary. Its open warnings remain open.

Use TDD with retained RED/GREEN output. Serialize heavy work under `/tmp/kandev-errands-repair-build.lock`, using the established bounded Go settings, private caches and pinned pnpm. Retain every process handle; preserve production artifacts before managed E2E rebuilds. Do not touch a live/main database or provider. The disposable PostgreSQL owner must stop it and record release. Report exact changed paths, hashes, commands, real cases/skips and remaining limits.

## Results

Parent accepted Task05 after independent source review, verification of the final 16-path source manifest and 26 command-log hashes, and the completed checks in the [handoff](definite-non-delivery-handoff.md). The implementation and scoped verification are complete. Parent owns the upstream merge commit, publication and separate review history; the existing performance gate remains open.

The integration verifier stopped before broad checks on the frozen no-commit merge. Public parent-origin `PromptTask` with real SQLite reproduced a committed failed receipt that still returned delivery uncertainty. The original RED is retained. After replacing the test's reused active turn with a genuinely new `PromptTask` turn, the exact unpatched merged production also failed the same non-uncertainty assertion (`task05-corrected-red-v2`, exit 1). The three temporarily replaced production files were restored byte-for-byte; restoration manifest SHA-256 is `f1ee2886a9a85940f2edf542e47b105c57404aaac9937d76198afab84b344d35`. The earlier corrected-test compile error and two failed GREEN attempts remain separate receipts.

The public failure exposed the need to use existing reserved-turn ownership for trusted parent-origin Errand instructions: eager publication leaves a completed turn without an Errand terminal decision after refusal. Parent's read-only boundary review approved selecting the existing reserved-turn, session-identity and dispatch-guard paths after completed-session restoration and before turn claim. The corrected new-turn test and reused/referenced-turn control pass. Their assertions retain exact refusal, prior-result preservation, later distinct admission and no original replay; no synthetic terminal decision is created.

Repository RED ran 11 SQLite leaf cases: eight passed, while changed dispatch attempt, contradictory terminal inbox and contradictory terminal effect incorrectly settled. The frozen repair adds those conditional-update fences and one metadata-only exact receipt read without changing lock order or schema. Focused repository race GREEN passed 3 top-level / 47 total entries, with 40 leaf cases (20 SQLite and 20 actual disposable PostgreSQL), zero skips. This includes production payload retention and late contradictory evidence. Read-only review accepted both source hashes with no findings. PostgreSQL 16 used loopback port 55488 and isolated test schemas; stop exited 0, and its recorded PID/start identity, listener and postmaster file were absent afterward. An initial default-socket startup failure is retained separately from the successful private-socket start.

The caller RED then passed both public core cases and nine negative/control leaves, while three positive leaves failed as expected: initial failure still classified the committed receipt as unknown, initial Send Now retained its result claim, and a newly opened repository/service did not resolve the pruned receipt. A separate runtime RED demonstrated that a nested uncertainty guard was ignored by initial unsent settlement. The bounded caller repair now captures the prepared receipt through existing callbacks, uses the existing Send Now failure-result owner, and binds reopened receipt evidence to the Errand turn, execution, incarnation and generation. The runtime settlement also preserves an existing uncertainty guard.

Read-only review accepted the 13-file caller v1 source with no findings. Its GREEN attempt (handle 12609, exit 1) exposed eight useful uncertainty negatives: the reserved-turn rollback ran before the existing uncertainty check and deleted the pending turn. The bounded v2 change adds that same uncertainty guard to the existing rollback condition, without changing any oracle. Focused caller race GREEN v2 passed 14 top-level / 49 total entries, zero skips or failures, against `task05-caller-frozen-v2.json`. The unchanged runtime cases separately passed 3 top-level / 7 total entries in the retained v1 run. Independent delta review accepted v2 with no findings, verified all 13 hashes and reproduced the v1 hash by reversing only the added condition in memory. Normal optimized compilation passed for backendapp, orchestrator, Errands and runtime lifecycle. The required five-minute lint wrapper timed out during package loading; its identical-scope extended run found one test-only staticcheck style issue. The equivalent expected-state `if/else` was changed to a tagged switch, retaining all five cases and assertions. Parent independently accepted this v3 delta, verified all 13 hashes, and reversed its bytes to the accepted v2 hash. All production hashes are unchanged. The affected race rerun passed all five original leaf cases (1 top-level / 6 total entries), zero skips. Final identical-scope extended lint and the unchanged normal five-minute wrapper both passed with zero issues on v3. Web typecheck and four-file E2E lint passed. Managed desktop and phone verification each passed exactly three controls with one worker, strict WebSocket accounting, zero retries and zero skips: ordinary SQL-only prompt Resume, real Errand recovery refusal and explicit interrupted Errand continuation. Phone reused the fresh desktop build with `--no-build`; all 708 recorded artifact hashes remained unchanged. The repository proof does not claim a reopened process or full application startup; the separate caller reopen case closes the old pool and constructs new owners. No main-instance or real-provider operation has run; managed tests use disposable mock-provider backends.

The scoped merged integration race selector passed 130 top-level / 499 total entries with zero skips or failures: orchestrator 42/107, lifecycle 55/221 and SQLite repository 33/171. The agentctl package compiled but the supplied selector matched no tests there; its reconnect scenario ran in lifecycle. SQL guard passed. Store conformance passed 12 top-level / 590 total entries, with 500 leaf cases: 245 SQLite, 245 actual PostgreSQL and 10 engine-independent cases. The final PostgreSQL owner, listener and postmaster file were absent after shutdown. A startup attempt inherited the shared lock; the waiting integration handle 72465 was stopped before any tests ran, the exact PostgreSQL owner was stopped, and the corrected startup used `flock --close` with descriptor-absence and free-lock checks. That setup interruption remains a separate receipt.

Before the managed rebuild, 1,405 existing binary/asset/test-artifact archive members and originals were verified (`task05-pre-e2e-build-artifacts.tar`, SHA-256 `da3b01bf9b14531faa2fde95a9f9c12a7d5203f71b496f07caf4b874cec1d332`). The first managed build exposed the Makefile override of inherited `GOFLAGS`; its exact nine-process group was stopped before browser tests began, all identities were absent and the lock was free. The corrected command pins `MAKEFLAGS=GOFLAGS=-p=1`; its actual Go child environment confirms `-p=1`, two Go scheduler workers and the 3 GiB soft memory target. This is a retained build-setup interruption, not a browser-test failure.

After browser teardown, the two interrupted commands’ temporary Go build directories were matched to retained compiler response files and this checkout’s Task05 sources. The readable process scan found no cwd, descriptor or mapped-file references and no active Go builders. Only `/tmp/go-build2474439669` and `/tmp/go-build2636147331` were removed: 454,027,918 logical file bytes / 459,509,760 allocated bytes; the observed free-space increase was 459,468,800 bytes. Protected-process visibility limits are recorded rather than treated as a complete host scan. Current caches/modules, binaries, browser assets, compiler response files, raw receipts and archives remain retained. The three main process identities and the staged merge index are unchanged.

Final Go formatting passed for all 22 changed files. Catalog validation passed 455 decisions / 2,029 specifications, and all-spec lint passed. The first Markdown formatting check requested only alignment of the new handoff table; that formatting was applied without changing source or assertions. Final source hashes, whitespace/format checks and the no-owned-process/lock release receipt accompany the handoff.

Raw commands, logs, exits and manifests are retained under `/tmp/kandev-upstream-f52-verification.0g9keazg/`. The 25 staged upstream paths and parent-owned documentation changes remain preserved; this writer has not staged, committed or pushed anything. Task05 remains in progress.
