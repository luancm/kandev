---
id: "01-result-duration"
title: "Persist and display elapsed time on finished Errand results"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
acceptance_criteria:
  - AC-TASKS-ERRANDS-010.6
  - AC-TASKS-ERRANDS-010.7
  - AC-TASKS-ERRANDS-011.5
  - AC-TASKS-ERRANDS-011.7
system_design:
  - ../../specs/tasks/system-design/errand-result-duration.md
---

# Task 01: Recorded elapsed time on finished results

## Outcome and scope

Persist optional elapsed milliseconds on the existing immutable delivered item and display them in both collapsed and expanded completion history. Reuse the existing creation-to-recorded-settlement meaning, including queueing and continuation intervals; do not present it as isolated provider runtime. Older results remain readable without invented measurements.

The existing requirement already asks for recorded duration where available. The [owning design](../../specs/tasks/system-design/errand-result-duration.md) specifies measurement, compatibility and authority. No new requirement ID, database migration, telemetry, polling or recovery behavior is needed.

## Acceptance

1. Normal delivery and preserved cancellation producers copy `duration_ms` from the claimed result row using creation and recorded settlement only. Preserve zero; omit absent/zero timestamps and negative intervals. Prove JSON omission/zero behavior, notification and turn-context delivery, preserved deletion notice and unchanged stored metadata after later child activity. Duration must never alter claim/revision/admission semantics.
2. The parser preserves finite nonnegative safe integers, rejects malformed values without dropping the card and keeps legacy snapshots timing-free. The localized shared card shows the frozen value collapsed and expanded, preserving historical status, result, placement and Open. Existing WAIT timing remains distinct.
3. Desktop and native-phone checks prove duration survives reload and later child follow-up, long labels fit without document overflow, and Open/Back, keyboard/touch disclosure and the parent composer remain usable. Use current source builds with isolated mock data; do not inspect or mutate the user's live instance for browser testing.

## UI-02: Finished result duration

Parent transcript, desktop and phone; same shared inline disclosure:

```text
Collapsed, measured:  > 1 errand finished (2m 14s)
Collapsed, legacy:    > 1 errand finished
Expanded:
  Result title                         Open
  Done (2m 14s)
  Recorded result preview
```

