# ADR-2026-08-12-role-based-git-remotes: Resolve Git Remotes By Role And Repository Identity

**Status:** accepted
**Date:** 2026-08-12
**Area:** backend, frontend, protocol, GitHub, GitLab, Azure DevOps

## Context

Kandev currently gives the literal remote name `origin` several unrelated meanings. Git status uses it to find the comparison base, Push and change-request creation use it as the writable destination, and some fallback paths treat it as the provider repository. The fork-PR work correctly separated the attached repository from the current branch's push destination, while upstream change `c4b4791de` separately added a coherent snapshot of the branch's configured tracking upstream. In a triangular workflow those identities differ, so combining them produces incorrect sidebar diff counts, unsafe or disabled remote actions, and provider-specific abstractions that do not fit GitLab namespaces or Azure repository identities.

Git remote names are controlled by each checkout and can differ across local, Docker, SSH, and other executors. A boolean such as `isFork` would describe one GitHub topology without identifying the repository or ref required by Git status, mutation, and provider operations.

This decision extends [ADR-2026-08-09-runtime-branch-remote-identity](2026-08-09-runtime-branch-remote-identity.md) and [ADR-2026-08-04-remote-contribution-bindings](2026-08-04-remote-contribution-bindings.md). It preserves their attached-repository authorization boundary and contribution binding, while superseding any clause that gives semantic authority to `origin`, treats the tracking upstream as the Push destination, or derives one repository role from another.

## Decision

Kandev models four independent repository roles:

| Role | Authority | Purpose |
| --- | --- | --- |
| Attached repository | Persisted task repository identity | Authorization, workspace scope, and task association |
| Writable action head | Git's resolved push destination for the current checkout | Push, Force Push, provider source-ref identity, and source-history safety |
| Comparison target | Linked change base, remote-contribution target, or attached repository plus selected base ref | Cumulative diff, sidebar counts, Rebase, Merge, and change-request target |
| Tracking upstream | Explicit Git `@{upstream}` for the current local branch | Pull and tracking-divergence evidence |

Roles may identify the same repository and ref, but no role implies another. In particular, Kandev does not invent a writable action head by copying the tracking upstream and does not invent a Pull target from the comparison target. Fork relationships remain provider topology and are represented by explicit source and base identities; Kandev does not persist or branch on `isFork`.

A credential-free repository identity contains a provider, normalized host, full repository path or provider repository ID, and no executor-local remote name. A ref identity adds one literal branch/ref. Repository identity comparison follows the provider's rules; branch/ref comparison remains case-sensitive. GitHub adapters may split a two-segment path into owner and repository, GitLab retains its full namespace path, and Azure retains organization, project, and repository identity. A GitHub, GitLab, or Azure provider operation must receive complete source and base identities that its adapter can translate; missing, cross-host, mismatched, or unsupported identities fail closed rather than falling back to the attached repository or a named remote.

The backend owns durable comparison selection and sends agentctl an additive `ComparisonContext`: the credential-free comparison repository identity, literal comparison ref, and nullable stored base SHA qualified to that exact identity/ref. The backend sends a complete replacement on launch, resume, linked-change association or removal, remote-contribution refresh, and selected-base change. An explicit clear removes the prior context and stored anchor; omission of the additive field during rolling compatibility means no update, not clear. Agentctl never reconstructs provider comparison intent from local remote names.

When an attachment has linked changes, the backend selects a canonical base only from the unique linked change whose exact source repository/ref matches the worktree's writable action head and branch context. Multiple exact matches, no exact match among multiple linked changes, or incomplete source identity make comparison selection unresolved. Kandev does not choose by row order, PR/MR number, branch name alone, or mutable canonical-base fields. Only when no linked change competes does a validated remote-contribution target take precedence over the attached repository plus selected base branch.

Agentctl owns one deep remote-role resolver module. Given the current branch and current `ComparisonContext`, it resolves executor-local remote names and refs, normalizes configured URLs, and returns the writable action head, tracking upstream, and comparison evidence together with an opaque remote-role generation. The generation changes whenever branch/configuration or comparison context changes and binds later mutations to the role set that the caller observed. Status and Git mutation callers use this result instead of reimplementing remote selection. The resolver is a concrete local module tested with temporary Git repositories; no hypothetical adapter interface is added.

Each writable-action or tracking observation carries an exact ref identity when known, `observation_state` (`unknown`, `absent`, or `present`), a nullable remote-head commit, and nullable ahead/behind counts. `unknown` means Kandev lacks authoritative evidence and cannot authorize a mutation from that snapshot. `absent` requires an exact identity and proof that the ref does not exist; it is the only state that can support a first Push, together with local contribution evidence. `present` requires the observed remote-head commit. A null count means unknown and is never coerced to zero. Counts, head SHA, identity, and state are one atomic snapshot and cannot be assembled from different observations.

