# Restart continuation, 2026-11-09

Status: INCOMPLETE. Prior changes preserved. Source inspection recovered the predecessor's identity sentinel, pre-writer validation, forwarding rejection without storage cancellation, and valid lifetime-context forwarding/batch tests. Consumer ownership and actual parent-provider integration remain unfinished; this is not slice acceptance.

## Current continuation evidence

All Go commands use `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go -C apps/backend test -trimpath -p 1` from repository root.

- Producer initial GREEN: `-race ./internal/agentctl/server/process -run 'TestErrandRetainedOutput|TestDelivery(UnknownGeneration|ConflictingIdentity|IdentityConflict)|TestLateTerminalKeeps' -count=1`, handle 75914, exit 0, 1.199s.
- Valid forwarding mutation RED: same source with only forwarding's `errors.Is(err, errDeliveryEventIdentity)` skip removed via Go overlay; `-race -overlay=/dev/shm/kandev-errand-remediation-0n6s5t7i/slice3-forwarding-red.json ./internal/agentctl/server/process -run TestDeliveryIdentityConflictDoesNotCancelSuccessor -count=1`, handle 57916, exit 1, 0.085s: `identity conflict cancelled successor provider`. The fixture has a lifetime context and joins owned operations. No source rollback was needed; overlay and source remain in task scratch.
- Moved the cancellable-ingress doc comment back above its actual function; grouped producer test imports. No production behavior change in this continuation.
- Added `TestModelSelectionWarningTextDoesNotExemptOrdinaryWork`: real task service persists identical warning text as message, thinking and tool-call activity after the typed notice. It asserts the activity stays on the open original work turn with no lifecycle-only exemption. This is persistence-boundary coverage, not an end-to-end stream-handler test.
- Final-source focused race command, handle 94828, exit 0: `-race ./internal/agentctl/server/process ./internal/orchestrator -run 'TestErrandRetainedOutput|TestDelivery(UnknownGeneration|ConflictingIdentity|IdentityConflict|Failure|StorageCause)|TestLateTerminalKeeps|TestHandleSessionModelSelectionWarning|TestModelSelectionWarning|TestResumeTodos' -count=1`. Producer PASS 1.520s; orchestrator PASS 2.236s, including both existing `TestHandleSessionModelSelectionWarning*` retry/idempotency regressions, new ordinary-work coverage, separate lifecycle turn and all selected `TestResumeTodos*` cases.

- Consumer regression probes (isolated overlay, no permanent failing tests): `-race -overlay=/dev/shm/kandev-errand-remediation-0n6s5t7i/slice3-consumer-probes/overlay.json ./internal/agent/runtime/lifecycle -run 'TestErrandUnknownSubmissionDoesNotAdoptCurrentTurnProbe|TestErrandExplicitConflictingTurnProbe' -count=1`, handle 70445, exit 1, 0.362s. Both assertion-quality failures return `successor`: (1) original submission with generation zero inherits the current successor turn in `durableAgentEvent`; (2) explicit successor turn overrides the retained generation-one original turn in `promptTurnIDForEvent`. Probe source and overlay are retained in that scratch directory.
- Changed-Go lint handle 42803 exited 4 after its five-minute package-loading timeout, with no code diagnostics. Same scoped command retried as handle 55226 after dependency warming; also exited 4 at timeout, printing `0 issues.`. Neither run is a lint pass: `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOLANGCI_LINT_CACHE=/dev/shm/kandev-errand-remediation-0n6s5t7i/lint-cache GOMAXPROCS=2 GOMEMLIMIT=3GiB bash scripts/lint-go-changed apps/backend/internal/agentctl/server/process/manager.go apps/backend/internal/agentctl/server/process/delivery_event_identity_test.go apps/backend/internal/orchestrator/event_handlers_streaming.go apps/backend/internal/orchestrator/errand_warning_turn_test.go`.

## Inspection conclusions

Consumer implementation has not changed. `lifecycle/types.go:promptTurnIDForEvent` trusts an explicit turn and can assign the mutable current prompt to an explicitly supplied submission. `sqlite/agent_delivery_projection.go:canonicalDeliveryTurnTx` trusts a requested turn and otherwise selects the newest open turn. The immutable journal and inbox boundaries need exact validation; producer correctness alone does not close this gap.

