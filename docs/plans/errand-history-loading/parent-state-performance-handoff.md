# Ordinary parent status performance correction

## Cause and change

The candidate's ordinary-session HTTP response was small (2250 bytes for three ordinary sessions) but still took approximately 378–411 ms with 1000 finished children. The isolated CPU profile traced almost all handler CPU to `ParentErrandState` and `ProjectSnapshot`: the parent waiting indicator enumerated finished Errands, loaded each child session and assessed recovery to compute two booleans.

The production repository now computes existence and outstanding work in one scalar SQL statement, using the existing shared outstanding/recovery predicate. Parent scoping uses the existing parent index. No result payloads, child DTOs, pending-action lookups or per-child service calls are materialized. Aggregate database work may still scale with history; this is not a constant-scan claim. Existing feature-off and empty-parent guards remain, database errors propagate, and stores without the optional capability retain the complete projection path. The production composition passes the concrete SQL repository and therefore uses the compact capability.

No schema or migration change is required. The existing upgrade manifest/fixtures stay unchanged. This is internal read optimization; existing public guidance about ordinary-session behavior remains valid and no public copy change is necessary.

## Evidence

- Direct HTTP diagnostic: `/tmp/kandev-history-http-diagnostic`, managed run `/tmp/kandev-run.e2e.o1Tk1m3P.log` (1/1).
- CPU profile: `/tmp/kandev-history-http-profile-diagnostic/session-errand-history-bas-3f89e-eline-1000-finished-Errands-chromium/ordinary-http.pprof`; summary `/tmp/kandev-history-http-profile-top.log`; managed run `/tmp/kandev-run.e2e.SpsbqAdA.log` (1/1). Only the disposable fixture enabled profiling. Temporary harness instrumentation was restored.
- Service RED `/tmp/kandev-parent-state-red.log`: all four compact-read cases failed because the old service enumerated children; empty/feature-off control passed.
- Initial GREEN attempts `/tmp/kandev-parent-state-green.log` and `/tmp/kandev-parent-state-both-green.log` retain the test-fixture failure where parent and child IDs were accidentally identical. Recovery parity and the scoped 1000-row query-plan test passed; the fixture is corrected. This is not a production failure or accepted final receipt.
- Final targeted race receipt `/tmp/kandev-parent-state-both-final.log`: 24 top-level tests passed, zero failures/skips, across service, SQLite/PostgreSQL repository and orchestrator. SQL guard passed (`/tmp/kandev-parent-state-sqlguard.log`). Full store conformance passed on SQLite and PostgreSQL, including upgrade checks (`/tmp/kandev-parent-state-conformance.log`, 185.547 s). The standard five-minute changed-package lint wrapper timed out during package loading (`/tmp/kandev-parent-state-lint.log`); the longer scoped run found one unchecked `rows.Close` in the new query-plan test. The test now checks that error. Final three-package lint passed with zero issues against the repository-selected base `37f081a976526de1a6967f8714e72e247ec0b386` (`/tmp/kandev-parent-state-lint-final.log`, extended 12-minute limit). The corrected query-plan test also passed with race detection (`/tmp/kandev-parent-state-plan-final.log`). New paired browser measurements also remain pending. The original performance matrix is retained; strict limits have not changed.

## Source snapshot

- `apps/backend/internal/errands/interfaces.go`: `bba2f1ea560160715b45b589735c25b154a8be4935564d375ec3caad8828afb1`
- `apps/backend/internal/errands/delivery.go`: `33b2c86cd33c23a0eda1848eba0ec47b1ff19bd2fb793dd1a7dcf15b65866c47`
- `apps/backend/internal/errands/parent_state_test.go`: `c6c13ecc270b845b752dab84660ef6759b45447126af18447352cd69ac241114`
- `apps/backend/internal/task/repository/sqlite/errand_parent_state.go`: `667a6ac8f172cbc027ef07fbed62deeb4451c368085f916fb4d899c7958c4caf`
- `apps/backend/internal/task/repository/sqlite/errand_parent_state_test.go`: `3bb56bdcb16315cab9aac170f7182ca33ff696aafb4cd9e4bfd9fb1b486195d2`
- `apps/backend/internal/orchestrator/errand_parent_queue_test.go`: `3b7a97ece8e3ba9493b28eda57c7c5cd806391bac161240da97ade6cda3217ed`

## Final check receipts

- `/tmp/kandev-parent-state-red.log`: `4eeff8171e7dc8ec7a4fa40cd5d194ff560305ce5ed54add10acfeb2bf98256f`
- `/tmp/kandev-parent-state-both-final.log`: `82436079b2cf268f388e0b226c62f640da52bd87ba3cd0c195e1806778ee460f`
- `/tmp/kandev-parent-state-conformance.log`: `e52c070b3216841f7c84d575ab60a918d7384728a6018d0e8594cf85d3a526e1`
- `/tmp/kandev-parent-state-sqlguard.log`: `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`
- `/tmp/kandev-parent-state-lint.log`: `81ff2de3b9f14a98b26c0230721d14a4c3016dbffaa361f3ef077b13a9aae4b2`
- `/tmp/kandev-parent-state-lint-extended.log`: `e5c18f638f9879d07db589410c23f082aefb4b720df03ea25e48e75260d8b405`
- `/tmp/kandev-parent-state-lint-final.log`: `e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47`
- `/tmp/kandev-parent-state-plan-final.log`: `5ca6059a49c7ea866bba12448365ac18899fb863f2a830412caeec908bb35cd5`

SQL guard is silent on success; its exit 0 is recorded by the serialized check runner. Store conformance passed 11 top-level tests with no skips, including both dialects and stable upgrades. No disposable PostgreSQL listener remains after the runner exited. Browser performance acceptance is still pending.

## Rebuilt browser smoke

Managed production-build Chromium smoke passed 1/1 with 1000 finished children: `/tmp/kandev-run.e2e.lOfgIlkh.log`, raw sample under `/tmp/kandev-history-parent-state-build-smoke`. Full child session/transcript/turn caches are all zero after Back. Browser resource timings for ordinary-list requests are approximately 3.4–53.2 ms across navigation/reload/restore, compared with 378–411 ms in the earlier direct HTTP diagnostic. These different measurement contexts are supporting evidence, not a paired latency acceptance claim. The new full paired cohort completed under `/tmp/kandev-history-paired-v2`: 84/84 scenarios, all structural bounds and 28/30 strict numeric limits pass. [The second paired handoff](paired-performance-v2-handoff.md) records the two remaining low-history fixed-cost limits. The history-dependent waiting-indicator cost is resolved; overall acceptance remains open.
