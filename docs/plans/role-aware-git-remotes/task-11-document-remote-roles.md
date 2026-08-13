---
id: "11-document-remote-roles"
title: "Document role-aware Git behavior"
status: pending
wave: 11
depends_on: ["10-cover-desktop-mobile-role-parity"]
plan: "plan.md"
spec: "../../specs/platform/git-remote-roles.md"
---

# Task 11: Document role-aware Git behavior

## Acceptance

- Public Git-operation documentation defines attached repository, writable action head, tracking upstream, and comparison target without assigning special global meaning to `origin`.
- Public session/review documentation uses the same role vocabulary for Changes, comparison bases, Push, Pull, and provider change-request creation; it does not promise that `origin` is the source or target.
- Existing public references in `sessions-and-review.md`, `automation-and-mcp.md`, and `integrations.md` are reviewed and updated wherever they assign semantic authority to `origin`, a provider default, or an attached repository.
- Document Push, Pull, Rebase, Merge, Create PR/MR, sidebar comparison counts, arbitrary remote names, first-push behavior, and unresolved/ambiguous fail-closed behavior.
- Describe remote-contribution checkout as one validated materialization strategy, not as the universal remote convention.
- Update scoped agent guidance only where implementation changed package ownership, runtime transport, or frontend state conventions. Do not duplicate the public guide in agent instructions.
- Complete and record the public-docs impact assessment required by the docs-maintainer workflow.

## Verification

```bash
git diff --check -- docs/public/git-operations.md docs/public/sessions-and-review.md docs/public/automation-and-mcp.md docs/public/integrations.md apps/backend/internal/agentctl/AGENTS.md apps/web/AGENTS.md docs/plans/role-aware-git-remotes/task-11-document-remote-roles.md
rg -n 'attached repository|writable|tracking|comparison|Push|Pull|Rebase|Merge|Create' docs/public/git-operations.md docs/public/sessions-and-review.md
(node --test scripts/validate-public-docs.test.mjs)
(node scripts/validate-public-docs.mjs)
```

## Files likely touched

- `docs/plans/role-aware-git-remotes/task-11-document-remote-roles.md`
- `docs/public/git-operations.md`
- `docs/public/sessions-and-review.md`
- `docs/public/automation-and-mcp.md`
- `docs/public/integrations.md`
- `apps/backend/internal/agentctl/AGENTS.md`
- `apps/web/AGENTS.md`

## Dependencies

Task 10 fixes the final observable desktop/mobile behavior that documentation describes.

## Parallelism

Sequential after behavior and E2E semantics stabilize.

## Output contract

Update only this task file's `## Results`. Report the public behavior documented, scoped guidance changes, docs-impact conclusion, files changed, and exact command results to the primary. Do not edit `plan.md` or another task file.

## Results

Public docs updated:

- `docs/public/git-operations.md` now defines attached repository, writable action head, tracking upstream, and comparison target; routes Push, Pull, Rebase, Merge, and Create PR/MR by role; documents arbitrary remote names, first-push evidence, comparison counts, and fail-closed unresolved/ambiguous behavior.
- `docs/public/sessions-and-review.md` applies the same roles to Changes, comparison bases, sidebar counts, Push/Pull, external file links, and provider change-request creation without assigning semantic authority to `origin`.
- `docs/public/automation-and-mcp.md` documents contribution source/target roles and describes target-plus-source remote checkout as one validated materialization strategy, not a universal remote convention.
- `docs/public/integrations.md` distinguishes provider authorization and defaults from Git remote roles, removes semantic `origin` assumptions from GitHub/GitLab task Git guidance, and links to the Git operations role reference.

Scoped guidance was not changed: this documentation task changes no package ownership, runtime transport, or frontend state convention. Docs impact: public behavior and user-facing Git terminology changed, so the four existing public guides were updated; no new page or `meta.json` entry was needed. The pages remain how-to/reference guidance and link to the central role reference rather than duplicating it in agent instructions.

Validation:

- `git diff --check -- docs/public/git-operations.md docs/public/sessions-and-review.md docs/public/automation-and-mcp.md docs/public/integrations.md apps/backend/internal/agentctl/AGENTS.md apps/web/AGENTS.md docs/plans/role-aware-git-remotes/task-11-document-remote-roles.md`: passed.
- `rg -n 'attached repository|writable|tracking|comparison|Push|Pull|Rebase|Merge|Create' docs/public/git-operations.md docs/public/sessions-and-review.md`: passed; role and operation references present.
- `node --test scripts/validate-public-docs.test.mjs`: passed, 61 tests.
- `node scripts/validate-public-docs.mjs`: passed, 41 published docs pages.

Files changed: the four public guides above and this task's Results section. No scoped AGENTS file changed.