The original prompt generation is present in the replaceable `delivery_prompt_turn_binding`, and the accepted interruption assessment retains it only for eligible proven-dead recovery. `errand.turn_settlement` lacks prompt generation. The general prompt-owner history must therefore be retained separately at admission, before successful predecessor replacement too; treating every saved binding as an interrupted-delegation assessment would invent death authority. `UpsertExecutorRunning` already owns a transaction with task/session/delivery locks and is a candidate atomic retention seam. This is implementation guidance, not an accepted new contract.

API inspection found journal replay restores `DeliverySubmissionID` from the journal envelope and JSON retains `PromptGeneration`; no API propagation defect was established. `promptSubmissionPayload` excludes generation, so the SQL prompt payload cannot substitute for retained generation evidence.

Acceptance-fixture concern: `lifecycle/errand_interrupted_delegation_test.go:newInterruptedDelegationRecoveryFixture` seeds an `interrupted_unknown` journal submission and uses SQL `PrepareAgentDeliverySubmission` plus direct accepted/dispatching transitions when `admitted=true`. It does exercise a real original process death and native reconciliation, but it does not send the original prompt through the real child dispatch/provider boundary. The existing external-package result test also uses the fake `interruptionExternalBoundaries` deliverer. Final acceptance needs a fixture that crosses actual child admission and real parent notification/queue/readiness/admission; extending the notification counter alone cannot satisfy the requested evidence. This does not invalidate the narrower recovery-proof tests.

Presentation gap: `errands.ProjectStatus` still defaults an idle launched Errand with no settled outcome to `running`; uncertain unproven history therefore still has the original misleading projection. Proven interruption produces failed status and the explicit interruption result text. No new uncertain status or frontend change is authorized here, and no historical repair eligibility is established. Actual explicit-resume/provider acceptance remains unverified.

Latest root headroom is 43 GiB, available RAM 9.3 GiB; no shared caches, unowned files, index, commits, parent checkout or live state were changed. Exact incremental write set: process/manager.go (comment placement), process/delivery_event_identity_test.go (import grouping), orchestrator/errand_warning_turn_test.go (new coverage), and this handoff. Existing orchestrator/event_handlers_streaming.go behavior remains the predecessor's change.

## Final checkpoint

All command handles are finished. Focused final-source race checks pass; lint remains unverified after two bounded timeouts. Consumer probes deliberately fail only via retained overlays. No SQL source changed in this continuation, and SQL guard/store conformance/PostgreSQL gates were not rerun; prior accepted evidence is unchanged, not refreshed. No full-suite or whole-task completion claim.

Remaining implementation is substantial: immutable prompt-owner retention before every relevant replacement, exact inbox/projection/replay validation, genuinely unowned native-work provenance, and actual child-admission-to-parent-provider acceptance. The new ordinary-work test covers real persistence after a typed warning; it does not substitute for stream-handler coverage across all activity types. Primary must not accept the complete slice on these results. No consumer implementation was added in this continuation.

HEAD remains `11c2b68f869bc11eab8142764655a0a91415bbc6`; no commits, index operations, live state changes, or cache cleanup. Scratch probes remain available under `/dev/shm/kandev-errand-remediation-0n6s5t7i/` as named above.

---

# Slice 3 continuation: partial implementation, resource blocked

Status: INCOMPLETE. Root free space fell from 3.5 GiB to 1.2 GiB after serialized Go builds, below the explicitly required 1.5 GiB stop threshold. All command handles are finished. No further heavy checks or code changes should proceed until the primary establishes headroom. Shared caches and unowned files were not deleted. This is not slice acceptance, whole-task completion, or historical repair eligibility.

## Changes in this continuation

Exact write set (prior slices preserved):

