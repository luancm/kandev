---
id: "18-fallback-session-identity"
title: "Preserve role and parent in summary fallback"
status: done
wave: 18
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-005
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
acceptance_criteria:
  - AC-TASKS-ERRANDS-002.3
  - AC-TASKS-ERRANDS-005.5
  - AC-TASKS-ERRANDS-010.3
  - AC-TASKS-ERRANDS-010.5
  - AC-TASKS-ERRANDS-011.1
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Task 18: Preserve role and parent in summary fallback

## Scope and authority

Repairs review finding(s) 20 at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals.

## Low-level implementation and regression plan

Owned production symbol is taskSessionSummaryObservation in apps/backend/internal/task/service/service_sessions.go; test file apps/backend/internal/task/service/service_session_summary_projection_test.go. Copy Role:session.Role and ParentSessionID:session.ParentSessionID into the existing observation literal, next to immutable identity. Do not derive them from metadata, infer from title or condition on features.errands. The concrete narrow repository already selects these fields and models.TaskSessionSummaryObservation.ToTaskSession already copies them.

Extend TestSessionSummaryFallbackProjectsContextMetadata with errand and ordinary-peer rows and assert both list and batch fallback preserve Role/ParentSessionID through ToTaskSession and dto.FromTaskSessionSummaryObservation. Keep context/error/model bounded projection and exclusion assertions. Name TestSessionSummaryFallbackPreservesErrandIdentity. Use the existing contextMetadataFallbackRepository that deliberately exposes ONLY repository.SessionRepository; accidentally embedding *sqlite.Repository exposes the narrow reader and never tests the fallback. Include nil input to the pure converter and ordinary role ''/parent '' baseline. The initial RED is missing role/parent, not failure to mount UI.

No public schema/API/migration change: already published optional fields merely stop disappearing on one implementation path. Error and authorization propagation stays as-is.

~~~
(cd apps/backend && go test -trimpath -race ./internal/task/service ./internal/task/dto -run 'SessionSummary|TaskSessionSummary|Errand' -count=1)
bash scripts/lint-go-changed apps/backend/internal/task/service/service_sessions.go apps/backend/internal/task/service/service_session_summary_projection_test.go
~~~

## Dependencies and ownership

None.

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

Parallel-safe with Task04C backend projection: the owned task-service files do not overlap its inventory. The user authorized two independent work orders. Heavy checks must acquire `/tmp/kandev-errands-repair-build.lock` and follow `/tmp/errands-verification-resource-policy.md`; no concurrent builds, lint, typechecks or browser suites.

## Results

**Status: done.** `taskSessionSummaryObservation` copies the stored role and parent session identity into the bounded observation. The service regression uses the external `service_test` package so it can directly verify `dto.FromTaskSessionSummaryObservation` without creating an import cycle.

- **TDD RED:** `cd apps/backend && go test -trimpath -race -p 1 -parallel 1 ./internal/task/service -run '^TestSessionSummaryFallbackPreservesErrandIdentity$' -count=1 -v` failed at the batch fallback with empty role/parent instead of `errand` and `fallback-summary-parent`. Log: `/tmp/errands-task18-red-final.log`.
- **Focused GREEN:** `go test -trimpath -race -p 1 -parallel 1 ./internal/task/service ./internal/task/dto -run '^TestSessionSummary(FallbackPreservesErrandIdentity|ServiceUsesNarrowReads)$|^TestSessionRoleProjectsOnSessionDTOs$|^TestTaskSessionSummaryProjectionPreservesCompactFieldsAndErrorClearing$' -count=1 -v` passed 4 tests: 2 service and 2 DTO. Both list and batch fallback paths pass through `ToTaskSession` and `dto.FromTaskSessionSummaryObservation`. Log: `/tmp/errands-task18-focused-green-final.log`.
- **Required broader filter:** `go test -trimpath -race -p 1 -parallel 1 ./internal/task/service ./internal/task/dto -run 'SessionSummary|TaskSessionSummary|Errand' -count=1` exited 1 because `TestTaskStatusSummaryErrandsRepairCASReloadsObservation` failed its `existing` and `missing` subtests in `service_status_summary_errands_test.go`; the DTO package passed. This test and its implementation are outside Task18 ownership. Log: `/tmp/errands-task18-required-filter.log`.
- **Lint:** `bash scripts/lint-go-changed apps/backend/internal/task/service/service_sessions.go apps/backend/internal/task/service/service_session_summary_projection_test.go` reported one unrelated `cyclop` issue in `service_status_summary_reconcile.go:56` (complexity 16, maximum 15). Patch-scoped `golangci-lint run ./internal/task/service --new-from-patch=/tmp/errands-task18.patch --timeout=5m --concurrency=1` passed with 0 issues. Logs: `/tmp/errands-task18-lint-final.log` and `/tmp/errands-task18-patch-lint-final.log`.
- **Formatting:** `gofmt -l` returned no paths and scoped `git diff --check` passed for both owned Go files.
