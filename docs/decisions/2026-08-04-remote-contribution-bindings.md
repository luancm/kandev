# ADR-2026-08-04-remote-contribution-bindings: Bind Remote Contributions to Target Repositories

**Status:** accepted
**Date:** 2026-08-04
**Area:** backend, protocol, security, GitHub, GitLab

Extended by [ADR-2026-08-12-role-based-git-remotes](2026-08-12-role-based-git-remotes.md), which preserves this binding and authorization decision while superseding its literal `origin` and upstream-as-push routing clauses with provider-neutral repository roles.

## Context

`create_task_kandev` can resolve a repository URL, but a contributor pull request or merge request has two repository identities: the target repository that owns the change and the source repository and branch that must receive commits. GitHub also exposes pull-request refs that are readable but are not a stable writable branch. GitLab represents the same relationship with target and source projects.

Adding caller-supplied provider, change number, head branch, source repository, and push-remote fields would enlarge every MCP catalog and let an untrusted caller construct inconsistent or over-broad Git credential requests. Replacing the attached target with the source repository would instead break base-branch comparison, existing repository identity, CI/PR association, and normal task behavior.

## Decision

Remote change URLs are a semantic extension of the existing `create_task_kandev.repositories[].repository_url` value. The public input schema gains no properties.

The backend parses and resolves recognized URLs through the workspace's provider service. It persists a versioned, non-secret `remote_contribution` binding in the target `task_repositories.metadata`. The binding contains only provider-authored change identity, base/head refs and SHA, source repository identity and canonical credential-free remote URL, and collaboration eligibility. The target repository remains the attachment's normal `repository_id`; together with the provider-authored base ref it supplies the binding's exact target identity.

Runtime materialization preserves the attached target repository and adds whatever executor-local remote configuration is needed to reach the validated contribution source. A materializer may use `origin` for the target and a dedicated name for the source, but those names are implementation details. The target repository/ref is the comparison role, the source repository/ref is the writable action head, and an independently configured tracking upstream remains the only Pull role. Ordinary attachments use the same role resolver rather than receiving special `origin` semantics.

Managed GitHub credentials may add a source-repository lease scope only when it exactly matches a valid contribution binding attached to the authorized task and session. Executor-owned credentials are not broadened; they must pass a non-mutating source-branch push preflight before the agent starts. At mutation time, the exact source identity/ref must still match the binding and the current generation-bound writable-action observation. No token, lease, credentialed URL, remote name, or credential-helper detail is persisted.

GitHub PR and GitLab MR associations are created before launch. Provider title, body, comments, and diff content remain untrusted and are not copied into the initial prompt or trusted task context.

## Consequences

- MCP clients get the workflow with a minimal description-only catalog change.
- The provider, not the caller, is authoritative for source and target repository/ref identity.
- Target comparisons, review integration, and task association remain anchored to the attached repository without assigning semantic authority to `origin`.
- Fork pushes require a narrow second credential scope and explicit runtime routing to the validated writable source.
- Runtime and broker code must understand one versioned contribution binding and fail closed on unknown, stale, or inconsistent values.
- A task may persist while launch fails if executor-owned credentials cannot write the source branch; the task remains retryable after credentials or collaboration permissions are corrected.

## Alternatives considered

### Add `change_url` and provider-specific fields to the MCP tool

Rejected. A single new URL field would be modest, but supporting caller-supplied head/source details would grow the schema and create conflicting sources of truth. Even a lone `change_url` is unnecessary because `repository_url` already selects the repository materialized for the task.

### Attach the contributor fork as a second task repository

Rejected. The task would materialize two copies of substantially the same repository, confuse primary repository and PR association, and expose the fork as an independent workspace source rather than a narrow push destination.

### Replace the attached target with the contributor source

Rejected. Base comparisons, provider identity, authorization, and existing automation require the target to remain the durable task attachment. Executor-local remote names do not change that boundary.

### Checkout GitHub's `refs/pull/<number>/head` and push it

Rejected. The ref is suitable for reading the submitted head but is not the contributor's writable branch identity, and GitLab has no identical provider-neutral contract.

### Tell the agent to run provider CLI commands manually

Rejected. It postpones URL validation and permission failures until after work starts, exposes more provider-specific behavior to prompts, and cannot support deterministic resume or credential scoping.
