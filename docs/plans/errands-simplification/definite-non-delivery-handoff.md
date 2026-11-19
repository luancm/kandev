# Task05 non-delivery integration handoff

The repair preserves committed non-delivery proof through trusted-parent `PromptTask`, initial admission, initial Send Now and reopened Errand recovery. Only a new, empty, unreferenced reservation may roll back on exact refusal. Acceptance, uncertainty, referenced/reused turns, cancellation and successor ownership retain their existing protection. No terminal decision, replay, schema or callback API was added.

Six production Go files changed by +166/−17 lines relative to the merged index. Exact receipt ownership now includes dispatch attempt and terminal inbox/effect fences; existing admission and claim owners consume that proof. The design’s conditional-settlement field list was aligned without changing lock order. The [work order](task-05-definite-non-delivery.md) retains the genuine public RED, byte-exact production restoration, intermediate failures and bounded decisions.

Parent accepted repository, caller v1, production v2 and the mechanical v3 test-style delta with no findings. Reversing the switch-only test change reproduces the accepted v2 file hash; all cases and assertions remain intact. The final 13-file Go manifest is `/tmp/kandev-upstream-f52-verification.0g9keazg/task05-caller-frozen-v3.json`, SHA-256 `179848f0e1c3f77cb7121c6b91cf2f1db6cfb5eecfef75c1ef955e5446e0b962`.

| Check                                   | Actual result                                                                                                    |
| --------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| Exact repository receipt/retention race | 3 top-level / 47 entries; 20 SQLite + 20 PostgreSQL leaves                                                       |
| Public caller and ownership race        | 14 top-level / 49 entries                                                                                        |
| Post-style initial-admission race       | 1 top-level / 6 entries; all five original leaves                                                                |
| Merged integration race                 | 130 top-level / 499 entries; agentctl compiled but matched no selector cases                                     |
| Store conformance                       | 12 top-level / 590 entries; 245 SQLite + 245 PostgreSQL + 10 engine-independent leaves                           |
| SQL guard and normal optimized compile  | Passed; backendapp, orchestrator, Errands and runtime lifecycle compiled without the race inlining accommodation |
| Go lint                                 | 22 changed files / five packages; extended run and unchanged normal wrapper both passed with zero issues         |
| Web static checks                       | Typecheck and four-file E2E lint passed                                                                          |
| Managed recovery controls               | Desktop 3/3 and phone 3/3, zero retries; phone reused the same 708 verified artifact hashes                      |

These suite counts include nested entries and overlap; they are not a unique-test total. All executed cases above had zero skips. Race tests used the recorded orchestrator-only `-gcflags=github.com/kandev/kandev/internal/orchestrator=-l` resource accommodation; normal optimized compilation was separate. Disposable PostgreSQL 16 actually ran on loopback 55488 with isolated schemas, then stopped with owner/listener/postmaster absence verified.

Raw evidence is under `/tmp/kandev-upstream-f52-verification.0g9keazg/`; `task05-command-receipts.json` indexes commands, handles and outcomes. The original lint timeout, test-style finding, inherited PostgreSQL lock setup and build-only Makefile flag correction remain separate receipts. The final managed build verified effective `GOFLAGS=-p=1`, `GOMAXPROCS=2` and `GOMEMLIMIT=3GiB`. Its prior artifacts were preserved in a verified 1,405-member archive before rebuilding; no benchmark or pilot evidence was overwritten.

The Errand reopen case closes the old SQLite pool and reconstructs repository/service owners; it does not prove full application crash recovery. Provider/runtime fixtures are controlled, and browser providers are mocks. The separate performance acceptance gate remains open.

Cleanup removed only two proven inactive failed-build scratch directories, retaining provenance and all current caches, modules, artifacts and raw evidence. Allocated removal was 459,509,760 bytes; observed free-space increase was 459,468,800 bytes. Readable cwd/fd/map checks found no references; protected-process visibility limits are recorded. Final formatting/spec validation, source manifests and ownership release are recorded in the final validation and hygiene receipts. Parent accepted the implementation and scoped verification after checking the final source and receipt hashes. The writer performed no shared Git staging, commit or push; parent owns integration and publication. Performance acceptance remains open.
