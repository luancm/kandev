---
id: "26-documentation-reconciliation"
title: "Reconcile manifests and final repair evidence"
status: done
wave: 26
depends_on:
  - "25-current-dark-media"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-004
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
  - REQ-TASKS-ERRANDS-012
acceptance_criteria:
  - AC-TASKS-ERRANDS-004.13
  - AC-TASKS-ERRANDS-010.11
  - AC-TASKS-ERRANDS-011.7
  - AC-TASKS-ERRANDS-012.2
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 26: Reconcile manifests and final repair evidence

## Scope and authority

Repairs review finding(s) 21 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. No production changes have been made for this packet.

## Low-level implementation and regression plan

### Exact affected records

- docs/plans/errands/plan.md.
- docs/plans/errands/task-07-task-summary-indicators.md, task-09-e2e-and-docs.md, task-13-inline-chat-summary.md: completed -> done, preserving historical Results and explicit supersession of inline inventory by task14.
- docs/plans/errands/task-15-context-window.md remains done; manifest checkbox must match. Do not silently call the newly found context-clear bug accepted; link new Task 20 repair order.
- task-16-context-token-readout.md remains in_progress for media.
- task-17-panel-order.md remains in_progress until Task 24 passes; its existing depends_on task16 is edit-order provenance, not a reason browser coverage must wait for media. Clarify that source implementation prerequisite is met and media is independent; retain shared-file serialization.
- task-18-await-duration.md remains done, not reopened merely because task17 media/order acceptance is pending.
- docs/specs/tasks/README.md: add requirements AND system-design links for errands.md, errand-context-window.md, errand-panel-order.md and errand-await-duration.md, which are absent from the existing task index. No new glossary proposed.
- docs/specs/tasks/system-design/errands.md and errand-context-window.md: repair algorithms/ownership notes as below, not new requirements.
- Include corresponding requirement/design plan links in the primary repair package. Main docs/plans/errands manifest should reference the repair package and vice versa.
- docs/public/coordination.md and agent-communication.md already describe active-first/token/await behavior correctly at HEAD. README.md and docs/screenshots.md search found no Errands text needing a correction. The new AC004.13 uncertainty policy requires a concise public recovery note in both pages with implementation: known failures retry automatically; genuinely uncertain reception retains the result for inspection and deliberate recovery. Do not promise automatic replay or call handler success a receipt. Update the additive MCP response reference. This design checkpoint does not publish unimplemented behavior. If media is eventually published, docs/screenshots.md/asset references require the normal separate docs review.

### Concrete manifest edits

The primary has applied the metadata, index and manifest corrections at the design checkpoint. Preserve those edits. This packet remains pending for final implementation receipts, public recovery copy and closure of remaining acceptance gates.

1. Establish one CURRENT checkpoint near top with HEAD 6400a265bb8a0a35fb702eced2aff1eeb81023d9 origin-only publication receipt, already completed baseline/task18, current reviewed repair package, task16-media/task17-browser gaps and explicit no-PR/upstream/default/live scope. Rename the old 'Current delivery checkpoint (2026-10-29)' as historical; its own text contains superseded pending claims.
2. Add 16,17,18 to main Work orders table and Progress. Set 15 checked/done; 16 and 17 unchecked/in_progress; 18 checked/done. Existing table ends at15 even though later appendices mention newer orders.
3. Manifest frontmatter add REQ-TASKS-ERRAND-ORDER-001 and REQ-TASKS-ERRAND-AWAIT-DURATION-001, plus ../../specs/tasks/system-design/errand-panel-order.md and errand-await-duration.md. Context pair is already declared. Keep every linked work-order requirement covered by its referenced system design and every work-order design listed by manifest.
4. Keep original completed baseline work orders as historical implementation records; new review defects receive new linked repair orders instead of overwriting old evidence. Updated repair Results must name exact failed-before regression, final command, count, source and log after implementation.
5. Correct sections still describing UI-11 inline inventory as current: Recorded design boundaries desktop errand list, Scope table, E2E scenario matrix and structural UI preview table. Current is 010.11/011.7 chip-owned management, separate start/await/finished history; superseded 010.10/011.6 and UI-11 belong under labelled historical acceptance, not current test obligations. Existing current visual previews UI-03/UI-08 should include Active/Completed parent sections and context used/capacity/bar.
6. Don't rewrite the coordination archive or discard publication receipts. Don't convert previous SQLite/real-provider/PostgreSQL evidence into certification for newly repaired code; older broad PostgreSQL timeout and provider-specific support limitations stay explicit.
7. Normalize work-order status values to pending/in_progress/blocked/done/cancelled, never completed. Requirements stay normative; plans own delivery, media/coverage gaps and publication. If primary normalizes draft accepted spec metadata, do so explicitly from established acceptance and keep no new behavior hidden in that cleanup.

### Design amendment inventory (primary owns durable files)

