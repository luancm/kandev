---
created: 2026-11-12
status: in_progress
requirements:
  - REQ-TASKS-ERRAND-HISTORY-001
system_design:
  - ../../specs/tasks/system-design/errand-history-loading.md
legacy_specs: []
---

# Implementation Plan: Errand history loading

## Outcome and current state

Implementation is preserved in checkpoint `dcc3de55a2`. Upstream main `037c376196` is merged with conflicts resolved in the working tree; affected backend/frontend checks and all 58 changed-package lint checks pass. Shared-summary publication now happens once per coalesced request, with focused regression and rebuilt desktop/phone evidence in [the handoff](summary-publication-handoff.md). Final comparison against matching upstream main and Task04 acceptance remain open.

Additional Task04 requirement: compare pure ordinary-task loading against matched upstream main and latest main, with candidate Errands off/on. The [comparison protocol](main-comparison-protocol.md) distinguishes whole-feature overhead from the prior eager-Errands baseline. The [comparison completed 56/56 scenarios](main-comparison-handoff.md) with stable artifacts. It found fixed ordinary-page heap overhead and navigation-tail warnings. Task04 remains open; existing numeric gates remain unchanged.

Deliver demand-only finished history while preserving ordinary task-page loading. [Requirements](../../specs/tasks/requirements/errand-history-loading.md) and [design](../../specs/tasks/system-design/errand-history-loading.md) own the contract. Source base requested: `6e4c5ca9bc`. Existing Errands ADR remains authority. The user explicitly authorized executing the work orders on 2026-11-13, retaining the Errands implementation/review workflow. Tasks00–03 are accepted and done; Task04 measured acceptance is in progress. Numeric baseline caps are frozen before production implementation. Earlier design-checkpoint statements below are historical. Parent-owned [remaining plan](../errands-remaining/plan.md) and its [investigation](../errands-remaining/loading-performance-investigation.md) retain their independent state.

Task system ownership follows durable child identity and inspection, including desktop/phone outcomes. Scope excludes normal-session lazy-loading conversion, general asset optimization, unrelated settings pages, providers, runtime cleanup redesign and external publication. Existing detail laziness and ordinary session fields/revisions/subscriptions are preserved. No ADR is added because the existing delegation boundary and this focused read design capture the rationale.

Earlier paired-performance checkpoint: Tasks00–03 remain accepted. The second full paired cohort completed 84/84 scenarios with stable production source and all structural bounds passing. Twenty-eight of thirty numeric limits pass. All 100/1000-history limits and all separate no-Errand warm-navigation controls pass; two low-history fixed-cost limits remain: desktop cold start exceeds its limit by 56.316 ms, and phone retained heap exceeds its limit by 94,448 bytes (about 92.2 KiB). The zero-finished cohort still contains two outstanding Errand controls. [The second paired handoff](paired-performance-v2-handoff.md) contains all limits, medians, per-round evidence and source hashes; the first matrix remains unchanged. The strict measurement failures remain recorded; the current integration state is summarized above. Six CPU captures found generic rendering/GC cost; a separate render-count check recorded 117 commits in each build without isolating another defect. The user subsequently agreed that a proportionate fixed feature cost can be reasonable and requested completion plus a bounded-use roadmap. No numerical allowance is silently applied: original measurements remain unchanged, and any budget disposition must state its rationale and limits explicitly. The shared-summary publication regression has since been fixed and verified; its final timing follow-up remains pending.

Completed remediation: parent serialization now computes existence/outstanding booleans through one scalar SQL read instead of projecting every finished child. Verification passed 24 targeted race tests, 11 SQLite/PostgreSQL persistence-conformance tests, SQL guard and scoped Go lint. The rebuilt 1000-history smoke passed; full candidate child session/transcript/turn caches and rich subscriptions return to zero after Back. [The correction handoff](parent-state-performance-handoff.md) preserves RED/GREEN, the initial fixture/lint issues and final receipts. After the paired matrix, 5 GiB of old owned build-cache entries were removed under the shared lock; that checkpoint left about 18 GiB free. Raw evidence and current build artifacts are preserved.

## Sequence and integration

Task00 is accepted after the bounded Era Sol review and parent inspection of the corrected helper (`ef7d21f3ea03b0dfadb60c24e40c7cb764cca455e32643ac42fc35054a870f64`) and managed desktop/phone 1000-history smoke receipts (1/1 each). The [baseline handoff](baseline-handoff.md) and [frozen caps](baseline-results.json) retain original successful-run measurements and the separate unexplained phone timeout. No supplemental heap cohort was needed because baseline application state, child loops and GC endpoint are unchanged. Task01/02 own the remaining database materialization/enrichment, deterministic reconciliation and controlled identity-race proof; candidate performance remains unproven.

Task01 is accepted. Era Sol reviews resolved snapshot coherence, unread parity, production query-plan and upgrade evidence, and offset normalization. Parent closed the last single-digit-hour finding after inspecting the shared normalization/padding, v3 predecessor replacement and parser-differential cases, matching final source and GREEN receipt hashes. The [handoff](task-01-handoff.md) records prior 234-test race and 11-test conformance passes and subsequent focused SQLite/PostgreSQL timestamp, plan and upgrade checks. Earlier broad checks were not repeated for the final SQL-only correction. This is additive read-contract acceptance, not UI/performance acceptance.

