---
status: building
created: 2026-07-22
owner: kandev
---

# External VCS File Links

## Why

People reviewing or editing task files need a quick way to open the same file in its external repository and share that provider page with colleagues. Local worktree paths and Kandev-only diff views do not provide a portable collaboration link.

Decision: [ADR-2026-08-12-role-based-git-remotes](../../decisions/2026-08-12-role-based-git-remotes.md).

## What

- Task file and diff toolbars expose an `Open file in <provider>` action whenever Kandev can construct a valid external file URL for the file's repository and revision.
- The action appears across task Changes diffs, built-in file viewers and editors, mobile equivalents, and Review diffs. It opens the external provider page in a new browser tab without replacing the Kandev task.
- The first version supports GitHub, GitLab, and Azure DevOps. GitLab links preserve the configured host for self-hosted installations.
- Linked changes expose one provider-neutral identity pair: exact source repository/ref and exact canonical base repository/ref. GitHub, GitLab, and Azure adapters must populate both sides completely at their provider seam; generic-host, incomplete, mismatched, or cross-host identities fail closed.
- For a linked pull request or merge request, added, modified, untracked, and renamed-new content targets the exact source repository and head ref. Deleted content and renamed-old fallback target the exact canonical base repository and base ref. The attached task repository remains an association/authorization anchor and is not assumed to be either rendered side.
- When an attachment has multiple linked changes, the file's repository/worktree context and exact writable action head select the unique linked change whose source identity matches. Zero or multiple exact matches leave linked-change selection unresolved; Kandev never chooses by association order, change number, branch name alone, or canonical base identity.
- With no linked change, a published head-side file uses the worktree's exact writable action identity and a base-side file uses the accepted comparison identity. An unresolved linked-change selection does not fall through to an arbitrary attached repository or another linked base.
- Multi-repository tasks resolve repository, revision, and repository-relative file path from the file's own repository/worktree context. A file in one repository never links through another repository's provider metadata or published branch.
- GitHub links use the selected repository's credential-free web origin and `blob` route. GitLab links use its configured web origin and `-/blob` route. Azure DevOps links use its organization/project/repository web URL with file `path` and Git branch `version` query parameters.
- Provider URL components, revisions, and file paths are encoded without changing their semantic values. Generated links contain no embedded credentials, access tokens, executor-local remote names, or local filesystem paths.
- Added or untracked files require a known published writable source ref; Kandev does not link them to a base revision where they do not exist. Deleted files link to their base-ref version. Renamed files use the published new path when available, otherwise the base revision's previous path when that path is known.
- Complete source/base identity is replaced atomically when an authoritative linked-change refresh changes it. Lightweight provider updates that omit identity retain the last complete pair; an explicit unlink or authoritative clear removes both sides so stale source/base combinations cannot survive.
- The action is unavailable when the repository is local-only, its provider is unsupported, required provider metadata is missing, linked-change selection is unresolved, repository context is ambiguous, or no revision/path combination is expected to exist externally. Kandev does not guess an unknown provider's web URL shape.
- The action has an accessible provider-specific name and tooltip. On touch layouts it remains directly reachable, has at least a 44 px active dimension, and does not introduce document-level horizontal overflow.

## Failure modes

- If source repository/ref identity is absent or incomplete, Kandev omits head-only links instead of combining a source branch with the base or attached repository. Base-side files may still use a complete, uniquely selected canonical base identity.
- If base repository/ref identity is absent or incomplete, Kandev omits deleted/renamed-old links instead of combining a base branch with the source or attached repository.
- If multiple linked changes do not yield one exact source match for the file's worktree/action identity, Kandev marks selection unresolved and omits linked-change-derived links.
- If the provider is GitHub, GitLab, or Azure but its adapter cannot translate the complete provider-neutral repository/ref identity into that provider's URL shape, Kandev omits the action rather than trying another provider or the attached repository.
- If provider or repository metadata cannot produce a credential-free HTTPS web URL, the action is omitted rather than opening a malformed or sensitive URL.
- If a popup blocker prevents the new tab, Kandev remains on the current task surface and does not lose local state.

