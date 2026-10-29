---
id: "09-e2e-and-docs"
title: "Add errand E2E scenarios, public docs, and harness notes"
status: completed
wave: 14
depends_on:
  - "13-inline-chat-summary"
  - "08-errand-ui"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-003
  - REQ-TASKS-ERRANDS-004
  - REQ-TASKS-ERRANDS-005
  - REQ-TASKS-ERRANDS-006
  - REQ-TASKS-ERRANDS-007
  - REQ-TASKS-ERRANDS-008
  - REQ-TASKS-ERRANDS-009
  - REQ-TASKS-ERRANDS-010
  - REQ-TASKS-ERRANDS-011
  - REQ-TASKS-ERRANDS-012
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
  - AC-TASKS-ERRANDS-008.1
  - AC-TASKS-ERRANDS-008.2
  - AC-TASKS-ERRANDS-008.3
  - AC-TASKS-ERRANDS-009.1
  - AC-TASKS-ERRANDS-009.2
  - AC-TASKS-ERRANDS-010.2
  - AC-TASKS-ERRANDS-010.3
  - AC-TASKS-ERRANDS-010.4
  - AC-TASKS-ERRANDS-010.5
  - AC-TASKS-ERRANDS-010.6
  - AC-TASKS-ERRANDS-010.7
  - AC-TASKS-ERRANDS-010.8
  - AC-TASKS-ERRANDS-010.9
  - AC-TASKS-ERRANDS-010.10
  - AC-TASKS-ERRANDS-011.1
  - AC-TASKS-ERRANDS-011.2
  - AC-TASKS-ERRANDS-011.3
  - AC-TASKS-ERRANDS-011.4
  - AC-TASKS-ERRANDS-011.5
  - AC-TASKS-ERRANDS-011.6
  - AC-TASKS-ERRANDS-012.1
  - AC-TASKS-ERRANDS-012.2
  - AC-TASKS-ERRANDS-012.3
system_design:
  - ../../specs/tasks/system-design/errands.md
---

# Task 09: Add errand E2E scenarios, public docs, and harness notes

## Summary

Prove the user-visible behavior end to end on desktop and phone with
deterministic mock-agent scenarios, document errands for users, and record
the package, slice, flag, and metrics in agent guidance. Record a real-provider
pilot before describing this first delivery as ready for daily use.

## In scope

- Deterministic mock-agent scenarios `/e2e:errands` and
  `/e2e:errands-await`, with a small `errands_scenarios.go` helper. Capture
  the real IDs returned by start calls and use them for await/follow-up.
  Current `script.go` remembers only plan versions, not arbitrary tool
  results; adding registry strings alone cannot implement these journeys.
  Echo the received status-block count and use causal file/event gates for
  long-running, permission, settlement and cancellation states. Do not add a
  general-purpose result-binding framework or fixed sleeps.
- `apps/web/e2e/tests/session/errands.spec.ts` (`chromium`) and
  `mobile-errands.spec.ts` (`mobile-chrome`). Reuse `backend.useEnv` and its
  existing restoration to enable the flag only for these tests. Share
  errands helpers/page-object methods between specs; keep fixtures disposable.
- Separate focused scenarios for conversational parent/open/follow-up/send
  result; real finished-card metadata live and after reload; Auto-run on/off
  and manual Send now; individual/bulk/queued cancellation; cap/admission;
  workflow hold then final advancement; two-client deletion; and retained
  sessions after disable/restart. Assert the actual wire-backed UI rather
  than injecting the desired message metadata into a store.
- Phone checks through the existing Sessions/Threads picker and errand view:
  visible back/actions, full-screen transcript, temporary bottom drawer,
  fixed Stop-all footer and one scrolling list. Assert real 44px targets,
  viewport containment, safe-area clearance, dismissal/focus and no page
  horizontal overflow; include a narrow fine-pointer check at the phone
  breakpoint where shared controls branch.
- Inline current-summary cases from task 13: default collapse, live header
  updates, grouped expansion, independent Open/Stop-all actions, shared
  chip/list state, reading-position and bottom-follow behavior. Keep historical
  start/result cards and neighboring diff/todo widgets; test reload and mobile
  hit geometry. Existing changed-files scroll suites guard the shared host.
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
- An opt-in pilot on one installed structured profile and an existing
  attach-capable executor: start two disjoint errands, end the parent turn,
  send a parent prompt while they run, open/steer a child, receive results,
  exercise turn cancel versus Stop, then disable/restart and inspect retained
  sessions. Record provider/model/executor, exact revision, observed results,
  failures and rollback. Use disposable isolated data; do not alter the
  developer's main instance or publish user task contents.
- After every continuation work order and its checks pass, record results
  and promote draft requirements to `active`, draft design to `current`,
  and plan to `implemented` only if the final behavior matches. If pilot
  execution is unavailable, record it as pending and keep readiness open.

