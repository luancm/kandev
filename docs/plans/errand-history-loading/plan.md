---
created: 2026-11-12
status: done
requirements:
  - REQ-TASKS-ERRAND-HISTORY-001
system_design:
  - ../../specs/tasks/system-design/errand-history-loading.md
legacy_specs: []
---

# Implementation Plan: Errand history loading

## Outcome and current state

Tasks00–04 are complete at production source `ee0f077cb9`, which includes upstream main `585824a5d0`. Ordinary sessions retain full membership and their existing detail-loading behavior. Finished Errand history loads only on demand in bounded pages; inspection loads one child and releases its unowned detail caches after Back. Exact late-tool result delivery and shared-summary publication are also repaired and verified in the merged branch.

The final main comparison passed 28/28 scenarios with unchanged builds, and all eight latency measures stayed within the predeclared variance warning boundaries. The history-growth comparison passed 84/84 scenarios, all structural bounds and 28/30 strict numeric limits. The two remaining strict misses and roughly one-MiB fixed heap cost are explicitly accepted as first-delivery exceptions under the user's direction to prioritize dependable bounded work and assess proportionate fixed overhead. Their original numbers remain unchanged. [Final measurements and disposition](summary-followup-handoff.md) and [Task04](task-04-acceptance.md) state the limits, including the phone first-switch median difference and the lack of universal no-regression proof.

The user authorized direct primary completion, integration, commit and push. [Final integration evidence](../errands-remaining/final-integration-handoff.md) records checks and the source checkpoint; the Kandev task plan records the remote delivery receipt. The [bounded-use roadmap](../errands-remaining/bounded-use-roadmap.md) defines the subsequent real-provider pilot. Earlier design checkpoints and intermediate measurement failures below remain historical evidence.

Task system ownership follows durable child identity and inspection, including desktop/phone outcomes. Scope excludes normal-session lazy-loading conversion, general asset optimization, unrelated settings pages, providers, runtime cleanup redesign and external publication. Existing detail laziness and ordinary session fields/revisions/subscriptions are preserved. No ADR is added because the existing delegation boundary and this focused read design capture the rationale.

