---
spec: docs/specs/ui/ci-pr-automation.md
created: 2026-08-09
status: complete
---

# Implementation Plan: Link fork pull requests to tasks

## Overview

Make GitHub branch-only PR discovery use the exact repository and remote branch
that Git reports as the current branch's push target, falling back to its
upstream tracking target. The attached task repository remains the
authorization anchor, while the runtime remote supplies a credential-free head
identity. Remote names are arbitrary: `origin` for a contributor fork and
`upstream` for the main repository is a soft convention only.

This is a backend and agentctl protocol change. No branch-picker UI work is
included; the existing branch records already retain their remote name, and the
value of grouping or filtering those branches should be evaluated separately.

## Confirmed root cause

Kandev persists the provider identity derived during repository discovery and
uses it for `FindPRByBranchForWorkspace`. In a fork workflow, that durable
identity can describe the attached fork while GitHub stores the pull request
under the main repository that owns its base. The PAT GraphQL path and the
GitHub CLI path therefore inspect pull requests based in the attached fork and
leave the watch at `pr_number=0`.

The smallest service-path reproduction used an attached `myorg/myrepo` fork,
branch `feature-branch`, and a PR based in `upstream/myrepo` whose head was
`myorg/myrepo:feature-branch`. The throwaway
`TestCheckSessionPR/finds_fork_PR_opened_against_upstream` failed with
`expected fork PR against upstream to be found` and was removed after
diagnosis. This checkout independently confirms the topology: `origin` is
`luancm/kandev`, `upstream` is `kdlbs/kandev`, and GitHub's
`Ref.associatedPullRequests` returns both base and exact head repository
identities.

The first draft treated the attached repository as the head. That is not
general enough: users may attach the main repository and push through a fork,
rename either remote, configure a distinct push URL, or push a local branch
under another remote ref. The runtime checkout is the authority for that
mutable head observation, but it must not reinterpret the task's persisted
repository identity.

## Architecture boundary

[ADR-2026-08-09-runtime-branch-remote-identity](../../decisions/2026-08-09-runtime-branch-remote-identity.md)
records the split:

- `repositories.provider_*` and the task `repository_id` remain the durable
  authorization and association anchor.
- Agentctl observes the branch's configured push target, falling back to the
  upstream tracking target, because the checkout may live inside a remote
  executor.
- Only normalized provider, host, owner, repository, and remote branch fields
  cross the git-status stream. Raw remote URLs and credentials do not, and no
  downstream component needs to interpret the remote name.
- A configured remote may narrow exact-head discovery but cannot expand the
  workspace GitHub repository scope or automation principal's access.
- Literal remote names have no semantics. Conflicting push targets fail closed.

## Backend and protocol

### Observe the exact runtime head ref

- `apps/backend/internal/agentctl/server/process/workspace_git_status.go`:
  resolve the current branch's Git push remote and remote ref using Git's own
  branch/push configuration, then fall back to the upstream remote/ref. Read
  `remote.<name>.pushurl` first and `remote.<name>.url` second. Normalize all
  configured push URLs and accept them only when they identify the same
  provider repository; otherwise emit no head identity. Preserve the existing
  `RemoteBranch` and ahead/behind behavior for push-state compatibility.
- Extend and reuse the existing agentctl provider/remote parsing helpers rather
  than adding another URL grammar in lifecycle or the orchestrator. Strip
  credentials and never expose the raw configured URL in status, lifecycle,
  logs, or persistence.
- Add a small credential-free runtime-head shape to
  `apps/backend/internal/agentctl/types/streams/git.go`, carry it through
  `apps/backend/internal/agent/runtime/agentctl/git.go`,
  `apps/backend/internal/agent/runtime/lifecycle/event_types.go`, and
  `apps/backend/internal/agent/runtime/lifecycle/events.go`, and include it in
  the orchestrator's git-status event data.
- Keep local branch and remote head branch distinct. A custom refspec or
  triangular workflow may push `local-feature` as `review-feature`; PR lookup
  must use `review-feature`, while watch uniqueness and branch-switch handling
  continue to use `local-feature`.

