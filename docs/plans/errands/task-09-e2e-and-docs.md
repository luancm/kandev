---
id: "09-e2e-and-docs"
title: "Add errand E2E scenarios, public docs, and harness notes"
status: pending
wave: 7
depends_on:
  - "07-task-summary-indicators"
  - "08-errand-ui"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-003
  - REQ-TASKS-ERRANDS-004
  - REQ-TASKS-ERRANDS-005
  - REQ-TASKS-ERRANDS-006
  - REQ-TASKS-ERRANDS-007
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
acceptance_criteria:
  - AC-TASKS-ERRANDS-003.1
  - AC-TASKS-ERRANDS-003.5
  - AC-TASKS-ERRANDS-004.10
  - AC-TASKS-ERRANDS-004.11
  - AC-TASKS-ERRANDS-005.4
  - AC-TASKS-ERRANDS-006.1
  - AC-TASKS-ERRANDS-006.2
  - AC-TASKS-ERRANDS-006.3
  - AC-TASKS-ERRANDS-006.5
  - AC-TASKS-ERRANDS-006.6
  - AC-TASKS-ERRANDS-007.1
  - AC-TASKS-ERRANDS-007.5
  - AC-TASKS-ERRANDS-007.6
  - AC-TASKS-ERRANDS-010.2
  - AC-TASKS-ERRANDS-010.3
  - AC-TASKS-ERRANDS-010.4
  - AC-TASKS-ERRANDS-010.5
  - AC-TASKS-ERRANDS-010.6
  - AC-TASKS-ERRANDS-010.7
  - AC-TASKS-ERRANDS-010.8
  - AC-TASKS-ERRANDS-010.9
  - AC-TASKS-ERRANDS-011.1
  - AC-TASKS-ERRANDS-011.2
  - AC-TASKS-ERRANDS-011.3
  - AC-TASKS-ERRANDS-011.4
  - AC-TASKS-ERRANDS-011.5
system_design:
  - ../../specs/tasks/system-design/errands.md
---

# Task 09: Add errand E2E scenarios, public docs, and harness notes

## Summary

Prove the user-visible behavior end to end on desktop and phone with
deterministic mock-agent scenarios, document errands for users, and record
the new package, slice, flag, and metrics in the agent guidance files.

## In scope

- Mock agent predefined scenarios in `apps/backend/cmd/mock-agent/scenarios.go`:
  `/e2e:errands` emits text, starts three errands whose objectives are inline
  `e2e:` scripts, and ends its turn; `/e2e:errands-await` starts two errands
  and awaits them by the returned IDs. A reply to a user prompt that carries
  an errand status block states how many errands the block listed. Document
  both in `cmd/mock-agent/AGENTS.md`.
- `apps/web/e2e/tests/session/errands.spec.ts` (`chromium`) and
  `apps/web/e2e/tests/session/mobile-errands.spec.ts` (`mobile-chrome`), each
  enabling the flag with `backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" })`
  and using causal waits from `e2e/helpers/causal-waits.ts`. Scenarios are
  listed in the [plan E2E section](plan.md#e2e-tests).
- Public docs through `/docs-maintainer`: an Errands section in
  `docs/public/coordination.md` and the tool contract in
  `docs/public/agent-communication.md`, including the flag, limits, the
  conversational parent, the delivery routes, MCP server inheritance, and
  the shared-workspace caution. Upstream recently added the
  "Autopilot parent questions", "Show an image with an operator question",
  and "Show visual alternatives on choices" sections to
  `agent-communication.md` (#4581, #4752, #4766); merge the errand content
  around them instead of rewriting them.
- Guidance: root `AGENTS.md` Observability bullet for `errands_*` counters;
  `apps/backend/AGENTS.md` package structure entry for `internal/errands`;
  `apps/web/AGENTS.md` store slice entry for `errands`.
- After all checks pass, update this plan's verification results and the
  work-order statuses as the spec workflow prescribes.

## Out of scope

- New product behavior. A failing scenario that needs a behavior change goes
  back to the owning work order.

## Acceptance

1. The desktop spec proves start, live cards, no automatic tabs, an enabled
   composer whose prompt receives the status block, open, follow-up,
   **Errand finished** cards from a notification turn, a parent turn cancel
   that leaves errands running, **Stop all errands**, the board indicator,
   and the task staying in progress until the final parent turn.
2. The phone spec proves nesting, the full screen errand view, the actions
   menu, the chip drawer with **Stop all errands**, full-width finished
   cards, 44px targets, internal scrolling, and no horizontal overflow.
3. Public docs and agent guidance describe the shipped behavior, and the
   documentation and specification validators pass.

## ASCII UI preview

The specs assert the structure of [UI-01 to UI-10](plan.md#ascii-ui-preview),
covering AC-TASKS-ERRANDS-010.2 to 010.9 and 011.1 to 011.5.

```text
Desktop: parent cards -> composer prompt -> Open -> errand view [Stop]
         -> Errand finished card -> chip popover [Stop all errands]
Phone:   Sessions drawer -> errand (full screen) -> [.] Stop -> back
         -> chip drawer [Stop all errands]
```

## Verification

Run from the repository root. In a fresh worktree, first run
`(cd apps && pnpm install --frozen-lockfile)`. Run the two E2E commands
sequentially.

```bash
(cd apps/backend && go test -trimpath ./cmd/mock-agent -run 'Errand|Scenario' -count=1)
(cd apps/web && pnpm e2e:run --project chromium tests/session/errands.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-errands.spec.ts)
(cd apps/web && pnpm run e2e:sleep-ratchet)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/cmd/mock-agent/scenarios.go`, a scenario test, and
  `apps/backend/cmd/mock-agent/AGENTS.md`.
- New `apps/web/e2e/tests/session/errands.spec.ts` and
  `mobile-errands.spec.ts`, plus any page-object helpers under `apps/web/e2e/`.
- `docs/public/coordination.md`, `docs/public/agent-communication.md`.
- `AGENTS.md`, `apps/backend/AGENTS.md`, `apps/web/AGENTS.md`.
- `docs/plans/errands/plan.md` and work-order results.

## Dependencies

Tasks 07 and 08.

## Risks

- `backend.useEnv` restarts the managed backend; keep the flag scoped to these
  specs and restore it in cleanup so other specs keep the flag off.
- Errand objectives run through the mock agent's own script parser; keep the
  scripts short and deterministic to avoid timing-based waits.
- Notification turns start on their own with Auto-run on; assert them with
  causal waits on the delivered card, not on elapsed time.
- `docs/public/coordination.md` is already large, and
  `docs/public/agent-communication.md` keeps changing upstream; add focused
  sections, rebase before editing, and follow `docs/public/README.md`
  readability rules.

## Parallelism

`sequential`

## Inputs

- `/e2e`, `/mobile-parity`, `/docs-maintainer`, `apps/web/e2e/README.md`, and
  `apps/backend/cmd/mock-agent/AGENTS.md`.

## Results

Pending.
