---
id: "01-exact-tool-replay"
title: "Preserve exact tool provenance through terminal replay"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-ERRANDS-002
  - REQ-TASKS-ERRANDS-003
acceptance_criteria:
  - AC-TASKS-ERRANDS-002.4
  - AC-TASKS-ERRANDS-002.7
  - AC-TASKS-ERRANDS-003.1
  - AC-TASKS-ERRANDS-003.7
system_design:
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
---

# Preserve exact tool provenance through terminal replay

## Ownership and implementation boundary

Own the ACP tool tracking and prompt-end event producer in `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go` and `adapter_prompt.go`, exact-source persistence/correlation in `apps/backend/internal/task/repository/sqlite/agent_delivery_tool_source.go`, and focused lifecycle/orchestrator integration tests. Read the scoped backend and agentctl guidance first. Coordinate any shared model/schema extension through the existing durable delivery owner; do not introduce a second recovery coordinator.

First establish `TestPromptEndPreservesInitiallyTerminalTool` in the existing adapter test package. A completed initial generic tool call must remain completed and produce no spurious cancellation; an actually pending tool still receives the intended cleanup. Cover failed initial calls and Codex correlation controls. Preserve provider-normalized payload behavior.

Then record the exact admitted-tool provenance extension in the existing recovery design. Retain source session, incarnation, execution/admission, stream, harness/prompt generation, native identity, source sequence, turn/submission, tool and parent-tool identities atomically with source acceptance. Use a versioned internal effect if existing retained admission plus tool-source records cannot express it. Do not change received payload bytes or infer omitted prompt generation from current state. Historical reconstruction is allowed only from an exact retained source and matching admission. Absent, contradictory or multiply-owned source evidence must remain blocked, including after compaction. Reusing a tool ID under a new turn must not mutate either turn's output or grant terminal authority.

Add `TestErrandLateOwnedToolTerminalReplay` beside repository durable-delivery tests, reproducing the recorded source (completed owned tool), late missing-identity cancellation and correctly attributed complete. Assert cursor progression, original-turn attribution, no successor mutation and duplicate idempotency. Add an orchestrator boundary test proving one parent result and exact settlement without manually injecting a successful receipt. Use the real SQLite path, plus PostgreSQL for changed persistence semantics; include unknown/ambiguous/native-session/parent-tool/generation controls and ordinary-session positive controls.

## Acceptance and exclusions

1. The original adapter failure is RED before the change; completed calls are not spuriously cancelled and legitimate pending cleanup still works.
2. Exact retained evidence allows the following real terminal to settle once, while ambiguity and successor controls remain fenced. Compaction/restart behavior is explicit and tested.
3. Relevant race suites, SQL guard, store conformance and scoped lint pass; receipts record both dialect activation and any skips. No live repair is performed or claimed.

No UI changes, automatic resend, manual acceptance of uncertain effects, provider-wide certification, commit or deployment belongs to this work order.

## Verification

Run from repository root, retaining logs and process handles. Serialize heavy checks with the shared lock and inspect disk first. The commands below describe the original scope; exact executed selectors and final receipts are in the implementation handoff.

```bash
(cd apps/backend && flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOMEMLIMIT=3GiB GOFLAGS=-p=1 go test -race -trimpath -p 1 -parallel 1 ./internal/agentctl/server/adapter/transport/acp ./internal/task/repository/sqlite ./internal/orchestrator -run 'TestPromptEndPreservesInitiallyTerminalTool|TestErrandLateOwnedToolTerminalReplay' -count=1 -v)
(cd apps/backend && flock /tmp/kandev-errands-repair-build.lock env GOMAXPROCS=2 GOFLAGS=-p=1 go test ./internal/persistence/storeconformance -count=1 -v)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the repository SQL guard and changed-Go lint with the exact final paths according to `apps/backend/AGENTS.md`; run the affected existing adapter/durable ownership suites alongside the new regressions. Use only a disposable PostgreSQL cluster/schema. UI result delivery is validated through the existing orchestrator event projection; no rendered composition changes require a new browser fixture.

## Results

Complete in the primary session. Adapter RED/GREEN proves the producer defect and correction. SQLite/PostgreSQL replay and ownership controls pass; lifecycle/orchestrator tests prove one parent result and preserve ordinary Auto-run behavior. Store conformance, SQL guard, scoped lint, documentation catalog/spec checks and formatting pass. [The implementation handoff](implementation-handoff.md) records the initial fixture/lint corrections, exact source/receipt hashes and evidence limits. That source-only handoff preceded delivery. Integration and publication are complete in source `ee0f077cb9`; [the integration receipt](../errands-remaining/final-integration-handoff.md) and [current package](../errands-simplification/plan.md) own later checkpoints. No main-instance deployment or unidentified historical-record repair is claimed.