The structured tracking-upstream observation is authoritative for Pull and tracking divergence. During compatibility migration, existing `RemoteBranch`, `RemoteHeadCommit`, `RemoteAhead`, and `RemoteBehind` fields are a projection of that one snapshot and are never independently merged with it. The writable-action observation is authoritative for Push and source-history safety. `BaseCommit`, `Ahead`, and `Behind` remain comparison evidence. Partial status updates retain an omitted structured observation, replace it atomically when present, and use an explicit `unknown`/clear update to invalidate stale evidence.

Provider history and local checkout history remain separate authorities after upstream change `c4b4791de`: the provider's linked-change source history describes the current PR/MR, local Git describes the checkout, the writable-action snapshot supplies Push divergence, the tracking snapshot supplies Pull divergence, and the comparison target supplies review scope. Provider/local drift classification compares the provider source identity and head with the writable action head, not whichever ref is tracked. A provider-ahead state permits Pull only when the explicit tracking upstream is the same source identity and the fast-forward relationship is proven; otherwise Pull remains unavailable.

The backend supplies comparison identity from the uniquely matched linked change base, then a validated remote-contribution target when no linked change competes, then the attached repository and selected base branch. Agentctl maps that identity to configured remote URLs without assigning meaning to a remote name. If it cannot resolve one unambiguous target ref, it reports comparison evidence as unresolved. A stored base SHA may be used only when the comparison context qualifies it to the same repository/ref; a prior target's SHA is cleared rather than reused. A live non-default comparison ref remains authoritative and is never re-anchored to a default integration branch merely because its merge-base is older. If no identity-qualified stored anchor is available, fallback stops at the safe branch-tip behavior and never substitutes `origin`.

Remote mutation follows the same roles. Push and Force Push target the writable action head; Pull is available only for an explicit tracking upstream; Rebase and Merge target the resolved comparison ref; change-request creation receives an explicit writable source and comparison target. Every mutation request carries the caller's expected remote-role generation, exact target identity/ref, and, when safety depends on it, the expected observation state and remote-head SHA. Under the serialized Git-operation lock, agentctl refreshes the relevant roles and rejects a generation, identity, state, or head mismatch before invoking Git. Normal non-fast-forward checks and force-with-lease remain the final protection against a ref moving after that check.

The backend separately authorizes the exact provider source and base identities against the attached repository, validated remote-contribution binding, workspace provider connection, and credential policy before dispatch. Runtime Git configuration may narrow that authorized source/ref but cannot expand provider automation scope or credential leases. GitHub, GitLab, and Azure adapters receive the same provider-neutral source/base contract and must fail closed if either side cannot be translated exactly.

## Consequences

Custom remote names, contributor forks, nested GitLab namespaces, Azure repositories, and triangular push configurations use the same contract. The sidebar diff count measures contribution scope instead of whichever repository happens to be named `origin`, and desktop/mobile remote-action safety evaluates the repository that the action actually mutates.

The status protocol gains explicit role observations and a generation-bound mutation contract. Backend lifecycle code must refresh or clear comparison context as durable task state changes. Provider adapters must translate the shared repository/ref identities into their native request shapes. When evidence is absent, unknown, stale, or ambiguous, affected actions become unavailable rather than guessing.

The stricter Pull rule removes the former comparison-target fallback for branches without an explicit upstream. Users must configure tracking before Pull is offered, while Push may still target a distinct Git-resolved action head.

## Alternatives Considered

- Treat `origin` as the target and `upstream` as the canonical repository. Rejected because names are executor-local conventions and remote-contribution tasks already use other layouts.
- Add `isFork` and parent-repository metadata. Rejected because a fork boolean does not identify the writable ref, comparison ref, provider host, or non-GitHub source/base relationship.
- Reuse the tracking-upstream snapshot for Push or the comparison target for Pull. Rejected because Git supports distinct push and tracking destinations, and a missing role is not permission to mutate another one.
- Let each status/action/provider caller resolve remotes independently. Rejected because duplicated selection rules already drifted; one deep module provides locality and a shared behavioral test seam.
- Infer a canonical parent whenever a repository is a provider fork. Rejected because comparison intent comes from the uniquely matched linked change, validated binding, or task selection, not from topology alone.
- Authorize a mutation from a previously rendered remote name. Rejected because the worktree can change between observation and action; expected generation, exact identity, and Git's final ref-safety checks close that gap without persisting executor-local names.