The event/count precedes elapsed time, and the expanded value stays beside the historical outcome. Punctuation and line breaks are illustrative; use existing localized duration units and translation interpolation when a sentence is needed. Phone may wrap the status/duration group while keeping Open reachable with its existing touch target. Preserve the current historical disclosure (nearest shipped exemplar), single chat scroller, safe-area geometry, parent reading position and composer. See the [combined preview](plan.md#ui-acceptance). No additional overlay or mobile-specific state is introduced.

## Files and ownership

- `apps/backend/internal/errands/compose.go`: `DeliveryItem` and `deliveryItems`; `delivery.go`: preserved cancellation envelope. Add only a small adjacent measurement helper if both producers need it. Existing `compose_test.go`/`delivery_test.go` or a focused `delivery_duration_test.go` own producer regressions.
- `apps/web/components/task/chat/errands/errand-parse.ts`, `errand-finished-card.tsx` and their parser/message-renderer tests. Reuse `formatPromptDuration` and locale unit keys. Update typed delivery-item fixtures only where required.
- Existing seven locale catalogs if new copy is necessary; use the Traditional Chinese converter and keep current translations from recovery intact.
- `apps/web/e2e/tests/session/errand-result-duration.spec.ts` and `mobile-errand-result-duration.spec.ts`: reuse the existing immutable-result helpers and flows, with `finished result duration` in focused test names. Separate spec files avoid the recovery worker's ownership of the existing Errands specs. A small `helpers/errand-result-duration.ts` may share the paired scenario without changing `helpers/errands.ts`.

Parent owns specifications, this work order, integration and final acceptance. The worker owns a `duration-handoff.md` alongside this file. Do not change settlement timestamps, result capture, status/refusal/continuation, cleanup, normal-session behavior or shared build configuration. Do not revert other edits, spawn children, stage, commit, push or deploy.

## Dependencies and parallelism

Backend snapshot/parser/card changes are independent of cleanup production, but recovery changes overlapping locale catalogs and browser specs require explicit ownership release or parent coordination first. Begin with the coherent producer/parser/card slice; keep overall acceptance pending until browser coverage can integrate. Shared heavy checks use `/tmp/kandev-errands-repair-build.lock`; preserve every process handle. Upstream refresh and final branch push remain parent-owned.

## TDD and verification

Add behavioral tests before production changes. Cover a measured value, zero, missing creation/settlement, reversed interval, legacy JSON, negative/fractional/string/NaN/infinite/unsafe values, snapshot immutability and all owned producers. Include measured/zero/absent round trips through `DecodeSendNowDelivery` and prepared-notice metadata, preserving the strict decoder and replay identity. Test saved messages through the existing renderer rather than only asserting formatter calls. Backend new test names start with `TestDeliveryDuration`; include every other changed test in the final selector.

Run from the repository root, retaining logs and explicit pass/fail/skip results:

```bash
(cd apps/backend && flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -race -trimpath -p 1 -parallel 1 ./internal/errands -run 'DeliveryDuration|DeliverySnapshot|Compose|CancellationNotice|DeletedOutstandingErrand' -count=1)
(cd apps/web && pnpm exec vitest run components/task/chat/errands/errand-parse.test.ts components/task/chat/errands/errand-delivery-message.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run exact-file Go lint with `bash scripts/lint-go-changed` and current scoped web lint/formatting for every changed source file. Use the existing build lock for builds and browser runs as well:

```bash
make build-web
make build-backend
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium e2e/tests/session/errand-result-duration.spec.ts -- --grep 'finished result duration' --retries=0
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome e2e/tests/session/mobile-errand-result-duration.spec.ts -- --grep 'finished result duration' --retries=0
```

Inspect a phone screenshot for long localized timing/status content in the expanded row. A mocked provider proves composition/persistence, not external provider timing precision. Do not rerun full suites without a specific unresolved concern. Check disk after the packet and remove only owned scratch.

## Risks and results

The main semantic risk is confusing creation-to-result capture with per-turn runtime; both design and UI wording preserve the existing elapsed meaning. Additive metadata must survive intermediate encoders, including cancellation notice preparation. Browser and locale ownership must be coordinated with recovery.

Producer/parser/card implementation is complete and reviewed with no actionable findings by Era Sol `4fa0283f-f14a-437c-adca-e55ef9f0b10a`. The [implementation handoff](duration-handoff.md) records backend/frontend RED, backend race GREEN, 38 focused Vitest passes, typecheck, i18n, scoped lint and formatting. These checks were inspected by the reviewer, not rerun. No locale additions were needed.

Browser worker `8671c3be` completed its isolated desktop/phone packet with 1/1 passing in each project on matching before/after source manifests. The [browser handoff](duration-browser-handoff.md) records exact commands, discarded stale-build attempts, reload/later-follow-up evidence and inspected phone screenshot. Long duration is seeded layout evidence; a separate real mock result verifies timestamp-derived duration. Scoped browser formatting/lint/sleep-ratchet checks passed. Era Sol accepted the measurement/persistence/phone evidence and requested real Tab navigation. That correction passed managed Chromium 1/1 and Era Sol `952a4e39-b5a3-4498-8b38-05e5f32db44f` accepted it with no findings. Parent integration remains pending; earlier source-changing runs are not acceptance evidence.

Public coordination guidance now explains recorded elapsed time, queued/idle intervals and immutable historical values. Public-doc validation passed all 63 tests and 55 pages; documentation catalog and all specification lint passed. Final code and browser reviews are accepted; commit and push remain parent-owned.