Task02 is accepted after Era Sol review and parent closure of the final same-owner preset race. Child Open and delayed saved-layout publication retain navigation/selection/environment/API/attempt/lifetime ownership; saved/customized, built-in and Reset Layout selections now share invalidation. Parent matched the final selector source and 14/14 GREEN receipt. The [Task02 handoff](task-02-handoff.md) records earlier four-package Go race, 302 Vitest, desktop/phone controls, remediation RED/GREEN and persistence checks. The original implementation's missing behavioral RED remains recorded; nonverbose persistence logs independently establish package passes, while DSN/no-skip details remain worker attestations. No candidate performance claim is made.

Task03 writer `befa04c5` reached a final message at 2026-11-14 00:05 UTC, but its Errand result was not delivered and status is `needs_attention/recovery_unresolved`. Parent recovered the final through the conversation API and confirmed session `WAITING_FOR_INPUT`, saved code/Results/screenshots, no active checks and a free build lock. It reports 176 tests, static/i18n checks, managed desktop/phone 1/1 and a desktop screenshot recapture; these remain reported evidence pending acceptance. No writer was resumed. Era Sol `bc22088f-2849-49e5-bb8c-3ba4c7b89496` also completed its review but failed result delivery. Parent recovered its final directly from SQLite: [three blocking findings](task-03-recovered-review.md) cover unbounded finished operational rows/revision bookkeeping, stale authorization-scope history, and stale selected-child inspection. The parent subsequently remediated all three findings and the session-only inspection case; [the final evidence](task-03-parent-remediation.md) records 274 combined tests, two inspection tests, static checks and rebuilt desktop/phone 1/1 passes. Task03 is accepted; no implementation writer is active. Parent diagnosed a missing-submission late tool update blocking the successful terminal in the durable journal; [incident evidence](../errands-remaining/verification-result-delivery-incident.md) records the exact sequence and remaining reproduction work. Further Errand launches are held; the separate [exact-tool replay repair package](../errand-tool-terminal-replay/plan.md) is now implemented and completing its final verification in the primary session. Task04 retains the full fixed-protocol performance and compatibility matrix. Parent owns remediation, integration and delivery; no new commit or push.

Disk reached 98% used (3.2 GiB free), with the shared Go cache at 33 GiB. Parent verified no active compiler/tests, acquired the shared build lock and removed 16.10 GiB of oldest regenerable Go cache entries, preserving newer entries and all baseline/raw test evidence. That Task03 recovery checkpoint left 26 GiB free. Future heavy work must monitor cache growth and reuse the retained cache.

| Order | Work order | Independently testable result |
| --- | --- | --- |
| 00 | [Baseline](task-00-baseline.md) | reproducible pre-change samples and numeric tolerance gate |
| 01 | [Bounded browser reads](task-01-browser-reads.md) | additive authorized summary/page/inspect contracts; old complete lists intact |
| 02 | [Ordinary loading isolation](task-02-ordinary-isolation.md) | ordinary boot/list unchanged in content; targeted child routing remains usable while excluding child panels |
| 03 | [Demand history and inspection](task-03-demand-ui.md) | typed bounded browser cache, explicit history and targeted child reads on both surfaces |
| 04 | [Measured acceptance](task-04-acceptance.md) | focused browser and performance evidence for every criterion |

Dependencies are strictly sequential; no simultaneous shared-contract writers. Backend DTO/action/client types are owned together by task01; task02 wires ordinary boot/list, targeted child identity/routing and restoration atomically; task03 switches browser Errands consumers. Additive endpoints can land before their consumer; no intermediate step silently narrows legacy lists. The user subsequently authorized implementation and parent-owned integration, commit and push. No current worker is active; live database repair or deployment is outside these work orders.

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

Work orders carry exact criterion references and root-safe commands. Existing coverage seeds are named in the design; new browser tests are explicitly proposed paths. Use TDD after later implementation authorization. Task04 owns public coordination documentation, targeted rendered checks and paired performance runs; there is no generic QA/review/full-audit work order. Keep statuses pending until evidence is recorded, then promote spec lifecycle only after the entire package passes.

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs/tasks docs/plans/errand-history-loading
```

PR-documentation coverage preflight uses `.github/scripts/pr-docs.cjs` exported `validateCoverage` with actual new work-order/spec contents (including untracked paths), as recorded in the design handoff. Do not rely on a HEAD-only diff to discover new files.

## Risks and handoff

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

## Parent compatibility integration

The independent read-only compatibility investigation is incorporated. Task02 owns targeted child routing before ordinary-list isolation, retains complete task-removal inventory, and protects ordinary membership/revision/subscription behavior. Task04 names existing normal-session browser controls, including zero-Errand and mixed-task variants. Task00 now names proposed measurement specs and exact baseline commands; task04 reruns the same harness with candidate-only evidence labels. The preview counts one running row separately from a completed pending result and retains individual Stop. These are design corrections, not executed product checks.

After these edits, catalog validation, specification lint, scoped whitespace and documentation contract-graph checks passed. All existing referenced test paths were checked; explicitly proposed history and measurement specs remain future implementation files. No production edits or measurements occurred. The package is ready for the required design-review checkpoint, with numeric performance budgets deliberately gated on task00 evidence before production changes.


The user authorized the fresh paired comparison on 2026-11-14. The [paired protocol](paired-performance-protocol.md) fixes interleaving, samples, unchanged threshold formula and structural gates before collection. Earlier references to a pending user decision are historical; Task04 remains in progress until this matrix passes.
