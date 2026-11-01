---
id: "25-current-dark-media"
title: "Complete current dark desktop and phone evidence"
status: pending
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

Repairs review finding(s) task16 acceptance at baseline `6400a265bb8a0a35fb702eced2aff1eeb81023d9`. This work order is part of [the repair package](plan.md). The [shared recovery design](../../specs/tasks/system-design/errand-recovery-and-delivery.md) resolves cross-packet contracts and takes precedence over historical diagnostic proposals. No production changes have been made for this packet.

## Low-level implementation and regression plan

No capture is performed or claimed by this design. Existing /tmp/kandev-errands-dark.scyGKz/PROVENANCE.json names c913e3873cbd207cc60249702014c065fd6d39f7 and the old /tmp/.../harness scripts. Those media and the old source copy cannot demonstrate the current token-count/bar or these repairs. Existing relay failure history is a concrete rehearsal risk, not a reason to replace the real submit behavior with a successful-looking DOM mock.

Task16 already authorizes an isolated current-task-revision demo and explicitly synthetic telemetry. Its source selection overrides the generic product-demo-seeding origin/main recipe for THIS experimental fork feature; record that override in provenance and never market the capture as an upstream-main/shipped-default feature. Do not fetch/checkout/change shared Git state in an implementation errand. Parent owns preparing any clean isolated source snapshot/worktree and source provenance at the later capture stage.

Capture work order must invoke product-demo-seeding before product-video-capture and read their full isolation/capture references then. Rehearse against newly built candidate frontend/backend with no old service reuse. Build/source provenance must identify exact repair candidate SHA; if source is uncommitted, record base SHA plus complete source diff/fixture digest and do not call it a clean SHA-only capture. Do not invent a commit or publication authorization to satisfy provenance.

Reproducible seed/story obligations:
1. Fresh take-owned temp HOME/KANDEV_HOME_DIR, DB, checkout/repo, ports, browser profile and artifact root. Mock agent/provider registration and absence of real credentials verified in logs. Never developer's main DB/instance; no --copy-db.
2. Separate native desktop and phone takes; dark theme persisted through actual settings/fixture contract. Use existing realistic fictional release-review scenario, configured compatible mock profile and actual MCP start-returned child ID.
3. Parent starts a gated long-running child through real tool path. Wait for child RUNNING, seed child metadata with a representative measured sample (68k/100k) using seedErrandContextWindow/seedTaskSession while preserving lifecycle fields. Seed parent different context and optionally a separate unmeasured/completed child. Disclose readings as synthetic demo telemetry.
4. Open actual pill -> show token counts AND thin occupancy bar in dark panel/drawer. Show active-first hierarchy without stale/duplicate inline inventory. Then close panel and submit a real parent follow-up while the gate remains closed; show visible agent response and separately verify child remains RUNNING. This is the specifically missing promise, not just an enabled composer screenshot.
5. Open actual child, show full child conversation/header, visible Back, phone visible actions; return to parent. Release gate, show actual delivery and immutable finished card. Reopen panel to show retained context/counts and completed access.
6. Causal trace/relay evidence: parent objective dispatch, child gate still pending during follow-up, actual parent follow-up submission/provider response, result settlement/delivery. Repair capture relay compatibility outside production source if necessary; no fake message widgets or direct DOM content replacement.
7. Desktop+phone screenshots must visibly include context readout; films must show follow-up/result story. Use the capture skill's required current dimensions/fps/continuous raw-master/camera rules when issuing media work order (desktop source 3840x2400@25 -> delivery 1920x1200@25; native phone 1290x2796@25). A desktop crop cannot substitute for native phone UI.
8. Retain raw masters, sampled inspected frames, posters, candidate WebM/MP4, semantic action timestamps/bounds/pointer tracks, fixtures, source/seed digests, codecs/ffprobe receipts and cleanup logs under one fresh root. Inspect playback/legibility, touch/navigation, safe-area/footer and no overflow. Cleanup only take-owned PIDs/paths and prove ports/processes stopped/released. No landing/public asset promotion is part of capture.

Test gate is a rendered rehearsal failing if tokens/bar, real follow-up response, still-running child or final delivery is absent. Implement the scratch runner and exact commands below after allocating the fresh root. Never execute the historical run-capture.sh against its historical source. Capture acceptance follows successful code repair and browser validation.