### Persist the searching head and resolve the canonical PR

- `apps/backend/internal/github/models.go` and
  `apps/backend/internal/github/store.go`: add `head_host`, `head_owner`,
  `head_repo`, and `head_branch` to `github_pr_watches` with empty defaults for
  existing rows. An empty head identity falls back to the attached
  owner/repository and local branch, preserving direct-repository behavior.
- Add one guarded store operation that updates branch plus runtime head fields
  only while `pr_number=0`, retaining the existing collision semantics for
  `(session_id, repository_id, branch)`. Git-status handling uses it so a
  changed remote is durable before the next background poll.
- Replace number-only watch resolution with an atomic operation that sets the
  PR number and canonical base owner/repository only while the watch is still
  searching. This prevents a numbered watch from briefly querying the attached
  fork with an upstream PR number and prevents a later status event from
  rewriting a resolved watch.
- Keep `repository_id` and the observed head fields after resolution. They
  record which attached repository and exact runtime ref produced the task PR,
  while `owner`, `repo`, and `pr_number` address GitHub's canonical base PR.

### Query by attached anchor and exact observed head

- Change the GitHub branch-lookup input from `(owner, repo, branch)` to an
  explicit attached repository plus expected head host/owner/repository/branch.
  The workspace service validates the attached repository first and rejects a
  discovered base outside workspace scope or automation-principal access.
- `apps/backend/internal/github/graphql.go`: query both the attached base
  repository's `pullRequests(headRefName:)` connection and the attached
  repository's `ref(qualifiedName:) { associatedPullRequests(states: OPEN) }`.
  Carry each candidate's base `repository.nameWithOwner`, head
  `headRepository.nameWithOwner`, and head ref name. Accept only candidates
  connecting the attached repository to the exact expected head ref, dedupe by
  canonical base repository plus PR number, and return only one unambiguous
  result.
- Preserve the branch-only optimization: discovery does not paginate review
  threads. The first numbered status sync remains responsible for complete
  review state.
- `apps/backend/internal/github/pat_client.go` continues through the shared
  GraphQL query. `apps/backend/internal/github/gh_client.go` routes the named
  GitHub CLI client through the same query using its existing GraphQL executor
  instead of `gh pr list`, keeping PAT, App/token, and CLI behavior identical.
- `apps/backend/internal/orchestrator/event_handlers_git.go` and
  `apps/backend/internal/orchestrator/event_handlers_github.go`: persist the
  observed head on searching watches, pass it through immediate push detection,
  and use the watch's exact head for on-demand and background discovery. Every
  successful path resolves the watch to `pr.RepoOwner`, `pr.RepoName`, and the
  PR number before status sync and task association.

### QA repair: preserve the head on immediate association

The initial implementation left one path incomplete: when push detection
found a PR before a watch existed, `associatePRFromPushScoped` created an
already-numbered watch without the runtime head fields. Preserve the exact
head in that creation operation and pass it from push detection; on-demand
association must preserve the watch's existing head as well. Do not add
`is_fork` or parent-repository metadata: the runtime push target and GitHub's
canonical PR result remain the sources of truth.

## Tests

- **Runtime remote selection:** custom remote names, push target preferred over
  upstream, push URL preferred over fetch URL, a renamed remote branch, and
  conflicting multi-push URLs. **Files:** agentctl workspace git-status tests.
- **Credential safety and propagation:** HTTPS credentials never appear in the
  emitted shape, invalid/local-only targets yield an empty identity, and all
  lifecycle hops preserve valid normalized fields. **Files:** agentctl process
  and lifecycle event tests.
- **Watch persistence:** additive migration/default behavior, guarded searching
  target updates, branch-collision behavior, and atomic transition to canonical
  base owner/repository plus number. **Files:** GitHub store tests.
- **Exact GraphQL matching:** attached fork to main, attached main to exact
  contributor fork, custom remote names, sibling fork ignored, duplicate
  candidate dedupe, ambiguous exact-ref rejection, and base-scope/access
  rejection. **Files:** GitHub GraphQL, service, PAT, and CLI tests.
