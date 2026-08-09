---
id: "03-document-fork-pr-linking"
title: "Document remote-aware fork PR linking"
status: complete
wave: 3
depends_on: ["02-link-exact-remote-pr"]
plan: "plan.md"
spec: "../../specs/ui/ci-pr-automation.md"
---

# Task 03: Document remote-aware fork PR linking

## Acceptance

- The Git operations public guide states that Kandev follows the current
  branch's configured push target, falling back to its tracking target, when it
  associates an open GitHub PR.
- The guide treats `origin` for the contributor fork and `upstream` for the main
  repository as a recommended convention only; custom remote names work.
- The guide explains that Kandev does not guess among same-named branches in
  other forks and cannot use a PR outside workspace GitHub scope/access.
- Public documentation validation passes without navigation or link errors.

## Verification

- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`

## Files likely touched

- `docs/public/git-operations.md`
- `docs/plans/fork-pr-task-linking/plan.md`
- `docs/plans/fork-pr-task-linking/task-03-document-fork-pr-linking.md`

## Dependencies

Task 02 must establish the final tested remote-selection, scope, and ambiguity
behavior.

## Parallelism

Sequential. Public wording must match the implementation that Task 02 lands.

## Inputs

- The shipped behavior from Tasks 01 and 02.
- `docs/public/git-operations.md` under **Create a pull request or merge
  request**.
- The docs-maintainer requirement to keep Git and review-flow documentation
  current with user-visible behavior.

## Output contract

Report documentation files changed, exact validation results, any
link/navigation issues, and synchronized task/plan statuses in the primary
session.

## Results

Updated `docs/public/git-operations.md` with push/tracking remote selection,
arbitrary remote-name guidance, exact-head matching, and fail-closed
ambiguity/scope behavior. `node --test scripts/validate-public-docs.test.mjs`
passed all 58 tests, and `node scripts/validate-public-docs.mjs` validated all
41 published pages.