UI-CONTEXT-01:
~~~
Desktop grouped panel           Native phone drawer
Active                         +--------------------------------+
  Main                          | Errands                    [x] |
    Audit release checklist     | Active / Main                  |
    Reviewer / Running          | Audit release checklist        |
    Context: 68k / 100k tokens   | Reviewer / Running             |
    [===========-----] [Open]   | Context: 68k / 100k tokens      |
                                | [===========-----]         >   |
Parent follow-up while running  |--------------------------------|
You: Check the rollback too.     | [       Stop all errands     ] |
Agent: I will include it.        +--------------------------------+
[Errands: 1 running]             parent composer remains usable after dismiss
~~~

Counts/copy here illustrate existing localized strings, not new copy requirements. Keep task16 in_progress until these final artifacts and process-release receipts exist.

### Candidate snapshot and provenance

The primary creates `$CAPTURE_ROOT/source` as a detached local clone of the current task HEAD, applies `git diff --binary HEAD` there, and copies the sorted non-ignored untracked files with their modes and symlinks preserved. All Git checkout/apply operations target that isolated clone; the shared checkout is read-only. Verify the clone's tracked patch and untracked inventory against the source before building. Record `SOURCE_BASE_SHA` from the task's actual HEAD; never commit merely to obtain capture provenance. Keep the tracked patch, untracked path/mode/kind/length/hash manifest, fixture bytes and the final complete digest under `provenance/`.

Compute `SOURCE_DIFF_SHA256` as SHA256 of the versioned prefix `errands-candidate-v1`, base SHA, tracked binary patch and sorted untracked path/mode/kind/length/raw-byte records, separated unambiguously with NULs. Recompute before each take and reject concurrent candidate changes. Earlier design-turn digests are not capture receipts. `LANDING_REPO` must resolve to an existing checkout with both encoder and camera scripts; record the path and revision and run the profile tests below before encoding. No capture fallback silently changes the requested dimensions or uses old assets.

### Scratch harness implementation

This work order implements these files only under its newly allocated $CAPTURE_ROOT/harness/, then copies the three Playwright files into the isolated $SOURCE_ROOT using the exact temporary paths listed below. No harness implementation belongs in the shared repository.

| Scratch file | Contract |
| --- | --- |
| harness/scenario.json | Versioned fictional Harborlight release-review narrative, exact prompts/responses, one running child, used=68000, size=100000, a distinct parent sample, expected row labels, exact record counts, and synthetic_telemetry: true; hash the exact bytes before each take. |
| harness/errands-media-relay.js | ACP stdio wrapper around the freshly built mock-agent; preserve JSON-RPC framing and forward all non-target messages unchanged. Map only the known initial parent prompt, child gate, real UI-submitted parent follow-up, and result notification to deterministic mock-agent e2e scripts. Record mode, session ID, prompt digest, and tool/result evidence to newline-delimited JSON. Never inject browser messages, alter the rendered DOM, or bypass the actual MCP start, composer submit, provider response, child result, or delivery paths. |
| harness/errands-media.capture.ts | Shared runErrandsMediaCapture({ api, seed, backend, testInfo, formFactor, mode }) function. Seed one fictional task/profile/repository; launch a headed Chrome-for-Testing app window through Playwright CDP; set the real persisted dark theme; start/stop an X11 FFmpeg recorder in record mode; drive the desktop or native-phone route; write device-pixel stills, semantic action metadata, seed counts, API state snapshots, relay evidence, logs, and cleanup receipts. In finally, release the gate, stop only owned processes, restore the isolated mock-agent executable and backend environment, and write an incomplete-take record on failure. |
| harness/desktop-errands-media.capture.spec.ts | One chromium project spec using test from the existing e2e/fixtures/test-base; request only apiClient, seedData, backend, and test info, then call the shared capture function with formFactor="desktop". |
| harness/mobile-errands-media.capture.spec.ts | One mobile-chrome project spec using the same worker fixtures and shared function with formFactor="phone"; set CDP mobile metrics and touch emulation to the required 430×932 CSS / DPR 3 profile, and exercise actual touch input. |
| harness/errands-media.sh | Strict stage runner with prepare, build, rehearse desktop|phone, probe, record desktop|phone, inspect, encode, and cleanup subcommands. It validates required environment, source/diff/fixture hashes, free ports/display/profile paths, secret absence, logs every command and PID, and exits nonzero on any acceptance failure. It never operates on the shared task checkout or the old capture root. |

