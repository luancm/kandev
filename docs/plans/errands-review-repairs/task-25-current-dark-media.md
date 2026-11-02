---
id: "25-current-dark-media"
title: "Complete current dark desktop and phone evidence"
status: done
wave: 25
depends_on:
  - "24-mixed-parent-browser-acceptance"
plan: "plan.md"
requirements:
  - REQ-TASKS-ERRANDS-CONTEXT-001
acceptance_criteria:
  - AC-TASKS-ERRANDS-CONTEXT-001.1
  - AC-TASKS-ERRANDS-CONTEXT-001.4
system_design:
  - ../../specs/tasks/system-design/errands.md
  - ../../specs/tasks/system-design/errand-recovery-and-delivery.md
  - ../../specs/tasks/system-design/errand-context-window.md
---

# Task 25: Complete current dark desktop and phone evidence

## Scope and authority

Demonstrate the implemented Errands feature from this task branch using an isolated fictional release-review scenario. The user clarified that the existing Kandev checkout is the source; a separate landing-site repository is not a prerequisite for this feature demo. This replaces the earlier promotional-camera/encoder prescription, retained in Git history, with ordinary native desktop and phone browser recordings and screenshots. No production asset publication is included.

The existing seeding isolation and truthfulness rules apply. Source is the current feature branch, not upstream main: record the exact SHA and build hashes and label the result as a branch demonstration with mock agents and synthetic context telemetry. No real providers, credentials or developer database may be used. No Xvfb, landing camera profile or external encoder checkout is required for Playwright's browser recording.

## Story and acceptance

1. Create a fictional Harborlight release-checklist task and a compatible controlled profile in a fresh fixture-owned HOME, database, repository and ports.
2. The parent starts a child through the actual MCP errand tool. Hold the child with a causal file gate after real progress output.
3. Seed clearly disclosed synthetic context telemetry (child 68k/100k, parent 23k/100k), preserving lifecycle identity. Show the dark desktop panel and native phone drawer with counts and the occupancy bar.
4. Submit a real parent follow-up while the gate is closed. Verify its visible response and persisted agent message while the child remains running.
5. Use actual Open/Back controls to inspect the full child transcript. Release the gate, verify actual result delivery and a truthful parent acknowledgment, and retain access to the finished child.
6. Save browser WebM recordings, screenshots and provenance. Inspect actual dimensions and meaningful frames for readable content, dark theme, native phone composition and overflow. State that these are straightforward recordings, without claiming the superseded promotional-camera specification.
7. Stop only take-owned processes, prove ports are released, and remove disposable databases/source copies after retaining the harness and evidence needed to reproduce the demonstration.

## UI preview

```text
Desktop parent conversation         Native phone parent conversation
  Errands panel: Active               Errands bottom drawer: Active
  Audit release checklist             Audit release checklist
  Context 68k / 100k + occupancy       Context 68k / 100k + occupancy
  Open -> child -> Back               Tap -> full-screen child -> Back

Parent follow-up receives a response while the child remains running.
After gate release: recorded result disclosure and parent acknowledgment.
```

## Implementation and verification

Use the existing worker-scoped E2E fixture and API helpers. Adapt the retained capture harness to current selectors and mock-agent framing; never reuse its old binaries or source revision. Keep all harness changes outside the shared checkout. The relay translates only the known fictional prompts and an explicit errand-result notification into controlled mock scripts; unrelated frames pass through unchanged. Do not replace DOM content or invent successful application behavior.

Allocate a fresh artifact root and an isolated source snapshot from current HEAD. Byte-compare it with the clean worktree and preserve verified build provenance. Rehearsal and recording use separate fresh fixture databases; desktop and phone run sequentially under the shared validation lock. Record exact commands, pass/fail results, artifact paths, source/build/harness hashes and cleanup receipts below. No source, helper-binary or shared Git configuration mutation is permitted in the user's worktree.

## Dependencies and parallelism

Task24 is complete. `sequential`; the primary owns capture and status integration. The artifacts and cleanup evidence below close Task26's media dependency and original Task16's remaining acceptance.

## Results

Completed on 2026-11-02 from source `065ebb42c735a9effb2b580b5a8de90a13d3bd31`. Every tracked source file in the disposable snapshot was byte-compared with the clean worktree; copied current-branch build outputs and the capture harness have SHA-256 receipts in `PROVENANCE.json`. The snapshot was not freshly rebuilt, and no old capture runtime was used. The initial fixture setup attempts stopped before recording because dependency symlinks and packaged plugin outputs were missing from the snapshot; those setup inputs were corrected before the successful rehearsals.

Final rehearsals and recordings ran sequentially under `/tmp/kandev-errands-repair-build.lock`, with one worker and zero retries/skips. Each invocation used `python3 /tmp/kandev-errands-demo.s63h2rg5/harness/run-take.py <desktop|phone> <rehearsal-final|record>`. Exact commands and isolated environment/port selections are retained in each take's `INVOCATION.json`.

| Check | Result | Receipt below `/tmp/kandev-errands-demo.s63h2rg5/takes/` |
| --- | --- | --- |
| Final desktop rehearsal | 1 passed, 23.8s | `desktop-rehearsal-final/logs/run.log` |
| Final phone rehearsal | 1 passed, 23.0s | `phone-rehearsal-final/logs/run.log` |
| Desktop recording | 1 passed, 25.8s | `desktop-record/logs/run.log` |
| Phone recording | 1 passed, 23.1s | `phone-record/logs/run.log` |

Both takes exercised actual MCP admission, progress, parent follow-up while the gated child remained running, child Open/Back, result delivery, and completed-history access with retained context counts. Seed evidence records exactly one task, two sessions and one errand, mock registration and synthetic 68k/100k child context. The relay's acknowledgment reports the identified release blocker without claiming a file edit. No real-provider or measured-token claim is made.

The [local player](../../../apps/web/e2e-artifacts/errands-demo-065ebb42c7/index.html), [desktop MP4](../../../apps/web/e2e-artifacts/errands-demo-065ebb42c7/desktop.mp4) and [phone MP4](../../../apps/web/e2e-artifacts/errands-demo-065ebb42c7/phone.mp4) are retained in the ignored workspace directory `apps/web/e2e-artifacts/errands-demo-065ebb42c7/`. The same directory contains original WebM recordings, dark screenshots, sampled frames, source/build/harness provenance, hashes, seed evidence, probes and playback receipts. These are local artifacts, not Git-tracked or published assets. The complete harness and invocation logs remain at `/tmp/kandev-errands-demo.s63h2rg5/`.

Desktop video is 1440×960 at 25 fps for 16.8s; native phone video is 392×726 at 25 fps for 14.48s, recorded from the 393×727 Pixel 5 viewport. MP4 encoding preserves the complete recording without cropping or speed changes and uses H.264/yuv420p with fast-start metadata and no audio. All MP4 frames decoded successfully; both MP4 and WebM versions played to `ended=true` in Chromium (420 desktop and 362 phone frames). Screenshots and sampled video frames were inspected for dark styling, readable counts/bar, native phone layout, child navigation and delivered/completed history. These are straightforward browser demonstrations, not promotional camera-processed films.

Cleanup verified zero owned runtime processes and listening ports before deleting only the capture's disposable source copy, helper binary, HOME/database, cache and per-take runtime directories. Harness, logs, media and receipts remain; `CLEANUP.json` lists every removed path. Disk checkpoint after cleanup: 59% used, 48 GiB available. No developer session, database, shared cache, live instance or shipped default was changed.
