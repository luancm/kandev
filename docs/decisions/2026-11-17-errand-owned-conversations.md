# ADR-2026-11-17-errand-owned-conversations: Errand-owned conversations

**Status:** accepted
**Date:** 2026-11-17
**Area:** backend, frontend, protocol, workflow

## Context

The unpublished Errands implementation represents delegated work as task
sessions. Its conversation and execution capabilities are useful, but ordinary
session selection, workflow, restoration, and navigation require Errand
exceptions. The user selected independent Errand ownership after comparing it
with retaining and simplifying the session-backed design.

Errands has not merged to main. The user accepts migrating or deleting their
experimental Errands data. Existing main users must retain their data, supported
contracts, configuration, recovery, and ordinary-session behavior.

## Decision

An Errand owns its conversation and execution. Its parent remains an ordinary
task session. Creating or restoring an Errand must not require a corresponding
`TaskSession` row or insert a synthetic session into ordinary frontend state.
This supersedes the session-identity and session-role prescriptions in
[the original Errands decision](2026-10-23-errands-first-class-delegation.md).
The original product, workspace, authority, and delegation boundaries remain.

Use additive Errand-owned turn and message storage, with scoped ownership,
stable history pagination, and complete message metadata. Preserve the main
session transcript tables and their constraints. Do not generalize or migrate
main's existing transcript storage solely to accommodate Errands.

Reuse the runtime's explicit execution-owner mechanisms and existing runtime
inventory. That inventory already supports owners without task-session rows;
another inventory table would duplicate its responsibility. Give Errands an
explicit owner and namespaced transport identity. Supply Errand-owned durable
admission, generation, transcript projection, and recovery adapters wherever the
existing task path requires task-session persistence. A taskless process launch
alone does not establish durable conversation recovery.

Reuse conversation rendering through explicit data and action interfaces.
Ordinary task chat keeps its session adapter; Errand chat receives Errand-owned
data. Remove replaced ordinary-session exceptions after the new path is wired.
Preserve steering, permission handling, transcript inspection, workspace
interactions, and desktop/phone navigation; independent ownership is not a
reduction in user capabilities.

Result settlement, parent reception, and physical runtime absence remain
separate facts. Preserve exact submission and execution identities, atomic
claims, cancellation fences, uncertain delivery, and restart evidence. Reuse
receipt mechanisms with owner-specific admission and projection rather than
creating another receipt protocol or copying weaker taskless-run guarantees.

The one-time branch-data transition may migrate or remove experimental Errands.
It must identify Errand-owned records and outstanding runtimes explicitly and
must not remove ordinary task/session data. Permanent compatibility readers for
unpublished Errands formats are not required. Main-origin database upgrades and
ordinary behavior remain acceptance requirements on both supported dialects.

## Consequences

Errands gains explicit persistence and runtime adapters while ordinary sessions
can shed Errand membership exceptions. The adapter and deletion boundaries must
be tested together; moving session dependencies behind renamed wrappers would
not achieve independent ownership.

The first slice adds Errand transcript persistence. It does not alone switch
launch, recovery, or UI ownership. Implementation and verification status belong
in the implementation record, not in this decision's acceptance status.

Reduced coupling and work are hypotheses until measured. Existing failed
performance gates and qualified historical evidence remain recorded; source
size, fewer imports, and passing functional tests do not establish a performance
improvement.

## Alternatives considered

- Retain task-session ownership and consolidate repeated recovery/claim reads.
  This remains the control, but retains the cross-cutting membership exceptions.
- Generalize all session storage into a shared conversation model. This may
  share more SQL, but imposes a broader migration on main users and changes
  established storage constraints without a demonstrated need.
- Rebuild execution and transcript rendering exclusively for Errands. This
  duplicates existing capabilities; reuse their narrow interfaces instead.