The temporary E2E paths are apps/web/e2e/helpers/errands-media.capture.generated.ts, apps/web/e2e/tests/session/errands-media.capture.spec.ts, and apps/web/e2e/tests/session/mobile-errands-media.capture.spec.ts. Keep those literal paths in $CAPTURE_ROOT/harness/capture-excludes and pass that file through command-local GIT_CONFIG_COUNT=1, GIT_CONFIG_KEY_0=core.excludesFile, and GIT_CONFIG_VALUE_0=$CAPTURE_ROOT/harness/capture-excludes. Do not change .git/info/exclude. Save a source copy of each staged spec before the take and remove all three plus the excludes file during cleanup.

The mock-agent override belongs only in the parent-prepared isolated $SOURCE_ROOT/apps/backend/bin/mock-agent: copy the just-built binary to $CAPTURE_ROOT/tools/mock-agent.real, install the scratch relay at the ignored binary path, and restore the original before source cleanup/status comparison. The E2E backend prepends that isolated apps/backend/bin to PATH, so this uses the fixture's actual mock-agent selection. Never overwrite the shared task worktree binary.

### Environment contract

| Variable | Required value and validation |
| --- | --- |
| KANDEV_REPO | Read-only path to the task worktree, used only to compare the candidate receipt; it must not equal $SOURCE_ROOT and receives no writes. |
| SOURCE_ROOT | Parent-prepared isolated snapshot of the exact current task candidate; it must not be /tmp/kandev-errands-dark.scyGKz/source or the shared workspace. |
| SOURCE_BASE_SHA | Candidate Git base, currently observed as 6400a265bb8a0a35fb702eced2aff1eeb81023d9; recompute for the execution snapshot. |
| SOURCE_DIFF_SHA256 | Complete tracked-plus-untracked candidate digest using the algorithm above; recompute before prepare and fail if it changes afterwards. |
| CAPTURE_ROOT | Fresh mode-0700 root from mktemp; all harness files, logs, profile directories, DB/temp roots, raw masters, screenshots, QA, delivery candidates, and teardown receipts stay below it. |
| LANDING_REPO | Explicit verified checkout containing both scripts/product-loop-camera.mjs and scripts/product-loop-encoder.mjs; required before encoding. Do not guess by checkout name or promote into it. |
| KANDEV_CAPTURE_MODE | rehearse or record; rehearsal proves the complete rendered path without FFmpeg, while record starts X11 capture before the first story action. |
| KANDEV_CAPTURE_FORM | desktop or phone; determines native CSS viewport, DPR, Xvfb size, recorder size, browser profile, and output paths. |
| E2E_PORT_OFFSET | Free integer 0–29, checked against all ports derived by apps/web/e2e/fixtures/backend.ts; desktop and phone use distinct offsets and run sequentially. Do not reuse old 27/28 values without a fresh listener check. |
| DISPLAY / KANDEV_CAPTURE_DISPLAY | New take-owned Xvfb display allocated by the runner with -displayfd; use 3840×2400×24 for desktop or 1290×2796×24 for phone. |
| KANDEV_CAPTURE_BROWSER_PROFILE / KANDEV_CAPTURE_CDP_PORT | New per-take Chrome user-data directory and free CDP port, recorded before launch. |
| KANDEV_CAPTURE_GATE / KANDEV_CAPTURE_RELAY_LOG / KANDEV_CAPTURE_REAL_AGENT / KANDEV_CAPTURE_SCENARIO_FILE | Absolute paths under $CAPTURE_ROOT; the original mock-agent path points at the preserved current build, and the gate is a new per-take file that is absent until release. |
| HOME, TMPDIR, XDG_CACHE_HOME, XDG_CONFIG_HOME | Per-take directories below $CAPTURE_ROOT. TMPDIR forces the worker-scoped E2E fixture's fresh SQLite DB, local repositories, and worktrees below the take root; the fixture's own HOME and KANDEV_HOME_DIR are recorded from backend.tmpDir and must also remain below $CAPTURE_ROOT. |
| KANDEV_E2E_BIN | Absolute $SOURCE_ROOT/apps/backend/bin/kandev path built in the current candidate snapshot. The worker serves its freshly built $SOURCE_ROOT/apps/web/dist; record both paths. |

Set isolation values only in the child-process environment; do not overwrite the primary shell's HOME or other global paths. Launch each child command with a strict allowlist (env -i) containing only the above values plus PATH, LANG, and required display/Playwright loader variables. Assert that GitHub/Jira/provider tokens and production Kandev URLs are absent. Enable Errands through the test fixture's backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" }); do not rely on a global profile/default or modify runtime flags. The standard fixture supplies KANDEV_E2E_MOCK=true, isolated ports, SQLite, repositories/worktrees, and backend process ownership.