- `apps/backend/internal/agentctl/server/process/manager.go`: event attribution consults bounded retained generation history after terminal removal; unknown numbered output no longer adopts the active submission; generation-zero fallback remains. Persistence rejects an explicit submission conflicting with known generation history before journal append.
- `apps/backend/internal/agentctl/server/process/delivery_event_identity_test.go`: permanent late-output and real journal-replay regressions, unknown/evicted generation cases, generation-zero dispatch/async controls, and conflicting identity append rejection. Existing terminal control retained.
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`: typed model-selection warnings use `CreateLifecycleSessionMessage`, which creates a separate completed lifecycle-only turn through the existing production API.
- `apps/backend/internal/orchestrator/errand_warning_turn_test.go` (new): SQLite-backed warning isolation from active work; warning turn completed and lifecycle-only, original work remains open and retains active ownership.
- This handoff only. No other documentation, Git index, parent checkout, live state, commits, pushes, tasks or sessions changed.

## Exact verification evidence

All commands below ran from repository root with `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go -C apps/backend test -trimpath -p 1`; add the following arguments:

- RED `./internal/agentctl/server/process -run 'TestErrandRetainedOutput' -count=1`, handle 41599, exit 1, 0.022s: both tests assigned predecessor output to successor, including real journal replay.
- RED `./internal/agentctl/server/process -run 'TestDelivery(UnknownGeneration|ConflictingIdentity)' -count=1`, handle 24508, exit 1, 0.022s: evicted/unknown generation adopted active; conflicting identity committed.
- Initial GREEN attempt, handle 88316, failed only because the conflict test treated the expected absent stream as an error after rejecting its first event. Corrected assertion permits `journal.ErrStreamNotFound` while requiring zero events.
- GREEN `-race ./internal/agentctl/server/process -run 'TestErrandRetainedOutput|TestDelivery(UnknownGeneration|ConflictingIdentity)|TestLateTerminalKeeps' -count=1`, handle 29112, exit 0, 1.169s. Covers all permanent producer regressions currently retained.
- RED `./internal/orchestrator -run TestModelSelectionWarningUsesSeparateLifecycleTurn -count=1`, handle 19596, exit 1, 0.138s: warning attached to active work turn.
- GREEN `-race ./internal/orchestrator -run 'TestModelSelectionWarning|TestResumeTodos' -count=1`, handle 40611, exit 0, 1.631s. This selection includes the new warning test and existing resume-todo tests; it does NOT select `TestHandleSessionModelSelectionWarning*`, which still needs running.

Gofmt applied to touched Go files. No changed-Go lint, SQL guard/store conformance, PostgreSQL, consumer identity, or actual parent-provider acceptance check was run in this continuation. No broader-suite pass is claimed.

## Important unfinished producer error path

Source inspection after the focused GREEN found that a conflict rejected in `persistDeliveryBatchRaw` enters `forwardUpdates`' generic storage-failure branch. It calls `cancelDeliveryAfterStorageFailure(update.DeliverySubmissionID)`; when the conflicting explicit ID names active B, it can cancel B. Do not accept the producer patch until identity rejection is distinguished from storage failure and this path is covered through real forwarding.

A temporary regression attempted this path using `newAllToolManager`; handle 38513, exit 1, 0.024s reached cancellation but panicked in `BeginOwnedOperation` because that fixture lacks `lifetimeCtx`. This is NOT an assertion-quality RED. The unfinished probe was removed from the permanent test file and retained at `/dev/shm/kandev-errand-remediation-0n6s5t7i/slice3-conflict-forwarding-probe.txt`. Repair the fixture with a lifetime context and joined owned-operation cleanup, then prove successor provider cancellation and journal/output isolation at the real forwarding boundary before implementing the fix. No production change was made from the invalid probe. The permanent test source is restored to the version covered by handle 29112 (apart from removing the appended probe).

## Remaining scope

Consumer ownership has not changed: `promptTurnIDForEvent` still trusts explicit turns and can bind a supplied submission to mutable current work; `canonicalDeliveryTurnTx` still trusts requested turns/adopts arbitrary open turns. Preserve exact task/session/incarnation/harness/submission/turn identity at admission and retain prompt-generation evidence through replacement, inbox and replay. Existing settlement intent has no prompt-generation column; existing running metadata stores only the current `delivery_prompt_turn_binding`. Use existing durable effect/provenance mechanisms as appropriate rather than guessing old ownership. This is implementation work still within the authorized scope, not a new scope request.

Typed warning separation is only partially verified: add typed-vs-identical-text message/reasoning/tool cases, status followed by actual output, repeated handling and persistence-failure coverage. No historical classification was added; incident work/warning-looking text/unattributed terminal remain non-authoritative.

Actual recovery through real parent deliverer, notification queue, readiness/admission and captured external provider prompt remains unimplemented. The accepted slice-2 notification counter still is not sufficient proof. All cancellation/newer result/role isolation and SQL uncertainty requirements remain in force.

Final resources: root 1.2 GiB free; `/dev/shm` 9.7 GiB free (about 52 KiB used before adding this probe); no running check handles. The threshold prevents further builds under current resources. Only primary should decide how to restore headroom without removing shared caches or unowned artifacts.

---

The previous scope-blocked handoff follows as historical evidence; its scope restriction was resolved by the current dispatch.

# Slice 3: turn attribution and parent delivery

Status: blocked before permanent code/test changes by a reproduced producer defect outside the exclusive write scope. Slices 1/2 and all existing dirty changes are preserved. This is not slice completion or whole-work-order acceptance.

## Required scope decision

The assigned scope excludes `apps/backend/internal/agentctl/server/process/`. The instructions explicitly require returning precise scope evidence before editing an unavoidable out-of-scope producer. A valid late A output can be irreversibly attributed to B before it reaches any assigned backend consumer. Add narrowly scoped ownership of `apps/backend/internal/agentctl/server/process/manager.go` and its focused producer tests before continuing implementation. No protocol/schema/UI redesign is indicated by this evidence.

`manager.go:838` (`deliverySubmissionIDForEvent`) reads `deliveryPromptByGeneration`, deletes its entry on complete/error, and falls back to `activeDeliverySubmissionID()` when no mapping remains. `recordDeliverySubmissionHistoryLocked` retains the original generation mapping separately, but this event attribution path does not consult that history. After A completes and B is active, a message chunk still carrying A's prompt generation therefore acquires B's submission ID. The existing `TestLateTerminalKeepsItsSubmissionDuringSuccessorDispatch` tests the first terminal, not subsequent retained output.

The production forwarding path assigns this submission at `manager.go:2633` before `persistDeliveryEvent`; the persistence path also invokes the same fallback at `manager.go:2811` and commits that ID into `journal.Event.SubmissionID`. A real journal probe reproduced the wrong durable ID. Backend validation must reject conflicting submission/generation ownership; reassigning this event to A downstream would contradict the retained journal identity. Merely rejecting it would leave valid retained A output unprojectable, so consumer-only changes cannot fulfill the requested valid late-output preservation contract.

## Behavioral RED evidence

Temporary Go overlays only; no repository file under agentctl was edited. The overlay extends the existing `delivery_event_identity_test.go`. It and the full probes remain in task-owned RAM scratch `/dev/shm/kandev-errand-remediation-0n6s5t7i/slice3-probe/` (8 KiB); all command handles are finished. The complete overlay source is reproduced below for durability.

1. From `apps/backend`: `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -trimpath -p 1 -overlay=/dev/shm/kandev-errand-remediation-0n6s5t7i/slice3-probe/overlay.json ./internal/agentctl/server/process -run 'TestErrandSlice3RetainedOutputSubmissionProbe|TestLateTerminalKeepsItsSubmissionDuringSuccessorDispatch' -count=1`. Handle 92656, exit 1, 0.032s: `retained A output attributed to "successor", want predecessor`. The existing terminal control did not fail.
2. From repository root: `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go -C apps/backend test -trimpath -p 1 -overlay=/dev/shm/kandev-errand-remediation-0n6s5t7i/slice3-probe/overlay.json ./internal/agentctl/server/process -run 'TestErrandSlice3RetainedOutputJournalProbe' -count=1`. Handle 84070, exit 1, 0.023s: `journal persisted A output with submission "successor", want predecessor`. This exercises real journal append/replay with only a temporary fixture database, no live runtime.

No GREEN, race, lint, SQL guard or PostgreSQL acceptance claim: production is unchanged and the probes deliberately fail. No integrated parent-provider test has been added; actual parent delivery remains unproven.

## Consumer inspection and remaining work

- `lifecycle/types.go:1155` trusts explicit turn IDs and falls back to the mutable current prompt turn for a supplied submission. Resolve against immutable exact admission identity before storing the inbox; never let a supplied A submission select B's current turn.
- `sqlite/agent_delivery_projection.go:542` trusts requested turns and otherwise adopts any open session turn. Errand projection needs task/session/incarnation/harness/submission/turn validation, original abandoned-turn replay, and separate unowned native activity with immutable provenance. Preserve ordinary native async compatibility.
- `lifecycle/received_delivery_projection.go` restores the retained envelope for replay; retained identity must remain authoritative.
- `orchestrator/event_handlers_streaming.go:4816` persists typed model-selection warnings using `getActiveTurnID`. Use separate completed lifecycle-only activity and keep subsequent real message/reasoning/tool output distinct. No text-based historical reclassification is permitted.
- The integrated SQLite/lifecycle/orchestrator acceptance must still exercise real parent deliverer, notification queue, readiness and admission with only the external transport faked; repeated recovery/drain must not duplicate the complete interruption block. Preserve unknown historical SQL outcomes, cancellation, newer revisions and task/role isolation.

No primary spec update is required for a new contract: the accepted ADR and owning designs already require immutable late-output attribution. Primary should record this producer prerequisite and scope decision in the work order/task plan before redispatch. No incident repair eligibility is established; g1 work and g2 warning-looking text remain unclassified without exact retained proof.

## Write set and resources

Repository write set for this attempt: only `docs/plans/errand-orphan-settlement/turn-attribution-handoff.md`. Temporary scratch: overlay JSON and Go probe source named above. No production/permanent test changes, commits, pushes, Git index operations, parent-checkout changes, task/session creation or live-state mutations.

Branch remains `feature/fix-stranded-errands-mo6`; HEAD remains `11c2b68f869bc11eab8142764655a0a91415bbc6`. `git diff --check` passed before this handoff write. Root stayed at 3.6 GiB free; RAM scratch filesystem remains 9.7 GiB free with only the 8 KiB probe retained. Available system memory was 12 GiB. No shared cache or unowned temporary file was deleted.

## Reproducible overlay source

Map the absolute path of `apps/backend/internal/agentctl/server/process/delivery_event_identity_test.go` to a temporary file containing this source using Go `-overlay`. The first test is the unchanged existing positive control.

```go
package process