## Scenarios

- **GIVEN** a GitHub task with a linked pull request for the file's repository, **WHEN** the user activates the file action, **THEN** a new tab opens the file on the pull request's exact source repository and head branch.
- **GIVEN** a GitHub fork pull request whose source and canonical base repositories differ, **WHEN** the user opens an added or modified file externally, **THEN** the URL uses the source repository and head branch regardless of which repository is attached to the task.
- **GIVEN** that fork pull request contains a deleted file, **WHEN** the user opens it externally, **THEN** the URL uses the canonical base repository, base branch, and old path.
- **GIVEN** a task with two linked changes and only one source repository/ref matches the file's worktree action head, **WHEN** the user opens a changed file externally, **THEN** Kandev uses that change's exact source/base pair.
- **GIVEN** a task with multiple linked changes and no unique exact source match, **WHEN** a file toolbar renders, **THEN** no linked-change-derived action is shown rather than choosing the first change or a same-named branch.
- **GIVEN** a fork pull request lacks exact source-repository identity, **WHEN** an added file toolbar renders, **THEN** no external-file action is shown rather than constructing a base-repository URL with the head branch.
- **GIVEN** a self-hosted GitLab task with a linked merge request, **WHEN** the user activates the file action, **THEN** a new tab opens the `-/blob` file route on the exact configured GitLab source host/project and merge-request head branch.
- **GIVEN** an Azure DevOps linked change, **WHEN** the user opens a source-side or base-side file, **THEN** the URL uses the exact organization, project, repository, and ref for that side and never combines Azure query parameters with GitHub/GitLab identity.
- **GIVEN** an Azure DevOps task with no linked pull request, **WHEN** the user activates the file action for an existing base-side file, **THEN** a new tab opens that file using the accepted comparison repository/ref.
- **GIVEN** a multi-repository task whose repositories have different providers and base branches, **WHEN** the user opens a file from each repository, **THEN** each action uses that file's repository, repository-relative path, provider URL shape, and revision.
- **GIVEN** an added or untracked file with no published writable source branch, **WHEN** its toolbar renders, **THEN** no external-file action is offered.
- **GIVEN** a deleted file on a published task branch, **WHEN** the user activates its file action, **THEN** the external provider opens the file's base-ref version instead of a missing head-ref path.
- **GIVEN** a renamed file with a published source branch, **WHEN** the user activates its file action, **THEN** the external provider opens the new path on that source branch.
- **GIVEN** a renamed file without a published source branch but with a known previous path and complete base identity, **WHEN** the user activates its file action, **THEN** the external provider opens the previous path on the base ref.
- **GIVEN** an unsupported, local-only, ambiguous, or incompletely configured repository, **WHEN** a file or diff toolbar renders, **THEN** it does not show an external-file action or expose credentials/local paths.
- **GIVEN** a supported task file on a phone viewport, **WHEN** the user opens its file or diff surface, **THEN** the provider-specific action is visible, touch-reachable, opens the same external file target as desktop, and causes no horizontal page overflow.
- **GIVEN** a supported file in Changes, a built-in editor/viewer, and Review, **WHEN** each toolbar renders, **THEN** each surface offers the same provider-specific open action for that file context.

## Out of scope

- Copying the external URL directly from Kandev.
- Guessing routes for generic or unknown Git hosting providers.
- Publishing, pushing, or creating a pull request or merge request so a local-only file can be linked.
- Adding line-range anchors or linking directly to a diff hunk.
- Changing external repository permissions or making a private repository accessible to a colleague.

## Implementation plan

See [External VCS file links](../../plans/external-vcs-file-links/plan.md) for the original feature and [Role-aware Git remotes](../../plans/role-aware-git-remotes/plan.md) for source/base identity remediation.
