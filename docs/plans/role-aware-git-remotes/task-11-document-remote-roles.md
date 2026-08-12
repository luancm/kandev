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

Pending.
