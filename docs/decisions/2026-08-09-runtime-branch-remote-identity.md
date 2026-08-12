# ADR-2026-08-09-runtime-branch-remote-identity: Separate Runtime Branch Remote Identity From Task Repository Identity

**Status:** accepted
**Date:** 2026-08-09
**Area:** backend, protocol, GitHub, security

Extended by
[ADR-2026-08-12-role-based-git-remotes](2026-08-12-role-based-git-remotes.md), which places this
attached-repository/runtime-head split inside Kandev's provider-neutral remote-role model.

## Context

Kandev persists a normalized provider identity for every attached repository.
That identity is a durable authorization and association boundary, but it does
not always identify the repository that owns the current branch. A contributor
may attach a main repository and push through a fork remote, or attach a fork
whose pull request targets the main repository. Git remote names are also user
controlled; `origin` and `upstream` describe a common convention rather than
stable roles.

Branch-only pull request discovery currently searches by the attached
repository and branch name. GitHub stores a cross-fork pull request under its
base repository, so the search can miss the pull request or confuse it with a
same-named branch in another fork. Reading local Git configuration in the
orchestrator is not portable because Docker, SSH, and other executors own the
checkout, while replacing the persisted repository identity from mutable
runtime configuration would weaken the existing authorization boundary.

## Decision

The persisted task repository remains the durable authorization and
association anchor. Runtime Git configuration may contribute an observed
branch-head identity, but it never overwrites or backfills the attached
repository's provider, host, owner, name, or remote URL.

Agentctl resolves the current branch's configured push ref, falling back to its
upstream tracking ref when no distinct push ref is available. It derives the
referenced remote under its actual configured name, prefers that remote's
configured push URL and falls back to its fetch URL, and emits only a normalized
provider, host, owner, repository, and remote branch identity. Raw remote URLs
and credentials do not cross the git-status stream. Invalid, local-only,
unsupported, or conflicting multi-push inputs fail to an absent runtime
identity rather than a partially trusted value.

GitHub branch-only discovery combines the attached repository anchor, the
observed head repository, and the exact branch ref. The provider query remains
anchored to the attached repository and considers both pull requests based in
that repository and pull requests associated with its exact ref. A candidate is
accepted only when its base or head is the attached repository, its head
repository and branch match the observed identity, and its base repository is
within the workspace GitHub scope and automation principal's access. Duplicate
representations of one pull request are collapsed; multiple eligible pull
requests fail closed.

A searching pull request watch durably stores the normalized observed head
identity and exact remote branch, separately from the local branch that
identifies the task watch. When discovery succeeds, Kandev atomically records
the pull request number and changes the watch's owner and repository to the
pull request's canonical base repository for subsequent numbered status
checks. The watch's task `repository_id` and observed head identity remain
unchanged, preserving the association and audit trail across restarts.

When no usable runtime identity exists, discovery treats the attached
repository as the expected head. It may preserve direct-repository behavior or
find a pull request associated with that attached exact ref, but it never
selects another fork from branch name alone.

`origin` for a local contributor fork and `upstream` for the main repository is
the recommended soft convention. No backend behavior depends on either name.

## Consequences

Fork pull requests can be linked from custom remote layouts without weakening
workspace repository scope. Remote executors report the identity from the
checkout they actually run, while the orchestrator remains independent of
their filesystem. Searching watches gain additive head-identity fields and the
git-status protocol gains credential-free remote-identity fields. Discovery
and watch resolution require explicit ambiguity, host, scope, and atomic-update
tests.

Changing a runtime remote may change which head Kandev observes for a searching
watch, but it does not rewrite repository history or already resolved pull
request identity. A branch with no normalizable remote may not auto-link a
cross-fork pull request until its push or tracking target is configured.

## Alternatives Considered

- Search only the attached repository. Rejected because an attached fork does
  not own a pull request whose base is the main repository.
- Scan every configured remote and choose a matching branch. Rejected because
  remotes may be unrelated, names are arbitrary, and multiple matches create
  an unsafe implicit selection.
- Assign fixed meaning to `origin` and `upstream`. Rejected because users and
  tooling legitimately use different naming and triangular push conventions.
- Read `.git/config` from the orchestrator on demand. Rejected because the
  checkout may exist only inside a remote executor.
- Replace the persisted repository identity with the current remote. Rejected
  because mutable executor state must not reinterpret task authorization,
  history, or provider associations.
