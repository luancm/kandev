# Exact tool terminal replay

The repair addresses the observed completed tool → late cancellation without prompt identity → blocked completion sequence. It does not infer success from transcript text or authorize uncertain external effects.

## Implementation

The ACP adapter no longer adds an initially terminal generic tool to its active-tool set. Legitimately pending tools retain their original native session, parent tool and prompt generation until cleanup; cleanup emits that captured identity rather than the current prompt's identity. Existing Codex correlation and background-task behavior remain covered by the full adapter suite.

Errand receipt intake atomically retains a versioned admitted-tool source alongside the accepted source event. The record binds the tool and its parent to the retained prompt admission, native identity, stream, source sequence and payload hash. Missing-submission updates can resolve only when these retained sources establish one original turn. Competing unowned sources participate in the same ambiguity check. Received bytes remain unchanged, and attribution alone cannot settle an Errand.

For existing records, replay can reconstruct provenance from intact, hash-valid admitted source receipts. Receipt acceptance backfills that proof before later payload compaction. Pruned, contradictory or ambiguous historical sources remain unresolved. No journal entry is skipped and no live database or runtime is modified. The internal effect uses the existing effects table, so this repair adds no schema migration or upgrade fixture change.

## Verification

Adapter RED reproduced both spurious cancellation of an initially completed/failed tool and loss of the original pending-tool owner. Repository RED reproduced the late unowned update rejection; a second RED established the missing historical-source reconstruction path. Logs are retained under `/tmp/kandev-tool-terminal-*`.

The repository race suite passed eight top-level tests on SQLite and disposable PostgreSQL 16, with no skips. Its evidence matrix covers compaction/reopen, intact and pruned historical receipts, corrupt payloads, reused tool IDs, competing unowned sources, wrong native/parent/generation/control/execution/incarnation/harness identity, missing admission, ordinary sessions and duplicate replay. The full ACP adapter package passed.

Twelve lifecycle/orchestrator top-level race tests passed. The new integration case uses real SQLite receipt/projection and terminal admission, processes the following completion through the normal ready handler, then composes exactly one parent result. Replaying the terminal produces no second result; no successful settlement receipt is injected. Existing stale-successor and automatic parent-notification controls pass. An initial integration fixture run correctly refused a mock execution whose prompt generation was not initialized; the final fixture supplies the actual admitted generation and leaves the production guard intact.

The full store-conformance suite passed eleven top-level tests in 190.420 seconds, including SQLite and PostgreSQL previous-stable upgrades with no skips. SQL guard exited zero. The five-minute lint wrapper timed out during package loading; the longer equivalent found a complexity limit and a duplicated status literal. Extracting the unique-owner decision and using the ACP SDK status constant resolved both. The final three-package lint reports zero issues; the repository eight-test race matrix and full adapter suite were rerun after that refactor and passed. The earlier twelve-test lifecycle/orchestrator receipt remains pre-refactor evidence. Browser rendering is unchanged by this repair; existing result-card and phone behavior remain separate evidence. Local mock/SQLite/PostgreSQL checks do not certify every provider or repair every historical incident.

## Delivery

Parent-owned integration, normal-hook commit, upstream merge and push remain pending. The roadmap and current task plan own that queue. Do not treat saved implementation as a deployed runtime repair.

## Final source and receipts

