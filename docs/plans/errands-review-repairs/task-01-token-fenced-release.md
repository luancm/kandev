---
id: "01-token-fenced-release"
title: "Token-fenced result release"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-004
acceptance_criteria:
  - AC-TASKS-ERRANDS-004.4
  - AC-TASKS-ERRANDS-004.9
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 01: Token-fenced result release

## Scope and authority

Repairs review finding(s) 10 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. Implemented with exact-token and route checks on every CAS retry.

## Low-level implementation and regression plan

Own `internal/errands/compose.go` and `delivery_test.go` before Task 03 replaces per-row claims with its transaction. `releaseClaim` filters old list rows by token, then `update` rereads fresh state but passes `releaseResult`, which checks only delivered and may release a successor B. Pass the requested token into the update closure and test both `ResultState==delivered` and `DeliveryToken==token` on every CAS retry; additionally verify route when the releasing caller has route-specific authority. On mismatch return false and publish/wake/deliver nothing. For batch/recovery release use the same predicate in the database transaction; Task 03 adds immutable result-revision and admission-generation fencing to this same predicate. Empty token remains no-op. This does not solve whether a given token is eligible for release (Task 04).

RED `TestReleaseClaimDoesNotReleaseReplacementToken`: pause A after list, release A, claim B, resume A, assert B's state, token, route, result, version and absence of new delivery/wake unchanged; positive control confirms A alone releases. Repeat both await and turn routes and CAS-conflict retry. Command:

```
(cd apps/backend && go test -race ./internal/errands -run 'TestReleaseClaimDoesNotReleaseReplacementToken')
bash scripts/lint-go-changed apps/backend/internal/errands/compose.go apps/backend/internal/errands/delivery_test.go
```

## Dependencies and ownership

None.

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

`sequential`. The integrating session may overlap disjoint read-only preparation, but grants one writer for each shared schema, repository, orchestration or browser file.

## Results

Implemented `releaseClaim` fencing against the requested token and listed route inside every fresh-state mutation/CAS retry. Route-specific release APIs restrict await versus turn-context/notification authority; the adjacent `await.go` change is only the `ReleaseAwait` call-site argument. Empty tokens remain no-ops. Token mismatch, route mismatch and CAS replacement produce no writes, publications, wakes or delivery. Matching-release positive controls exercise those effects.

- RED: `GOMAXPROCS=2 go test -race -trimpath -p 1 ./internal/errands -run 'TestReleaseClaim(DoesNotReleaseReplacementToken|ChecksRouteAuthority|RechecksTokenAfterCASConflict)' -count=1 -v` reproduced all six original failing route subcases; `/tmp/errands-recovery-task01-red.log`.
- Final GREEN: `GOMAXPROCS=2 go test -race -trimpath -p 1 ./internal/errands -count=1 -v` passed 84 top-level tests and 27 subcases with no skips, including notification authority, independent route-only CAS replacement and event/wake/delivery positive controls; `/tmp/errands-recovery-task01-final-tests.log`.
- Final integration after Task06 production completion: the same focused release selector passed 3 top-level tests and 9 subcases under `-race`, no skips; `/tmp/errands-recovery-task01-integration-race.log`.
- Final lint: `bash scripts/lint-go-changed apps/backend/internal/errands/compose.go apps/backend/internal/errands/await.go apps/backend/internal/errands/delivery_test.go` passed with 0 issues; `/tmp/errands-recovery-task01-final-lint.log`. The first lint found a redundant embedded Store selector in the recovered test, corrected before the final checks.
- No database/schema changes, Task03 atomic claim/revision fencing, or Task04 await recovery implemented here. No commit, push or live-instance mutation.