### Execution sequence

The primary allocates the fresh root and prepares its isolated `source/` snapshot from the current task candidate, including tracked and untracked changes. Compute provenance before introducing the temporary capture files. Implement the six scratch harness files above, then use this stage interface. Source and landing paths below are execution-time inputs resolved and checked before `prepare`, not permission to reuse an old capture.

    export KANDEV_REPO=/home/luancm/.kandev/tasks/important-this-proje_45kr4xxl/kandev
    CAPTURE_PARENT=/tmp
    test -z "$TMPDIR" || CAPTURE_PARENT="$TMPDIR"
    export CAPTURE_ROOT="$(mktemp -d "$CAPTURE_PARENT/errands-repair-media.XXXXXX")"
    export SOURCE_ROOT="$CAPTURE_ROOT/source"
    chmod 700 "$CAPTURE_ROOT"
    mkdir -p "$CAPTURE_ROOT"/{harness,tools,provenance,logs,raw,screenshots,semantic,configs,delivery,proofs,qa,teardown,takes}

    # Parent prepares source/ and supplies verified SOURCE_BASE_SHA,
    # SOURCE_DIFF_SHA256 and LANDING_REPO after the checks below.
    bash "$CAPTURE_ROOT/harness/errands-media.sh" prepare

prepare verifies the source base/diff and scratch fixture hashes, snapshots the scenario and exact temporary-spec bytes, validates LANDING_REPO markers, proves required provider credentials are unset, confirms no resource path points at a previous capture or personal Kandev home/database, and writes the resource manifest. It does not launch Kandev or modify the repository.

Build from the isolated candidate, following the current web E2E README's build order:

    pnpm --dir "$SOURCE_ROOT/apps" install --frozen-lockfile
    make -C "$SOURCE_ROOT/apps/backend" build
    pnpm --dir "$SOURCE_ROOT/apps/web" run build:e2e
    make -C "$SOURCE_ROOT/apps/backend" e2e-plugin-ui
    make -C "$SOURCE_ROOT/apps/backend" e2e-plugin-package
    make -C "$SOURCE_ROOT/apps/backend" build-e2e-delivery-fixture
    bash "$CAPTURE_ROOT/harness/errands-media.sh" build

The runner's build stage checks artifact timestamps/freshness, verifies the backend binary and dist path resolve inside $SOURCE_ROOT, records SHA-256 for kandev, mock-agent, agentctl, web bundle, plugin package, E2E delivery fixture, and the candidate source manifest, and refuses stale build output. The direct commands above are run once; build is a validation stage, not a second build.

Run complete recorder-free rehearsals first, one process and fresh worker database per form factor:

    bash "$CAPTURE_ROOT/harness/errands-media.sh" rehearse desktop
    bash "$CAPTURE_ROOT/harness/errands-media.sh" rehearse phone

Each rehearsal launches a fresh Xvfb and a headed app-mode Chrome through CDP, runs the matching Playwright project/spec with --retries=0, records proof screenshots and protocol/API assertions but no video, then tears down its test DB/profile/display. Rehearsal state is discarded; it is never the record database. The wrapper selects a newly checked free E2E_PORT_OFFSET and display for each run and records their exact values.

The record takes are a new run after both rehearsals pass; they cannot inherit rehearsal DBs, browser profiles, gates, cached client state, or task rows:

    bash "$CAPTURE_ROOT/harness/errands-media.sh" probe
    bash "$CAPTURE_ROOT/harness/errands-media.sh" record desktop
    bash "$CAPTURE_ROOT/harness/errands-media.sh" record phone
    bash "$CAPTURE_ROOT/harness/errands-media.sh" inspect
    bash "$CAPTURE_ROOT/harness/errands-media.sh" encode
    bash "$CAPTURE_ROOT/harness/errands-media.sh" inspect
    bash "$CAPTURE_ROOT/harness/errands-media.sh" cleanup

The exact E2E invocations performed by rehearse/record are:

    pnpm --dir "$SOURCE_ROOT/apps/web" e2e:raw --headed --project chromium tests/session/errands-media.capture.spec.ts --retries=0
    pnpm --dir "$SOURCE_ROOT/apps/web" e2e:raw --headed --project mobile-chrome tests/session/mobile-errands-media.capture.spec.ts --retries=0