import (
	"testing"
 "path/filepath"
 "github.com/kandev/kandev/internal/agentctl/journal"
 "github.com/kandev/kandev/internal/agentctl/server/config"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
)

func TestLateTerminalKeepsItsSubmissionDuringSuccessorDispatch(t *testing.T) {
	manager := &Manager{deliveryActiveID: "successor"}
	manager.TrackDeliverySubmission("predecessor", 1)
	manager.TrackDeliverySubmission("successor", 2)
	id := manager.deliverySubmissionIDForEvent(adapter.AgentEvent{
		Type: adapter.EventTypeComplete, PromptGeneration: 1,
	})
	if id != "predecessor" {
		t.Fatalf("late terminal submission = %q, want predecessor", id)
	}
	if manager.deliverySubmissionIDForEvent(adapter.AgentEvent{PromptGeneration: 2}) != "successor" {
		t.Fatal("late terminal removed successor association")
	}
}

// Probe retained output after A completed and B became the active dispatch.
func TestErrandSlice3RetainedOutputSubmissionProbe(t *testing.T) {
 manager := &Manager{deliveryActiveID: "successor"}
 manager.TrackDeliverySubmission("predecessor", 1)
 manager.TrackDeliverySubmission("successor", 2)
 if got := manager.deliverySubmissionIDForEvent(adapter.AgentEvent{Type: adapter.EventTypeComplete, PromptGeneration: 1}); got != "predecessor" { t.Fatalf("terminal = %q", got) }
 if got := manager.deliverySubmissionIDForEvent(adapter.AgentEvent{Type: adapter.EventTypeMessageChunk, PromptGeneration: 1}); got != "predecessor" { t.Fatalf("retained A output attributed to %q, want predecessor", got) }
}