## Out of scope

- New product behavior. A failing scenario that needs a behavior change goes
  back to the owning work order.

## Acceptance

1. The desktop spec proves start, live cards, no automatic tabs, an enabled
   composer whose prompt receives the status block, open, follow-up,
   **Errand finished** cards from a notification turn, a parent turn cancel
   that leaves errands running, **Stop all errands**, the board indicator,
   and the task staying in progress until the final parent turn. It also
   proves real snapshot persistence, Auto-run/manual dispatch, queued/cap
   handling, two-client deletion, flag-off retained-session behavior, and
   UI-11 inline-summary interaction alongside existing diff/todo cards.
2. The phone spec proves nesting, the full screen errand view, the actions
   menu, the chip drawer with **Stop all errands**, full-width finished
   cards, 44px targets, internal scrolling, and no horizontal overflow.
   UI-11 expands inline under the same chat scroller and preserves Open/back.
3. Public docs and guidance describe the proven behavior, documentation
   validators pass, and the selected provider/executor pilot records its
   outcome and disable/restart rollback. Unexecuted certification remains
   explicit; no claim of universal provider support is made.

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
`(cd apps && pnpm install --frozen-lockfile)` (task 00 normally did this).
Confirm discovery, then run the two managed E2E commands sequentially. They
build the production web/backend assets; do not use stale `--no-build` output.

```bash
(cd apps/backend && go test -trimpath ./cmd/mock-agent -run 'Errand|Scenario' -count=1)
(cd apps/web && pnpm e2e:raw --project chromium e2e/tests/session/errands.spec.ts --list)
(cd apps/web && pnpm e2e:raw --project mobile-chrome e2e/tests/session/mobile-errands.spec.ts --list)
(cd apps/web && pnpm e2e:run --project chromium tests/session/errands.spec.ts tests/git/turn-changed-files-scroll.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-errands.spec.ts tests/git/mobile-turn-changed-files-scroll.spec.ts)
(cd apps/web && pnpm run e2e:sleep-ratchet)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/cmd/mock-agent/scenarios.go`, new `errands_scenarios.go`
  and `errands_scenarios_test.go`, and
  `apps/backend/cmd/mock-agent/AGENTS.md`.
- New `apps/web/e2e/tests/session/errands.spec.ts` and
  `mobile-errands.spec.ts`, plus any page-object helpers under `apps/web/e2e/`.
- `docs/public/coordination.md`, `docs/public/agent-communication.md`.
- `AGENTS.md`, `apps/backend/AGENTS.md`, `apps/web/AGENTS.md`.
- `docs/plans/errands/plan.md` and work-order results.

## Dependencies

Tasks 13 and 08, transitively 00, 10, 11, 12 and 07. A product failure returns to
its owning work order; do not bypass its unit/contract evidence inside E2E.

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

Implementation checkpoint, 2026-10-29: deterministic mock scenarios call
real MCP start/await tools with returned IDs and causal child gates. Focused
mock-agent tests passed. Desktop, phone and opt-in real-provider specs exist;
discovery finds eight desktop and four phone cases including changed-files
scroll regressions. The fresh managed backend/helpers and production web
build passed. The initial browser attempt could not launch the missing
Playwright revision; Chromium 1228 is now installed. The next desktop run
passed changed-files scrolling and cap/cancellation but exposed delivery and
await defects, now fixed with focused regressions. Browser reproof on current
upstream and provider acceptance remain pending. Public coordination/tool guidance and scoped ownership/metrics
notes are authored; harness, specification, catalog and public-doc source
validators pass. The separate Landing publication build is unavailable in
this checkout and is not claimed.

## Pilot and conditional store evidence

Use the user's selected installed profile and executor in a disposable
instance. Record whether the store is SQLite or PostgreSQL. The first personal
pilot does not certify every provider, remote executor, or SQL backend.
Before a PostgreSQL pilot or broader support claim, run the existing env-gated
repository tests with a disposable database; a skip is not a pass:

```bash
(cd apps/backend && test -n "${KANDEV_TEST_POSTGRES_DSN:-}" && go test -trimpath ./internal/task/repository/sqlite -run 'Errand' -count=1)
```

If the pilot cannot be run with available credentials/runtime, leave its result
pending with the exact missing prerequisite. Do not ask for credentials during
planning or modify the live installation to manufacture evidence. No upstream
PR or release work is included; a later contribution needs refreshed
`upstream/main` integration and its own explicit publication request.

Acceptance checkpoint, 2026-10-29: on merged upstream `b767ca83fd37`, the
focused backend and frontend checks, typecheck, changed-file lint, i18n and
managed desktop/phone browser journeys passed. The selected `gpt-6-luna`
Codex ACP pilot passed in a disposable SQLite worktree instance, including
flag-off restart. See the current delivery checkpoint in `plan.md` for exact
scope, merge, evidence and remaining broader-support limitations.