| Path | SHA-256 |
| --- | --- |
| `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go` | `f127e6dd2d7efd44e673f4e8cc51767e033fbe0d6285a2ff53dae8a48ccaadc2` |
| `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_prompt.go` | `f7e680499cd3c315ac02861ebf23df57d219ab1af87ed770fbee6c91e27ce9dd` |
| `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go` | `18c5acba5d3993230b72ad5db8c14e0a54076b8a88ae2bb955e9766c7ca7350a` |
| `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tool_owner.go` | `8f5ac3f824fd251437d97cff4fc5d4aa1884784b1ea109baa2e7f2e6ca591002` |
| `apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tool_terminal_identity_test.go` | `7d04ba9bdee9e6448f22778daf1d732a0133ce1fdc5f7e7436379ebf0898c20c` |
| `apps/backend/internal/task/repository/sqlite/agent_delivery.go` | `6508df907995fc1b22b6a835a0f68226e87391137940c0ff793c8e11b3de4aea` |
| `apps/backend/internal/task/repository/sqlite/agent_delivery_tool_source.go` | `cf8aa888045b7673108cd7ed5a1d026d0ba295300c5c5e8a68d2bdf5c7caf3fe` |
| `apps/backend/internal/task/repository/sqlite/agent_delivery_admitted_tool_source.go` | `d6bc016c728c9a398abdb25a4c997596890f111d60fdc62a960194f4c45946ba` |
| `apps/backend/internal/task/repository/sqlite/agent_delivery_legacy_tool_source.go` | `1c18a49ed04a4420c5028d1db4381f5924c18a2ac4a63fdbba8c2fe55129f454` |
| `apps/backend/internal/task/repository/sqlite/errand_admitted_tool_replay_test.go` | `fcd8202e45e7619bc87dae86130c2656a15a29a7f252d2c38dd5e727fbb03ca2` |
| `apps/backend/internal/task/repository/sqlite/errand_shell_ownership_test.go` | `05fdc4d1a2082efc293ffe3f926413173cfd6ee1706158f44829a66be0933f49` |
| `apps/backend/internal/orchestrator/errand_tool_terminal_replay_test.go` | `ca6cbcfa8c0264214f07d0f73800c1421fbd0e3716a2b4cd964670dc586085e5` |
| `docs/specs/tasks/system-design/errand-recovery-and-delivery.md` | `2e44814357de0b0cfd528103c582341981223ae955bfaf09bc4ea6fd8ede0cef` |
| `/tmp/kandev-tool-terminal-adapter-red.log` | `ce16b0411cdfb1b1261d5f83cc3cd31643e0e969a82b7a0f30bb1648ede8b592` |
| `/tmp/kandev-tool-terminal-replay-red.log` | `378756ab7306896399e3d7f8e20a42f866a6ff560487a412c47fbed000972d8c` |
| `/tmp/kandev-tool-terminal-evidence-red.log` | `a028d55335030e8d5dd39337d818daf795ffb51d80f3d4fa2b4e5b2ebc65dceb` |
| `/tmp/kandev-tool-terminal-both-engines-final.log` | `f9cc08e40ccfa68ec7de28125ebfe1ffe91c759144bad10a674d113c9d96e15d` |
| `/tmp/kandev-tool-terminal-adapter-final.log` | `83bbb37122d1d5ac2eb5e85802862f628cf72e62abefd24643541ae45c2f7700` |
| `/tmp/kandev-tool-terminal-integrated.log` | `1a0ac23f2715f2ace272fa658748ee3da4bd2338f3d66317c8b55a69c38327cb` |
| `/tmp/kandev-tool-terminal-conformance.log` | `bf87c625acb583b3d6a10dc4a98215442d74719ad78050097f6f86cf4311838f` |
| `/tmp/kandev-tool-terminal-sqlguard.log` | `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855` |
| `/tmp/kandev-tool-terminal-lint.log` | `81ff2de3b9f14a98b26c0230721d14a4c3016dbffaa361f3ef077b13a9aae4b2` |
| `/tmp/kandev-tool-terminal-lint-final.log` | `1a7d0e45f35b1c9f8066edeb8fe1709ffaf26389403a0ef1c9dc2a870e321f39` |
| `/tmp/kandev-tool-terminal-lint-green.log` | `e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47` |

Exact commands and bounded environment are retained in `/tmp/kandev-tool-terminal-verify.sh` and `/tmp/kandev-tool-terminal-final.sh`. Both-dialect runs use the disposable PostgreSQL 16 cluster on loopback port 55488 and isolated test schemas. The owned server stopped normally after validation. No test, compiler, linter or database process from this packet remains.
