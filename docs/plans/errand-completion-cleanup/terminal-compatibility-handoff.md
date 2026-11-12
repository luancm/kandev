# Terminal compatibility handoff

## Result

The backend cleanup compatibility correction is implemented and locally verified; parent integration remains pending. Legacy completion repair now refuses `CANCELLED` and `FAILED` sessions even when an exact successful historical receipt exists. A closed completion intent settles its obsolete success obligation without repeating Stop or changing the terminal session state. A pending intent still requires its exact stop, authenticated absence, or proven successor supersession; session state alone never establishes runtime absence.

Missing and conflicting initial admission evidence remains protected and retryable. Those refusals now carry bounded evidence keys through both legacy capture and persisted reserved-intent retry, so unchanged evidence produces one per-row warning and one aggregate report total, while evidence changes and transient errors remain visible. Sweeps still reassess each row and continue independent result delivery.

## Changed files

- `apps/backend/internal/errands/service.go`
- `apps/backend/internal/errands/teardown.go`
- `apps/backend/internal/errands/teardown_compatibility_test.go`
- `apps/backend/internal/orchestrator/errand_completion_cleanup.go`
- `apps/backend/internal/orchestrator/errand_teardown.go`
- `apps/backend/internal/orchestrator/errand_teardown_integration_test.go`
- `apps/backend/internal/orchestrator/errand_terminal_compatibility_test.go`

No schema, SQL persistence, frontend, public documentation, shared plan, or specification files were changed by this Errand. The concurrent panel-controls changes and unrelated loading-performance note were preserved.

## RED and GREEN evidence

- `/tmp/kandev-errand-terminal-compat-cancel-red.json` reproduced pending and closed completion intents failing on `CANCELLED`; `/tmp/kandev-errand-terminal-compat-cancel-green.json` passed the corrected cases.
- `/tmp/kandev-errand-terminal-compat-legacy-integration-red.json` reproduced unchanged legacy admission refusal being returned again; `/tmp/kandev-errand-terminal-compat-legacy-integration-green.json` passed after deduplication.
- `/tmp/kandev-errand-terminal-compat-diagnostics-red.json` reproduced duplicate stable diagnostics through the service sweep; `/tmp/kandev-errand-terminal-compat-diagnostics-green.json` passed while continuing healthy sibling result delivery.
- `/tmp/kandev-errand-terminal-compat-pending-evidence-red.json` reproduced the persisted reserved-intent path emitting the generic, repeatable refusal instead of a typed admission-evidence refusal; `/tmp/kandev-errand-terminal-compat-pending-evidence-green.json` passed with one warning across unchanged sweeps, then exact stop after evidence repair.
- `/tmp/kandev-errand-terminal-compat-legacy-green.json` passed exact-receipt legacy rejection for both `CANCELLED` and `FAILED` sessions.

The real SQLite integration matrix now covers missing and conflicting submission, sender, and admission-state fields, with exact evidence repaired before retry. Pending completion coverage includes live retry, unknown inventory refusal, and exact persisted-row removal paired with typed runtime absence. Closed receipt, cancellation race, failed stop/restart, successor, replacement, incarnation, result preservation, and exact-stop ownership regressions remain covered by the selected package suite.

## Final verification

The final race command exited 0 and passed 77 top-level tests (2 `internal/errands`, 75 `internal/orchestrator`), zero failures and zero skips. The selector was broadened to include the new admission-named regression:

```bash
flock -w 300 /tmp/kandev-errands-repair-build.lock env GOCACHE=/tmp/kandev-errand-terminal-compat-go-cache GOMAXPROCS=2 GOMEMLIMIT=6GiB go -C apps/backend test -race -trimpath -p 1 -parallel 1 ./internal/errands ./internal/orchestrator -run 'TestErrand.*(Completion|Teardown|Legacy|Sweep|Cancel|Admission)' -count=1 -json
```

JSON receipt: `/tmp/kandev-errand-terminal-compat-final-race.json`.

The final changed-package lint command exited 0 with `0 issues` after its warm package-load retry. Final gofmt and whitespace checks passed. The backend application build exited 0:

```bash
flock -w 300 /tmp/kandev-errands-repair-build.lock env GOCACHE=/tmp/kandev-errand-terminal-compat-go-cache GOMAXPROCS=2 GOMEMLIMIT=6GiB GOFLAGS='-p=2' make -C apps/backend build
```

Build output: `/tmp/kandev-errand-terminal-compat-build.log`. The Makefile reported that no codesign tool was installed for its macOS helper artifacts; it left those Go-signed targets as-is and exited successfully.

The changed-package lint command was `bash scripts/lint-go-changed` with the seven changed Go paths listed above. `gofmt -l`, `git diff --check` on tracked owned paths, and a trailing-whitespace scan of new test files produced no issues. No SQL guard, PostgreSQL, live database, or live runtime test was applicable or performed because persistence/schema code was unchanged.

## Source hashes

```text
bd6cc151131dc7c8ee54c680b89bcd0073dbbf7e0e05ec6f5e3d1b65c85556d1  apps/backend/internal/errands/service.go
a07869e0068b7e537da5668774d3385a1e61f90e9e814606e2a4195df99a67b0  apps/backend/internal/errands/teardown.go
00a2db0a2b3669b9560faecf738348398429bbdb98dfe94a1ce3c2d0037f4e48  apps/backend/internal/errands/teardown_compatibility_test.go
dc83d4ee451b9807b3cec844b3e8ec1f03bb6f1d2caba1a6499c2523b3c0d0bf  apps/backend/internal/orchestrator/errand_completion_cleanup.go
baf6beaab49c9acc4ad96ffbb6c9a1fc76b0f40f2b4ae49dd889e938bebe1538  apps/backend/internal/orchestrator/errand_teardown.go
6f6f2e4d6db205deffbe9d2520638c0adc2867d77814181f3a35c337d38cca97  apps/backend/internal/orchestrator/errand_teardown_integration_test.go
875d7e0dc9c8095c77e6b8baf9b70638d20c72d0e7d65fab89e84d6413e6be9c  apps/backend/internal/orchestrator/errand_terminal_compatibility_test.go
```

The shared build lock serialized every race, lint, and build process. Go compilation used `GOMAXPROCS=2`, `-p 1` for tests and a private cache; the build used `GOFLAGS=-p=2`. Every owned command exited before cache cleanup. After the concurrent panel-controls command released the shared lock, `go clean -cache` and `golangci-lint cache clean` removed the owned build and lint caches; Go left only its 12 KB README/trim metadata. Disk recovered from 12 GiB free (90% used) to 39 GiB free (66% used). No owned process or lock remains active.

## Parent integration boundary

No commits, pushes, live database edits, or runtime restarts were performed. The parent owns final integration, including the individual Stop interaction against this cleanup fix. That Stop/UI integration was not changed or revalidated in this bounded backend Errand.
