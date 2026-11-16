# Finished-result duration handoff

## Result

Added optional `duration_ms` to the immutable `DeliveryItem` snapshot. Both normal parent-turn composition and preserved cancellation notices derive it from the same result row's `CreatedAt` and recorded `SettledAt`; missing or zero timestamps and reversed intervals omit the field, while a measured zero remains present. The implementation does not read the clock or `UpdatedAt`, and it does not change settlement, claim, revision, admission, or status behavior.

Extended the existing strict send-now codec through the nested `DeliveryItem` schema. Round-trip coverage preserves measured, zero, and absent durations in prepared and normal receipts while retaining source replay identity.

The frontend parser preserves only finite nonnegative safe integer milliseconds and degrades malformed values to unavailable without dropping the card. The shared finished card shows the stored duration collapsed and beside the expanded historical status. It reuses `chat:errandCompleted` and the existing `task:durationUnitSeconds`, `task:durationUnitMinutes`, and `task:durationUnitHours` keys; no new locale keys are needed. The existing disclosure and result row now allow wrapping for narrow layouts.

## TDD evidence

- RED backend: `flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -race -trimpath -p 1 -parallel 1 ./internal/errands -run 'DeliveryDuration' -count=1` failed only the new duration expectations: turn-context and notification composition omitted elapsed time, zero was omitted, the cancellation notice omitted elapsed time, and the strict decoder rejected the new nested field.
- RED frontend: `pnpm exec vitest run components/task/chat/errands/errand-parse.test.ts components/task/chat/errands/errand-delivery-message.test.tsx` reported 4 expected failures and 34 passes: duration parsing, collapsed display, and expanded display were not implemented.
- GREEN backend: `flock -x /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -race -trimpath -p 1 -parallel 1 ./internal/errands -run 'DeliveryDuration|DeliverySnapshot|Compose|CancellationNotice|DeletedOutstandingErrand' -count=1` passed after adding the missing/zero settlement timestamp case and moving focused regressions to `delivery_duration_test.go`.
- GREEN frontend: the same focused Vitest command passed both files, 38 tests total.

## Other checks

- `pnpm run typecheck` passed.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` passed; the checker reports 608 unreferenced catalog entries as a non-failing warning.
- Exact-file `bash scripts/lint-go-changed` passed with `0 issues`; exact-file Oxlint passed with `--deny-warnings`; Prettier check passed; `gofmt -d` and `git diff --check` were clean.
- Browser E2E, rendered phone inspection, reload/follow-up acceptance, and reviewer disposition remain pending because browser specs and locale catalogs are coordinated with the recovery work. No browser specs or locale catalogs were edited here.

## Handoff and remaining acceptance

The producer, codec, parser, and shared-card slice is ready for parent review and integration. The parent still needs the requested Era Sol Que Me Faltava review, desktop and native-phone `finished result duration` E2E coverage, reload and later-child-follow-up proof, and a rendered phone check for long localized content and Open reachability. The card continues to use the existing inline history disclosure and scroll owner; no new mobile navigation or interaction state was introduced.

No commit or shared Git mutation was made. The worktree also contains the cleanup worker's edits in `apps/backend/internal/orchestrator/` and its cleanup plan/spec files; those files were not changed by this packet.

## Source hashes

Hashes identify the owned implementation files at `HEAD` `b2bbe9d5b764223c0dc100b9075a670159e1e9a8` plus this uncommitted packet:

| File | SHA-256 |
|---|---|
| `apps/backend/internal/errands/compose.go` | `c1e4c39d7f192aea5f0a0fccbbace894ce6be86769db24dab3bda9ef2959685d` |
| `apps/backend/internal/errands/delivery.go` | `2681304e1ad83e229a33192bf4e73e470b491916daf94eaa0a21eb1f06f387ac` |
| `apps/backend/internal/errands/delivery_duration.go` | `a3edfea9b5f131d5fa69c003d6d55c9c3e0258c6e238f2d5cc37e3c36adafa36` |
| `apps/backend/internal/errands/delivery_duration_test.go` | `224385998114173de6615b5032f27fdc53da39bb2ec12004a696075c0821661f` |
| `apps/web/components/task/chat/errands/errand-parse.ts` | `c97240d6edcbc35e30813ffc5366c6192030d26bdb6aca6c5b7c3978cb4641c4` |
| `apps/web/components/task/chat/errands/errand-parse.test.ts` | `cbf0c1c18dc25216c4529cd1d14dae56e270a87766d2d684891c6cedcab6f3e2` |
| `apps/web/components/task/chat/errands/errand-finished-card.tsx` | `64995d51871ff41464918fd8c285f7f6550808efb03aa8005f83af1867fb7ad9` |
| `apps/web/components/task/chat/errands/errand-delivery-message.test.tsx` | `8fcb8d2c4bc0847addaf41d479d3faf8b55e043e3b298b1925490838a5f606a7` |
