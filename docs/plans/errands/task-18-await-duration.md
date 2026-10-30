---
id: "18-await-duration"
title: "Show elapsed wait in errands chat entries"
status: done
wave: 19
depends_on:
  - "17-panel-order"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRAND-AWAIT-DURATION-001
acceptance_criteria:
  - AC-TASKS-ERRAND-AWAIT-DURATION-001.1
  - AC-TASKS-ERRAND-AWAIT-DURATION-001.2
  - AC-TASKS-ERRAND-AWAIT-DURATION-001.3
  - AC-TASKS-ERRAND-AWAIT-DURATION-001.4
system_design:
  - ../../specs/tasks/system-design/errand-await-duration.md
---

# Task 18: Show how long the await took

## Scope and acceptance

Measure actual await duration in the backend and carry it through the existing result to the completed chat card, with localized accessible display on desktop and phone. Show the actual end reason (all ready, any ready, timeout or parent ended) and per-errand result-returned state only with evidence that this await returned it. Immediate zero, timeout and parent-ended successful waits are covered. Legacy/malformed results omit the label. Preserve existing list, error, navigation and delivery behavior. No polling or new control. Likely ownership: internal/errands/await.go and adjacent tests, internal/mcp/handlers/errands.go and tests, MCP server result propagation tests, errand-parse.ts and tests, errand-renderers.tsx and tests, relevant locale catalogs and existing desktop/phone E2E fixtures. Public tool documentation must describe the additive duration field.

## Preview

```text
Waited for 2 errands  ·  12s  ·  All finished
  Audit release checklist       Done · Result returned
  Review support rota           Done · Result returned
```

On phone the same compact card stays in the transcript and wraps without horizontal overflow. Existing child Open targets and transcript scroller remain unchanged.

## Verification

Use test-first coverage of measured, immediate, timed-out and ended waits; serialization, all end-reason branches and legacy missing/invalid data; and desktop/phone reload rendering. From repository root:

```bash
(cd apps/backend && go test ./internal/errands ./internal/mcp/handlers ./internal/mcp/server -run 'Await|Errand' -count=1)
(cd apps/web && pnpm exec vitest run components/task/chat/errands/errand-parse.test.ts components/task/chat/messages/kandev/errand-renderers.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project chromium e2e/tests/session/errands.spec.ts -- --retries=0
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome e2e/tests/session/mobile-errands.spec.ts -- --retries=0
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run changed-Go and exact-file TypeScript lint. Use existing deterministic fixture to prove actual wait and reload rather than sleeping arbitrary time; ensure additive field survives MCP transport and ACP transcript persistence before marking done. Follow task17 to avoid concurrent edits to shared specs and browser fixtures.


## Results

Actual monotonic await duration and explicit end reason flow through the existing MCP response and appear in completed chat entries, with result-returned indicators only for results carried by the await. Legacy/malformed values are omitted. Backend Await/Errand selectors, handler serialization and focused outcome tests pass; frontend parser/renderer tests pass 31/31. Typecheck, changed-Go and TypeScript lint, i18n checks, public docs checks, specification validation and diff check pass. Desktop await including reload passed at `/tmp/kandev-run.e2e.AqjF7hcH.log`; native phone suite 6/6 including elapsed, outcome, returned result and reload passed at `/tmp/kandev-run.e2e.YzSKukDM.log`. Broader task16/17 mixed-parent browser proof and dark recapture remain separate outstanding work.
