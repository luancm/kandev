# Warning stream acceptance

Status: predecessor cancelled, ownership released. Primary recovered the partial tests at 06:53 UTC and completed the planned focused race command as handle 42492, exit 1. Only the two thinking-streaming subcases fail at errand_warning_turn_test.go:77: plain string("thinking") is compared with models.MessageType("thinking"). Correct the expected type, then rerun to exercise subsequent assertions; this is not behavioral RED evidence. No checks remain running. Existing dirty changes are preserved. The next backend writer additionally owns these two test files and this handoff for the small correction/final verification. No production warning defect has been established.

Read the backend guidance, TDD skill/reference, task work order, turn-attribution and consumer-ownership handoffs, and recovery-settlement-authority ADR. Existing warning retry/idempotence tests dispatch actual events but mock message persistence; existing turn-isolation tests use SQLite/task services but bypass event dispatch. Streaming methods in the shared service-backed fixture still inherit mock implementations, so new coverage must supply real persistence at those methods.

Initial headroom: root 28 GiB free, `/dev/shm` 6.9 GiB free. Stop heavy work below 1.5 GiB root free. Serialize checks, retain and poll every process handle; no edits during checks. Use `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB` and `go -C apps/backend test -trimpath -p 1`.

Planned acceptance command: `GOTMPDIR=/dev/shm/kandev-errand-remediation-0n6s5t7i GOMAXPROCS=2 GOMEMLIMIT=3GiB go -C apps/backend test -trimpath -p 1 -race ./internal/orchestrator -run 'TestModelSelectionWarning|TestHandleSessionModelSelectionWarning|TestResumeTodos' -count=1`. Changed-Go lint will run once for the owned test files after tests pass. No SQL production changes, SQL guard/conformance, broad suite, live runtime, index, commit or push operations are in scope.

Primary final acceptance: corrected typed thinking expectation and all planned handler/stream cases passed in final orchestrator race handle81577 (12.062s). No production warning defect found.