Historical paired checkpoint: all 100/1000-history limits and separate no-Errand warm-navigation controls passed, while desktop zero-finished cold start missed by 56.316 ms and phone retained heap by 94,448 bytes (about 92.2 KiB). The zero-finished cohort still contains two outstanding Errand controls. The [second paired handoff](paired-performance-v2-handoff.md) preserves all 84/84 scenarios, 28/30 strict limits, source hashes and per-round evidence; the first matrix is unchanged. Its six CPU captures found generic rendering/GC cost, and both render-count controls recorded 117 commits without isolating another defect. The [final disposition](summary-followup-handoff.md#first-delivery-disposition) explains acceptance without changing thresholds or relabelling failed observations.

Completed remediation: parent serialization now computes existence/outstanding booleans through one scalar SQL read instead of projecting every finished child. Verification passed 24 targeted race tests, 11 SQLite/PostgreSQL persistence-conformance tests, SQL guard and scoped Go lint. The rebuilt 1000-history smoke passed; full candidate child session/transcript/turn caches and rich subscriptions return to zero after Back. [The correction handoff](parent-state-performance-handoff.md) preserves RED/GREEN, the initial fixture/lint issues and final receipts. After the paired matrix, 5 GiB of old owned build-cache entries were removed under the shared lock; that checkpoint left about 18 GiB free. Raw evidence and current build artifacts are preserved.

## Evidence retention (2026-11-15)

Historical performance evidence has a private host-local archive at `/home/luancm/kandev-task-evidence/errands-20261115/historical-performance.tar.gz`: 802 files, 74,109,722 compressed bytes, SHA-256 `0f7c0ad181678b5e1412a7a5bafecf891966429b877c719f85f80c301dc49884`. Its adjacent `historical-performance-manifest.json` has SHA-256 `4c9c005d326ab5da702bd43f56f695f604e9d4fe734230b0c8ca05402e7ed811`; every archived member was hash-verified against that manifest. Original `/tmp` sources remain untouched. This is private local retention, not a published or repository artifact; checkout access alone does not provide the archive.

The earlier browser logs `kandev-run.e2e.N2xI620R.log`, `kandev-run.e2e.gWl0TF9A.log`, `kandev-run.e2e.4TqM2PR4.log` and `kandev-run.e2e.rDxT447u.log` were already absent from `/tmp` at archival. They were not recovered. Their historical reports and recorded hashes remain provenance, not newly verified raw receipts. All nine result JSONs, both measurement protocols, other distinct handoffs, failed observations and seven-sample limits remain in place.

## Sequence and integration

Task00 is accepted after the bounded Era Sol review and parent inspection of the corrected helper (`ef7d21f3ea03b0dfadb60c24e40c7cb764cca455e32643ac42fc35054a870f64`) and managed desktop/phone 1000-history smoke receipts (1/1 each). The [baseline handoff](baseline-handoff.md) and [frozen caps](baseline-results.json) retain original successful-run measurements and the separate unexplained phone timeout. No supplemental heap cohort was needed because baseline application state, child loops and GC endpoint are unchanged. Task01/02 evidence follows below.

Task01 is accepted. Era Sol reviews resolved snapshot coherence, unread parity, production query-plan and upgrade evidence, and offset normalization. Parent closed the last single-digit-hour finding after inspecting the shared normalization/padding, v3 predecessor replacement and parser-differential cases, matching final source and GREEN receipt hashes. The [handoff](task-01-handoff.md) records prior 234-test race and 11-test conformance passes and subsequent focused SQLite/PostgreSQL timestamp, plan and upgrade checks. Earlier broad checks were not repeated for the final SQL-only correction. This is additive read-contract acceptance, not UI/performance acceptance.

Task02 is accepted after Era Sol review and parent closure of the final same-owner preset race. Child Open and delayed saved-layout publication retain navigation/selection/environment/API/attempt/lifetime ownership; saved/customized, built-in and Reset Layout selections now share invalidation. Parent matched the final selector source and 14/14 GREEN receipt. The [Task02 handoff](task-02-handoff.md) records earlier four-package Go race, 302 Vitest, desktop/phone controls, remediation RED/GREEN and persistence checks. The original implementation's missing behavioral RED remains recorded; nonverbose persistence logs independently establish package passes, while DSN/no-skip details remain worker attestations. No candidate performance claim is made.

Task03 is accepted. The parent recovered the interrupted writer and reviewer outputs, then corrected unbounded finished-row bookkeeping, authorization-scope cache ownership and stale child inspection, including session-only events. [Recovered findings](task-03-recovered-review.md) and [parent remediation](task-03-parent-remediation.md) preserve the reported evidence, 274 combined tests, two inspection regressions and rebuilt desktop/phone passes. The separate [exact-tool replay repair](../errand-tool-terminal-replay/plan.md) now resolves the diagnosed delivery failure when retained source evidence is exact. Task04's [final disposition](summary-followup-handoff.md), [delivery evidence](../errands-remaining/final-integration-handoff.md) and [bounded-use roadmap](../errands-remaining/bounded-use-roadmap.md) own subsequent acceptance, delivery and live-pilot prerequisites.

Disk reached 98% used (3.2 GiB free), with the shared Go cache at 33 GiB. Parent verified no active compiler/tests, acquired the shared build lock and removed 16.10 GiB of oldest regenerable Go cache entries, preserving newer entries and all baseline/raw test evidence. That Task03 recovery checkpoint left 26 GiB free. Future heavy work must monitor cache growth and reuse the retained cache.

| Order | Work order                                                  | Independently testable result                                                                               |
| ----- | ----------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| 00    | [Baseline](task-00-baseline.md)                             | reproducible pre-change samples and numeric tolerance gate                                                  |
| 01    | [Bounded browser reads](task-01-browser-reads.md)           | additive authorized summary/page/inspect contracts; old complete lists intact                               |
| 02    | [Ordinary loading isolation](task-02-ordinary-isolation.md) | ordinary boot/list unchanged in content; targeted child routing remains usable while excluding child panels |
| 03    | [Demand history and inspection](task-03-demand-ui.md)       | typed bounded browser cache, explicit history and targeted child reads on both surfaces                     |
| 04    | [Measured acceptance](task-04-acceptance.md)                | focused browser and performance evidence for every criterion                                                |

Dependencies are strictly sequential; no simultaneous shared-contract writers. Backend DTO/action/client types are owned together by task01; task02 wires ordinary boot/list, targeted child identity/routing and restoration atomically; task03 switches browser Errands consumers. Additive endpoints can land before their consumer; no intermediate step silently narrows legacy lists. The user subsequently authorized implementation and parent-owned integration, commit and push. Live database repair or deployment is outside these completed work orders.

Compatibility matrix: legacy browser and MCP complete lists remain complete; new browser uses new typed actions; internal delivery/cancel/recovery use complete repositories; flag-off new reads retain authorized inspection and normal tabs still exclude children; SQLite and PostgreSQL use shared repository SQL without provider changes. Unsupported new action is an explicit error, never unlimited compatibility fallback.

## Performance gate and safety

Task00 blocks all production implementation until a committed-to-the-work-order measurement protocol and numeric tolerances exist. Use the same disposable task with fixed ordinary siblings and zero/100/1000 finished children, separate cold reload and warm switching, fixed build/runtime/browser/network/cache, at least seven repetitions per condition, raw samples and median/p95/range. Measure zero-Errand controls and define numeric regression budgets from observed variance, then freeze them before production changes. Task04 repeats identical conditions and reports actual values against budgets. Structural row/request/cache limits remain deterministic even if timing noise is high; structural bounds do not substitute for measured timing acceptance.

Future heavy commands must acquire `/tmp/kandev-errands-repair-build.lock`, use `GOMAXPROCS=2`, one browser worker/shard and sequential desktop/phone runs. Use disposable fixtures, never the main instance. Check free disk before builds; remove only package-owned disposable captures/build outputs after recording evidence. Preserve other sessions' artifacts and handles. Design validators below are light and need no build lock.

## ASCII UI preview

UI-01: Desktop parent Errands popover, finished history explicitly opened.

```text
Errands                    1 running
Parent conversation
  Running audit          [Open] [Stop]
  Done: result pending          [Open]
v Finished (1000)
  Loading... / No finished errands
  Could not load                [Retry]
  Review checklist              [Open]
  Showing 1-25 of 1000     [Load more]
                         [Stop all]
```

UI-02: Phone Errands pill -> inset drawer -> full-screen child.

```text
+-----------------------------+
| Errands                 [X] | fixed header
| 1 running, 1 result pending |
| Audit         [Open] [Stop] |
| v Finished (1000)           | one internal scroller
| Review checklist     [Open] |
|                     [More]  |
| [Stop all]                  | fixed safe-area footer
+-----------------------------+
          Open ->
+-----------------------------+
| [Back] Review checklist     |
| Child transcript            | full-screen reader
| ...                         |
+-----------------------------+
```

Loading, empty and error are alternative history-body states; retry retains live actions. At the 100-summary cache bound show the retained range and Start again; at end show an end marker. A changed chain offers Refresh rather than silently appending stale pages. Labels are illustrative localized copy, spacing is not a pixel requirement. Required structure maps to AC-TASKS-ERRAND-HISTORY-001.3/.4/.7; task04 proves desktop geometry, phone scroller/44px controls, focus, safe areas and Back anchor restoration.

## Validation and traceability

Work orders retain criterion references, TDD and verification evidence. Task04 owns public coordination documentation, rendered checks and paired performance runs. Requirements are active and the design is current; implementation status and the explicit performance disposition are recorded in the completed work orders. Historical proposed paths and design-checkpoint commands below remain provenance for the accepted package.

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/tasks docs/plans/errand-history-loading
```

PR-documentation coverage preflight uses `.github/scripts/pr-docs.cjs` exported `validateCoverage` with actual new work-order/spec contents (including untracked paths), as recorded in the design handoff. Do not rely on a HEAD-only diff to discover new files.

## Historical design risks and handoff

SQL parity for unresolved recovery and unread state must be demonstrated before optimizing; do not introduce a full materialization fallback. Outstanding queues can be large and are not bounded by runtime capacity. History is a live keyset listing with explicit invalidation/restart, not a snapshot. Numeric timing/memory thresholds are pending task00 measurement, so no performance gain or implementation acceptance is claimed. The review checkpoint is these uncommitted artifacts; later explicit implementation authorization is required.

## Design checkpoint results

On 2026-11-12 at `6e4c5ca9bc`: catalog validation passed (453 decisions, 2014 specifications); specification lint passed; specification-linter tests passed (36); scoped diff whitespace check passed. Actual new documentation paths were passed to the exported PR-documentation preflight and reported exempt. A separate contract-graph preflight included the proposed `apps/web/hooks/domains/session/use-task-errands.ts` trigger only to exercise reference validation: covered, five work orders, no errors. That synthetic trigger is not an edited production file. No build, browser run, production edit or performance measurement was performed. All new package files remain uncommitted; parent-owned investigation/plan edits are untouched.

Reproduce the coverage checks from repository root:

```bash
node <<'JS'
const fs = require('fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/errand-history-loading';
const paths = fs.readdirSync(dir).map(p => `${dir}/${p}`).concat([
  'docs/specs/tasks/requirements/errand-history-loading.md',
  'docs/specs/tasks/system-design/errand-history-loading.md',
]);
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
for (const changedFiles of [paths, paths.concat(['apps/web/hooks/domains/session/use-task-errands.ts'])]) {
  const result = validateCoverage({ changedFiles, fileContents });
  console.log(JSON.stringify({ status: result.status, errors: result.errors }));
  if (!result.ok) process.exitCode = 1;
}
JS
```

## Historical design compatibility integration

The independent read-only compatibility investigation is incorporated. Task02 owns targeted child routing before ordinary-list isolation, retains complete task-removal inventory, and protects ordinary membership/revision/subscription behavior. Task04 names existing normal-session browser controls, including zero-Errand and mixed-task variants. Task00 now names proposed measurement specs and exact baseline commands; task04 reruns the same harness with candidate-only evidence labels. The preview counts one running row separately from a completed pending result and retains individual Stop. These are design corrections, not executed product checks.

After these edits, catalog validation, specification lint, scoped whitespace and documentation contract-graph checks passed. All existing referenced test paths were checked; explicitly proposed history and measurement specs remain future implementation files. No production edits or measurements occurred. The package is ready for the required design-review checkpoint, with numeric performance budgets deliberately gated on task00 evidence before production changes.

The user authorized the fresh paired comparison on 2026-11-14. The [paired protocol](paired-performance-protocol.md) fixes interleaving, samples, unchanged threshold formula and structural gates before collection. Earlier references to a pending user decision are historical; the final Task04 disposition is linked above.