- **End-to-end backend association:** git status updates a searching watch;
  immediate and polled discovery use the persisted head; a found upstream PR is
  subsequently queried under its base repository. **Files:** orchestrator and
  GitHub watch-service tests.
- **Immediate resolved-watch regression:** a push found before watch creation
  stores the exact runtime head with the canonical base and PR number, and the
  persisted head survives a terminal reset. **Files:** orchestrator and GitHub
  watch-service tests.

Implementation follows RED-GREEN-REFACTOR for each task. Permanent regression
tests land with the production change; the diagnostic throwaway test does not
return.

## Public documentation

- `docs/public/git-operations.md`: document that Kandev follows the current
  branch's configured push/tracking remote when associating a GitHub PR, that
  remote names may differ, and that `origin` fork / `upstream` main is only a
  recommended convention.
- Explain that same-named branches in other forks are not guessed and that an
  out-of-scope or ambiguous PR remains unlinked.
- Validate the public docs with the repository's documentation checks.

## E2E tests

No browser E2E is planned. The existing linked-PR event and rendering contract
do not change, and the branch-picker remote grouping/filtering idea is outside
this fix. Backend protocol, persistence, service, and orchestrator tests cover
the changed behavior.

## Verification results

The diagnostic RED reproduction failed as expected and was removed. Permanent RED/GREEN tests cover agentctl runtime-head selection, lifecycle propagation, watch persistence, exact GraphQL/PAT/CLI matching, service scope checks, and immediate/polled orchestrator association. `cd apps/backend && go test -tags fts5 ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle ./internal/agentctl/server/api ./internal/agent/runtime/agentctl ./internal/github ./internal/orchestrator -count=1` passed. `node --test scripts/validate-public-docs.test.mjs` passed all 58 tests; `node scripts/validate-public-docs.mjs` validated 41 pages; `git diff --check` passed; and no temporary fixtures remain.

A full `go test -tags fts5 ./...` sweep reached unrelated existing
`internal/task/service` filesystem-environment failures in repository handler
tests; all packages covering this change remained green.

## Implementation waves and parallel candidates

Wave 1 (sequential):

- [x] [task-01-stream-runtime-head-ref](task-01-stream-runtime-head-ref.md)

Wave 2 (sequential, after the protocol contract is available):

- [x] [task-02-link-exact-remote-pr](task-02-link-exact-remote-pr.md)

Wave 3 (sequential, after behavior is final):

- [x] [task-03-document-fork-pr-linking](task-03-document-fork-pr-linking.md)

Wave 4 (sequential, QA repair):

- [x] [task-04-preserve-immediate-watch-head](task-04-preserve-immediate-watch-head.md)

No tasks are marked parallel-safe. Task 02 consumes Task 01's status contract,
and public wording must describe the final tested behavior. Shared spec, plan,
and status updates remain in the primary conversation.

## Risks

- Git may expose multiple push URLs. They must normalize to one repository or
  discovery must fall back safely instead of choosing by order.
- A remote name or branch can contain path separators. Use Git's remote/ref
  metadata rather than splitting `remote/branch` heuristically.
- A status update can race PR discovery. Searching-target updates and canonical
  watch resolution must both be guarded by `pr_number=0`.
- GitHub may report the same PR through both query connections, or one exact
  head ref may have multiple open PRs against different bases. Deduplicate the
  former and reject the latter.
- A GitHub CLI query must preserve deterministic host/login selection,
  cancellation, and credential isolation when moving from `gh pr list` to the
  shared GraphQL executor.
- A returned base repository may be outside workspace scope or App installation
  access. Runtime Git configuration must not make it eligible.

## Out of scope

- Rewriting repository persistence or changing configured Git remotes.
- Assigning required semantics to `origin` or `upstream`.
- Grouping, labeling, or filtering the branch picker by remote.
- Linking closed or merged PRs during initial branch-only discovery.
- Adding an agent MCP tool for explicit PR linking.
- Changing GitLab merge-request discovery or task PR automation after a PR is
  linked.

## Open questions

None.
