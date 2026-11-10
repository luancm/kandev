# Shell transcript ownership remediation

Implemented and tested; writer ownership released for primary review. This is the bounded shell ownership audit remediation, not whole-task acceptance, parent-provider delivery acceptance or historical incident repair. All prior dirty work was preserved. No commits, index changes, other checkout changes, live state operations, delegation or UI changes occurred.

## Scope and invariants

The audit finding was confirmed: canonical shell starts and output updates selected the first session/tool message, bypassing immutable Errand turn ownership. A successor checkpoint could therefore replace a predecessor's output.

Errand shell lookup now requires the resolved immutable turn and exact parent-tool ID, rejects duplicate matching messages, and includes the resolved turn in message creation identity. Ordinary-session selection remains unchanged. Checkpoint reset and revision semantics remain inside the selected message boundary.

Unowned native tool starts retain compact source provenance atomically with inbox receipt in a sequence-zero, versioned effect, `agent_delivery.unowned_tool_source.v1`. Source records contain sequence, tool, parent-tool, prompt generation, protocol message ID, control turn ID and resolved source-derived turn. Session, incarnation, harness and stream are retained in the existing effect columns. These records survive inbox body pruning and have no terminal/death authority.

Unowned shell and tool-status updates resolve through those retained sources, never canonical message fields or a session-wide first match. Matching requires the same session/incarnation/harness/stream, prompt generation, tool and parent-tool, an earlier source sequence, and no conflicting control-turn IDs. An exact protocol-message match disambiguates candidates; otherwise a unique turn owner suffices because protocol IDs legitimately change between start and streaming updates. Repeated starts with the same turn owner do not create false ambiguity. Multiple distinct owners, missing sources and conflicting evidence fail closed. The original all-preceding-turn settlement fence, lifecycle classification and provider uncertainty are unchanged.

The added call in `agent_delivery.go` is necessary: source provenance must commit with accepted inbox receipt, before projection/callbacks and pruning, including when starts and updates arrive before projection. No model, service, lifecycle or producer changes were needed. Generic unowned tool-status updates use the same source resolver to avoid creating a different turn from a changed protocol ID. Parent-tool equality also fixes the adjacent same-turn nested-tool collision.

Tradeoffs: one compact retained effect per unowned tool-start sequence, queried within the exact source stream; no schema or secondary mutable ownership registry. Historical unowned updates without retained/replayable start-source evidence remain blocked rather than adopting an admitted or unrelated turn. Sources arriving out of order must be received before their dependent updates can be accepted. Source records follow the existing session-owned effect lifetime. This remediation does not fabricate legacy provenance.

## Exact write set

- `apps/backend/internal/task/repository/sqlite/agent_delivery_shell_start.go`: scoped lookup argument and turn-bearing creation identity.
- `apps/backend/internal/task/repository/sqlite/agent_delivery_shell_projection.go`: turn/parent-aware Errand lookup and ambiguity rejection.
- `apps/backend/internal/task/repository/sqlite/agent_delivery_tool_source.go` (new): retained source receipt, source matching and unowned update resolver.
- `apps/backend/internal/task/repository/sqlite/agent_delivery_admission.go`: route unowned ownership resolution through the source resolver; preserve the previous writer's admission implementation.
- `apps/backend/internal/task/repository/sqlite/agent_delivery.go`: retain the resolved source in the inbox receipt transaction; preserve previous admission/terminal changes.
- `apps/backend/internal/task/repository/sqlite/errand_shell_ownership_test.go` (new): real SQLite receive/canonical projection regressions and shared PostgreSQL exercise.
- This handoff only among documentation files.

## Behavioral evidence

`TestErrandShellOwnership/{admitted,unowned}` covers completed admitted A and later admitted/unowned B sharing tool ID, separate message identity, B checkpoint, delayed A incremental output, later B incremental output, and exact/equivalent replay with reconstructed canonical IDs. Actual admitted ownership records are written through submission preparation and running-row admission retention. The successor turn itself is fixture-seeded directly to isolate transcript ownership from the separately owned settlement/admission workflow.

`TestErrandShellSourceEvidence` covers missing source, conflicting control-turn identity, ambiguous distinct source owners, exact protocol disambiguation, repeated same-owner starts, same-turn same-tool distinct parents, cold repository use after inbox bodies are removed, generic tool-status source correlation, and duplicate canonical-message rejection without output mutation.

Existing incremental-shell tests cover ordinary-session behavior, checkpoint/identity revision resets, crash rollback/replay, payload pruning, removed payloads, title updates, ordinary replacement, projected metadata and endurance. Existing Errand admission tests and canonical shell turn/segment controls also passed.

