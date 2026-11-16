---
created: 2026-11-16
status: done
requirements:
  - REQ-TASKS-ERRAND-HISTORY-001
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-004
system_design:
  - ../../specs/tasks/system-design/errand-history-loading.md
  - ../../specs/tasks/system-design/errands.md
legacy_specs: []
---

# Task09: evaluate ordinary recognition without child view ownership

## Decision and evidence

The user authorized behavior-preserving quality work through the roadmap and requires ordinary-page performance to remain protected. Final e0 measurements passed 28 ordinary and 84 history functional observations but only 6/10 fresh ordinary numerical limits. Away-navigation paired deltas were positive 7/7 on both surfaces; phone heap was positive 7/7. All original limits and failed observations remain fixed.

Separate diagnostic observations suggest unconditional branch/code footprint contributes to the cost, but neither CPU profiles nor heap ownership established a source hotspot. A source audit found ordinary headers and phone pickers execute the full child view-model hook even though its inspection effects are guarded. This work order evaluates that specific avoidable work; it does not claim the experiment will clear the performance gate or justify a broader lazy-loading rewrite.

Reference source is `e0d351ca512d049c79626c6841621033dfd52f06`. Complete source/build/raw evidence is preserved under `/home/luancm/kandev-task-evidence/errands-20261115/final-matrices-e0d351ca512d/`. Profile and module reports are under `/home/luancm/kandev-task-evidence/errands-phone-diagnostic-e0d351ca/` and `/home/luancm/kandev-task-evidence/errands-vite-metadata-e0d351ca/`. Diagnostic observations are not acceptance samples.

## Ownership and isolation

Primary owns this design, acceptance, integration and all Git mutations. Terra implements only in `/tmp/kandev-recognition-variant.q_umv1yn/kandev`, branch `experiment/errands-recognition-20261116-9e4185`, initially exact e0. Sol reviews the isolated diff and unchanged semantics. The delivered checkout, matching-main control and eager reference stay unchanged. The experiment is not authorized for integration or publication before its evidence is assessed.

The worktree's `.git` symlink points to its original Git-generated worktree directory. Its original marker, identical before/after HEAD/status/common directory and reason are recorded in sibling `setup.json`; no version-control build checks are bypassed. Install the pinned dependencies from `apps/` before tests. Workers are not alone in the codebase and must not revert others' changes.

## Scope and fixed interface

Own only a lightweight recognition module beside `use-errand-view-model.ts`, that existing model, `errand-header.tsx`, `mobile/mobile-session-picker-row.tsx`, and focused tests for those boundaries. Reuse existing scoped compact-row and retained-inspection selectors. The model must consume the same recognition predicate as the wrappers.

Recognition must exactly match the existing `selectErrandViewInputs.isErrand`: a session ID and either full-session Errand role, feature-enabled scoped compact evidence, or scoped retained inspection. Do not substitute `isErrandSession`, use task-wide live counts, or require resolved task identity. Partial role-only state can currently retain detail even while the model returns null; preserve that lifetime and the picker's `model ? null : ordinary picker` fallback.

Ordinary wrappers use a boolean recognition subscription and avoid mounting the full model. Recognized header and recognized phone picker each keep their existing full-model owner, effects and fallbacks. Preserve recognized-to-recognized component identity; add no session key. Cold unknown-child discovery remains with unchanged route admission. Missing evidence already yields empty view inputs and no inspection; introduce no new discovery read.

The phone picker keeps its ordinary fallback in a stable fragment slot and mounts the recognized-only full-model owner in a separate slot. Derive both recognition and fallback visibility from the same pure result: show the picker when recognition is absent or its resolved task ID is absent, matching the existing model-null condition. Use one scalar-mode or shallow-pair subscription. Do not swap ancestor component types, add a callback/state bridge, or reset an open fallback sheet when recognition changes while the model remains null. Verify open/focus continuity explicitly.

Keep feature-off role-only and scoped-inspection recognition; ignore compact-only recognition when disabled as before. Preserve retained parent/scroller identity, missing-parent behavior, summaries/counts, admission/continuation/recovery authority, scope/reset/deletion/eviction and cached-inspection handoff.

Reader/coordinator/store observer, route admission, announcer, retained-reader code, summary/history hook composition, transcript renderers, lazy loading, backend, localization and permanent instrumentation are outside this experiment. No UI copy or intended layout/interaction change. Existing desktop header and native phone picker remain the visual design.

## Proof and stop conditions

Use existing real-store functional oracles and add focused RED/GREEN where new recognition logic requires it. Protect ordinary rename/listener/unmount behavior on both flag values; late role/compact/history/inspection recognition; feature-off fallbacks; ordinary-to-child-to-ordinary transitions; both child owners surviving independently; final-owner eviction; partial role-only state with unresolved task identity. Preserve existing cold direct child/reload, navigation during pending reads, reset/deletion, once-only inspection, Open/Back and retained-parent controls.

A behavior-preserving extraction does not require an artificial failing test merely to prove fewer internal calls. New tests must assert meaningful recognition/lifetime/visible outcomes, not mirror component construction. The intended avoided ordinary work must still be demonstrated separately before attributing a performance result to this change.

Stop if the variant needs new discovery reads, removes a recognized-child owner, changes reset/eviction or historical/feature-off evidence, or requires an excluded production module. Report any unavoidable remount/lifetime difference before changing behavior. Do not weaken tests or add broad mocks to make the variant pass.