- Main Errands design Models/API + Frontend: fallback copies immutable role/parent, read repair batches flag-gated aggregate, CAS conflict re-observes it; task-scoped load freshness keyed by shared connection generation + read scope; stale attempts cannot publish/clear replacements.
- Main Frontend Tabs: add explicit Kanban PreviewSessionTabs owner parallel to Threads TaskSessionNavigationContext; selection and Back local to host, no workflow transfer.
- Main Copy/accessibility: announce on rendered/detail-active surface independent of unread acknowledgement, one polite region per view.
- Context design Data/Persisted summary projection: accept explicit null atomically into cache, omission is no-op, context request epoch fences live events/clear against delayed list/hydration, full-session reconnect transport generation included, no extra requests/subscriptions.
- Correct obsolete context producer path internal/agent/orchestrator/event_handlers_git.go -> internal/orchestrator/event_handlers_git.go.
- Ordering design already matches current implementation; add no new sort semantics. Wire its Implementation Plans link if absent and point task17 to the exact new mixed-parent evidence.
- Conformance repairs reuse existing ACs; the user-selected uncertainty policy adds AC004.13 in errand-result-delivery.md. Existing reset-freshness and bounded-summary criteria remain referenced dependencies; if named in new work-order frontmatter, reference their OWN owning system designs too because main Errands design frontmatter does not declare those REQ families.

### Documentation validation

Run only after primary creates/edits durable artifacts; none were run/claimed in this design errand:
~~~
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/plans/errands docs/specs/tasks
git status --short -- docs/plans/errands docs/specs/tasks
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
~~~

Add primary's new repair-plan directory to diff/status commands. Use .github/scripts/pr-docs.cjs validateCoverage with BOTH committed feature changes and working/untracked new package files (the existing manifest's script demonstrates this). For coverage, collect ALL .md in both old and new plan directories AND each referenced requirement/design, not only main errands.md. Run catalog and spec linters, then coverage cross-reference preflight; syntax/catalog success alone doesn't prove work-order mapping. Do not invent a passing count before running.

## Dependencies and ownership

[Task 25](task-25-current-dark-media.md)

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

`sequential`. The integrating session may overlap disjoint read-only preparation, but grants one writer for each shared schema, repository, orchestration or browser file.

## Results

Design checkpoint: manifest table/checklist, Tasks 07/09/13 status enum, Tasks 17/18 edit-order clarification, specification map and coordination-history EOF are corrected. Catalog/specification lint, full-range documentation coverage and whitespace checks passed as recorded in plan.md. The reconciliation checkpoint below supersedes this initial design-only status. Do not convert this documentation check into product validation.


### Historical pre-demo reconciliation checkpoint (2026-11-02)

Public coordination guidance and the agent-tool reference now distinguish known retryable failures from uncertain reception and describe await correlation metadata without calling it a receipt. Current design notes cover fallback identity, aggregate repair, request freshness, Preview/Threads ownership, active-surface announcements, passive context reconciliation, and flag-off retained-role restrictions. The old ordinary-tab ADR wording and view-model comment are reconciled. The missing conversation-navigation design reference in the repair manifest is added.

Catalog/spec lint passed (422 decisions, 1,862 specifications), public-doc tests passed 63/63, and all 54 published pages validated. The full committed feature range plus current changes passes documentation coverage: 709 changed paths and 85 loaded reference documents, `/tmp/errands-task26-coverage.json`. Source and publication receipts remain historical where labelled. This task remains in progress until Task25's missing media prerequisites are resolved; Task24 browser receipts are integrated; no media/browser completion is inferred from documentation validation.

### Final reconciliation (2026-11-02)

Task24's browser receipts and Task25's new desktop/phone recordings, source provenance, synthetic-data disclosure, playback and cleanup evidence are integrated. Original Task16 and repair Tasks25/26 are now done, with current checkpoints near the top of both manifests and matching table/checklist statuses. All 29 repair work orders and all original work orders are done; earlier pending statements remain only in labelled historical checkpoints. The user clarified that the existing feature checkout is the demo source, so the earlier landing-repository/camera prerequisites were removed from Task25 rather than left as artificial blockers.

Public guidance already matches the implemented behavior; this final change updates internal acceptance records only. Local media is linked from Task25 without publishing assets or changing the screenshot catalog. Earlier PostgreSQL skips and provider-specific limitations remain unchanged. Final validation passed: catalog (422 decisions and 1,862 specifications), all specification lint, 63/63 public-doc tests, 54 published pages, whitespace checks and full-range documentation coverage (728 changed paths, 88 reference documents, status `covered`). Receipts: `/tmp/errands-demo-doc-catalog.log`, `/tmp/errands-demo-spec-lint.log`, `/tmp/errands-demo-public-tests.log`, `/tmp/errands-demo-public-validate.log` and `/tmp/errands-final-demo-coverage.json`.