## Commands and receipts

All Go/lint commands below ran from the repository root with the exact prefix `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB`. Commands were serialized, every handle was polled to completion, and source was not edited during checks.

- Initial RED, handle 54745: `go -C apps/backend test -trimpath -p 1 ./internal/task/repository/sqlite -run '^TestErrandShellOwnership$' -count=1`. Unowned case failed expected message-count assertion; admitted fixture first hit the existing settlement fence, so that case was not valid ownership RED yet.
- Corrected permanent RED, handle 53382, same command: both admitted and unowned cases failed “should have 2 items, but has 1” at successor shell start.
- After turn-scoped lookup, handle 40029, same command: admitted case passed; unowned update failed `sql: no rows in result set`, demonstrating the missing source-correlation requirement.
- Source-mapping compile attempt 63424 failed on an int64/uint64 control-turn field mismatch; corrected before behavioral checks, not claimed as RED.
- GREEN, handle 89619: `go -C apps/backend test -trimpath -p 1 ./internal/task/repository/sqlite -run 'TestErrandShellOwnership|TestIncrementalShell|TestErrandAdmissionOwnership' -count=1`, PASS 27.962s.
- Additional RED, handle 31567: `go -C apps/backend test -trimpath -p 1 ./internal/task/repository/sqlite -run '^TestErrandShellSourceEvidence$' -count=1`. Repeated same-owner start failed with identity mismatch. A separate parent case had a fixture sequence gap; that fixture was corrected by projecting its status event. Same-owner candidate deduplication repaired the actual defect.
- Race GREEN, handle 8796: `go -C apps/backend test -trimpath -p 1 -race ./internal/task/repository/sqlite -run 'TestErrandShell|TestIncrementalShell|TestErrandAdmissionOwnership|TestPostgresErrandShell' -count=1 -v`, PASS 69.042s including shell endurance 63.29s. Output: `/tmp/shell-ownership-race.log`.
- Expanded assertions, handle 2570: `go -C apps/backend test -trimpath -p 1 -race ./internal/task/repository/sqlite -run 'TestErrandShell|TestPostgresErrandShell|TestCanonicalDelivery.*Shell' -count=1 -v`, PASS 4.028s.
- Initial lint handle 4903 found admission cyclomatic complexity 17 (limit 15). Extracted the unowned resolver; no behavioral change.
- Final-source race, handle 57129: `go -C apps/backend test -trimpath -p 1 -race ./internal/task/repository/sqlite -run 'TestErrandShell|TestPostgresErrandShell|TestErrandAdmissionOwnership|TestCanonicalDelivery.*Shell|TestIncrementalShell(Identity|Detail|Projection|Retention|Removed|Projected|Ordinary)' -count=1 -v`, PASS 6.930s. Every new regression and affected shell/admission control selected. Output: `/tmp/shell-ownership-final.log`.
- Final scoped lint, handle 88892: `bash scripts/lint-go-changed apps/backend/internal/task/repository/sqlite/agent_delivery.go apps/backend/internal/task/repository/sqlite/agent_delivery_admission.go apps/backend/internal/task/repository/sqlite/agent_delivery_shell_start.go apps/backend/internal/task/repository/sqlite/agent_delivery_shell_projection.go apps/backend/internal/task/repository/sqlite/agent_delivery_tool_source.go apps/backend/internal/task/repository/sqlite/errand_shell_ownership_test.go`, exit 0, zero issues.
- SQL guard, handle 82134: `go -C apps/backend run -trimpath -p 1 ./cmd/sqlguard ./internal`, exit 0.
- Store conformance, handle 29523: `go -C apps/backend test -trimpath -p 1 -race ./internal/persistence/storeconformance -count=1`, PASS 67.795s.

PostgreSQL: `TestPostgresErrandShellOwnership` and four existing incremental-shell PostgreSQL tests were selected and explicitly skipped because `KANDEV_TEST_POSTGRES_DSN` is unset. No PostgreSQL runtime pass is claimed.

## Resources and limits

Initially root had 31 GiB free, /dev/shm 6.9 GiB free and memory 11 GiB available. Final heavy-check sampling: root 29 GiB free, /dev/shm 6.6 GiB free, memory 11 GiB available. The 1.5 GiB root stop threshold was never approached. No shared cache or scratch cleanup was performed; no checks remain running after release.

Primary owns review, durable contract/task-plan updates, parent delivery/UI work and final initiative acceptance. No historical repair eligibility or whole-suite-green claim is made.