After focused tests/typecheck/scoped lint and source review pass, obtain the primary's next diagnostic authorization. Build normally with managed source/fixture freshness and exact identities; keep all experiment outputs separate from frozen e0. First use a controlled same-workload diagnostic to decide whether the avoided work has a worthwhile effect. Do not run a full matrix or claim acceptance from one sample. If the result is unhelpful, retain the experiment and evidence without integrating it. If supported, primary schedules the relevant unchanged functional and numerical gates before integration.

Every heavy command uses `/tmp/kandev-errands-repair-build.lock`; one browser worker and bounded Go resources, pinned Node24.22.3/pnpm9.15.9 and existing warm caches. Retain handles through completion, check disk, and remove only owned inactive scratch. No live runtime/database changes, no source/build-identity bypass, no staging/commit/push by workers.

## Results

The bounded experiment is complete and its production variant is rejected for integration. All ten managed desktop/phone functional controls and 28 unprofiled ordinary observations passed without retries, but the measurements did not establish a worthwhile consistent performance improvement. The delivered runtime implementation remains unchanged from e0.

The initial stage passed 71 focused tests, scoped lint, formatting and typecheck. The real picker continuity test caught a remount introduced by the experimental component split; the stable-slot revision preserved open state, focus and DOM identity. Targeted history-only and partial-role-only controls subsequently passed 14/14. Sol source and coverage reviews found no defect in the final variant. Initial provider/disconnected-store fixture failures and lint findings are retained separately from the behavioral RED. First-stage raw-output gaps are qualified in the external handoff; later raw receipts are retained.

The fresh instrumented phone pair suggested faster switching but no heap improvement. It was not sufficient evidence for integration: the delivered-side timing was unusually slow and profiling changes execution. The primary therefore authorized one unprofiled ordinary cohort with the unchanged seven alternating pairs per platform, one worker, no retries, and all existing timing/cache/GC boundaries.

| Numerical cap family                  | Variant passes | Delivered e0 passes |
| ------------------------------------- | -------------- | ------------------- |
| Each cohort's fresh b4-derived limits | 8/10           | 6/10                |
| Frozen historical 585 limits          | 7/10           | 9/10                |
| Frozen final-delivered e0 b4 limits   | 5/10           | 6/10                |

Fresh limits widened; the 8/10 result is not an improvement claim. Away paired medians improved by 7.121 ms on desktop and 26.778 ms on phone, but away p95 worsened by 13.937 ms and 35.759 ms respectively. Phone return paired cost worsened by 22.065 ms. Phone heap overhead remained positive in all seven pairs, with a paired median of 827,048 bytes, only 4,596 bytes below delivered e0. Independent Sol arithmetic/protocol review supports rejecting the variant. No cap was changed and no performance exception was accepted.

The implementation-independent real-store contract tests were copied unchanged onto the delivered e0 implementation. All 14 tests, scoped formatting/lint and typecheck pass there. They protect late role/summary/history/inspection evidence, independent owners, partial task identity, final eviction and picker continuity without depending on the discarded recognition module. Production files were byte-compared with e0 and are unchanged. Releasing an opener still does not prove reverse fallback continuity while a recognized owner retains inspection; that evidence qualification remains.

### Evidence and limits

The explicit experimental source identity is `recognition-stage2-21aed8a12893176d4569fcc0791fc5c29eeec2bee34ed0bfc06b277cf63bea47`: dirty base e0 plus four production files and one test file. Raw `sourceHead=e0` alone must not be interpreted as delivered source. Exact source, build, input and artifact identities were frozen through the runs and all three-arm before/after comparisons passed.

The durable experiment packet is `/home/luancm/kandev-task-evidence/errands-recognition-task09/unprofiled-ordinary-as8wvfz7/`. `experiment.tar.gz`, SHA-256 `9e392d21f6df10ce0c92b469e719892e3ff4834ad596022393846597e72a48c0`, contains 900 verified members, including the actual 714 variant artifacts. The final receipt is SHA-256 `239325c865cea54c1f024deedbc44e34972399e5cbfa31abe8cf1d85f4644d46`. Controls were reused only after exact comparison with the prior verified archive. All handles exited and the lock was released. No history matrix was repeated for this rejected source.

The separate backport packet is `/home/luancm/kandev-task-evidence/errands-test-contract-backport/`; it preserves the original test, exact diff, hashes, raw 14-test/static receipts and production-equality proof. The frozen prototype remains available in its isolated worktree and archive. Private compiler caches are not evidence and are excluded from the archive.

These are separate seven-pair cohorts, not direct delivered-versus-variant causal proof. The fixture has no execution/environment rows and does not certify real environment-bound restore costs. The residual source audit found no redundant navigation request or restoration defect: ordinary requests still begin in parallel, loaded membership bypasses inspection, and the timer selects the destination session's composer. A deferred restore can materialize twice while authority is unresolved to preserve the original saved layout; that is not evidence of avoidable work.

Global performance acceptance remains open. The selected source, test and documentation work does not authorize silently accepting fixed overhead or adding another speculative refactor. The [package plan](plan.md#current-delivery-checkpoint) owns delivery status; the [quality roadmap](../errands-remaining/code-quality-roadmap.md) records the remaining decision.
