# Kandev Domain Language

Kandev coordinates work across persisted repository attachments, executor-local Git checkouts, and provider change requests. These terms distinguish identities that may refer to the same repository in simple workflows but remain independent in general.

## Git repository roles

**Attached repository**:
The durable repository identity associated with a task. It anchors authorization, workspace scope, and task history.
_Avoid_: origin repository, task remote

**Writable action head**:
The exact repository and ref selected by Git as the current checkout's push destination. It may be the same as or different from the tracking upstream, comparison target, or attached repository.
_Avoid_: fork flag, push fork, head remote

**Comparison target**:
The repository and base ref against which Kandev measures the task's contribution and constructs a change request.
_Avoid_: origin branch, upstream repository

**Tracking upstream**:
The explicit Git upstream configured for the current local branch. It is the only repository/ref role used by Pull and supplies tracking-divergence evidence.
_Avoid_: remote head, push target

**Linked change**:
A provider change request associated with a task repository, with distinct source and base repository/ref identities.
_Avoid_: selected PR row, task remote

## Git role evidence

**Comparison context**:
The selected comparison target together with the stored base anchor qualified to that same repository/ref identity.
_Avoid_: base branch string, origin context

**Stored base anchor**:
A commit SHA captured for one exact comparison target. It is not transferable to a different repository or ref.
_Avoid_: global base SHA, generic fallback

**Remote-role observation**:
Point-in-time evidence for one resolved repository/ref role whose state is unknown, known absent, or present.
_Avoid_: remote status, zero counts

**Remote-role generation**:
An opaque version that binds a requested Git mutation to the exact role identities observed for a worktree.
_Avoid_: timestamp, branch generation

**Remote name**:
An executor-local Git configuration label used to address a remote. It is a routing detail, not a repository role or identity.
_Avoid_: origin role, upstream role