func TestErrandSlice3RetainedOutputJournalProbe(t *testing.T) {
 path := filepath.Join(t.TempDir(), "delivery.bbolt")
 j, err := journal.Open(journal.Config{Path:path})
 if err != nil { t.Fatal(err) }
 manager := &Manager{cfg:&config.InstanceConfig{DurableJournalPath:path, SessionID:"session", DeliveryStreamID:"stream", DeliveryIncarnationID:"incarnation", DeliveryHarnessGeneration:1}, logger:newTestLogger(t), deliveryJournal:j, deliveryActiveID:"successor"}
 t.Cleanup(func(){ if err := manager.closeDeliveryJournal(); err != nil { t.Error(err) } })
 manager.TrackDeliverySubmission("predecessor",1)
 manager.TrackDeliverySubmission("successor",2)
 if got := manager.deliverySubmissionIDForEvent(adapter.AgentEvent{Type:adapter.EventTypeComplete, PromptGeneration:1}); got != "predecessor" { t.Fatalf("terminal = %q",got) }
 _, err = manager.persistDeliveryEvent(adapter.AgentEvent{Type:adapter.EventTypeMessageChunk, PromptGeneration:1, Text:"late A output"})
 if err != nil { t.Fatal(err) }
 events, _, err := j.Replay(t.Context(), "stream", 0, 10)
 if err != nil { t.Fatal(err) }
 if len(events) != 1 { t.Fatalf("events = %d",len(events)) }
 if events[0].SubmissionID != "predecessor" { t.Fatalf("journal persisted A output with submission %q, want predecessor",events[0].SubmissionID) }
}
```