e2e:raw pins one worker. The helper launches the browser from Playwright's installed Chrome-for-Testing executable with --disable-infobars --test-type --hide-scrollbars --force-device-scale-factor=<DPR> --window-position=0,0 --window-size=<CSS width>,<CSS height> --app=<isolated task URL> --remote-debugging-port=<CDP port> --user-data-dir=<take profile>. It attaches using chromium.connectOverCDP, applies a new mobile CDP device-metrics/touch profile for phone, and drives real pointer/touch input. FFmpeg records the full physical X display with x11grab, -draw_mouse 0, 25 fps, no audio, and a visually lossless raw profile; it starts before the first recorded action and stops cleanly with q before Chrome closes. Playwright recordVideo is not used as a source master.

### Capture/encoding contract

Raw masters are one continuous 1x take with no crop, camera, speed change, concat, audio, or internal cuts. Use desktop CSS 1920×1200 at DPR 2 on a 3840×2400 Xvfb display; use native-phone CSS 430×932 at DPR 3 on a 1290×2796 Xvfb display. Extract desktop and phone screenshots at device scale, not from a desktop crop. Hide only the capture-only cursor/touch overlay for stills; never hide product UI. The video pointer/touch overlay is the one permitted capture treatment, stays synchronized with real CDP input, and emits dense semantic samples with timestamps, target bounds, target glyph bounds, independently measured pointer/touch glyph bounds, and motion-inclusive visibility intervals.

Before recording, probe measures FFmpeg realtime capacity at both exact source profiles and archives complete logs. Reject a run unless raw decoding proves the full physical dimensions, 25 fps constant cadence, 40ms frame timestamps with no gaps/duplicates, zero duplicated/dropped FFmpeg frames, no audio, clean four-edge fill, and no browser automation banner in the every-frame perimeter audit. Preserve raw masters and all source logs.

The exact desktop deliverable requested by Task 25 is 1920×1200 at 25 fps from 3840×2400. Use only the tested landing-editorial profile with formFactor: "landing", cameraProfile: "landing-editorial", both focusTrack and pointerTrack, maximum zoom ≤2.0, and a camera path derived from semantic events. Native phone remains 1290×2796 at 25 fps using formFactor: "mobile", its tested camera cap, and its standard mobile loop. Before encoding, require the resolved landing checkout to pass node --test scripts/product-loop-camera.test.mjs scripts/product-loop-encoder.test.mjs and prove its tests recognize landing-editorial; if that tested profile is absent, stop before encoding rather than inventing a 1920×1200 profile. Encode with node "$LANDING_REPO/scripts/product-loop-encoder.mjs" "$CAPTURE_ROOT/configs/desktop-errands.json" and the matching phone config. Outputs remain only under $CAPTURE_ROOT/delivery/ as WebM, MP4, and WebP.

inspect runs the reference ffprobe stream/frame probes for each raw and encoded file, checks dimensions, codec, constant 25 fps, timestamp cadence, no audio, file size, durations, and decoded pixels; hashes every input/output/config/metadata/proof. Extract 10%, 25%, 50%, 75%, and 90% frames, all panel/child/result peaks, a frame-indexed full raw perimeter/contact-sheet audit, pointer containment samples, and settled pointer-free screenshots. Watch both entire loops at normal speed and 0.5x; inspect meaningful original frames at 100% display scale and phone safe-area/footer/overflow. Test the staged WebM/MP4/WebP variants in a dedicated local player page below $CAPTURE_ROOT/qa/player/, including responsive desktop-vs-native-phone selection, playback/loop, poster, and reduced-motion behavior. The dedicated player is QA-only and is not a production page. Reject cropped text/bar, pointer teleport/duplication, stale or accumulated state, any visible host/fixture copy, unreadable parent response/result, camera motion away from the pointer, and any phone overflow.


## Dependencies and ownership

[Task 24](task-24-mixed-parent-browser-acceptance.md)

The listed production and regression files are exclusive ownership for this packet. Other errands share the checkout: preserve their changes, do not stage/commit or edit the shared manifest, and do not delegate further. Changes outside the listed boundary require integration before editing. Use repository TDD: behavioral RED, minimal GREEN, final relevant checks. Existing diagnostics are evidence, not replacement acceptance.

## Parallelism

`sequential`. The integrating session may overlap disjoint read-only preparation, but grants one writer for each shared schema, repository, orchestration or browser file.

## Results

Pending. Record each final command, observed result, test count and log path; distinguish skips, infrastructure failures and reproduced defects from passes.
