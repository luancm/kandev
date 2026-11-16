import { createHash, randomUUID } from "node:crypto";
import { execFile, execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { promisify } from "node:util";
import { expect, type Browser, type Locator, type Page, type TestInfo } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import {
  attachGatewayTrafficCapture,
  summarizeGatewayTraffic,
  type GatewayTrafficSummary,
} from "./ws-traffic";

const execute = promisify(execFile);
const FIXTURE_MARKER = "KANDEV_E2E_ERRAND_HISTORY_FIXTURE:";
const COMPLETED_SESSION_LIMIT = 1_000;

type FixtureTaskInput = {
  task_id: string;
  parent_session_id: string;
  ordinary_session_ids: string[];
  agent_profile_id: string;
  finished_count: number;
  include_outstanding: boolean;
};

type FixtureTaskProof = {
  task_id: string;
  finished_count: number;
  ordinary_session_count: number;
  outstanding_control_count: number;
  recovery_attention: boolean;
  pending_result_state: string;
};

export type HistoryScenario = {
  taskId: string;
  layoutScopeId: string;
  title: string;
  parentId: string;
  ordinaryIds: string[];
  finishedCount: number;
  hasOutstandingControls: boolean;
};

type ExerciseOptions = {
  page: Page;
  browser: Browser;
  api: ApiClient;
  seed: SeedData;
  backend: BackendContext;
  testInfo: TestInfo;
  finishedCount: number;
  mobile: boolean;
};

type BrowserCapture = {
  documentStartedAt: number;
  fetchCalls: Array<{ url: string; timestamp: number; caller: string }>;
  wsCalls: Array<{
    url: string;
    action?: string;
    sessionId?: string;
    taskId?: string;
    caller: string;
  }>;
  panels: {
    mounts: PanelEvent[];
    removals: PanelEvent[];
    identityTransitions: PanelEvent[];
  };
};

type PanelEvent = {
  sessionId: string;
  timestamp: number;
  identityKnown: boolean;
  role?: string;
};

type E2ESession = { id: string; role?: string; state?: string };
type E2EState = {
  taskSessions?: { items?: Record<string, E2ESession> };
  messages?: { bySession?: Record<string, unknown[]> };
  turns?: { bySession?: Record<string, unknown[]> };
  taskSessionsByTask?: { itemsByTaskId?: Record<string, E2ESession[]> };
  environmentIdBySessionId?: Record<string, string>;
};
type E2EStoreWindow = Window & {
  __KANDEV_E2E_STORE__?: { getState(): E2EState };
};

type HttpResponseRecord = {
  url: string;
  status: number;
  encodedResponseBodyBytes: number | null;
  responseHeaderBytes: number | null;
  decodedResponseBodyBytes: number | null;
  encodedBodySizeSource: "request-size" | "content-length" | "unavailable";
  responseBodyCaptureError?: string;
  contentLengthHeader: string | null;
  contentEncoding: string | null;
  resourceType: string;
  method: string;
  timing: Record<string, number>;
  callerStack: string;
};

type CapturedTransport = {
  browser: {
    capture: BrowserCapture;
    taskId: string;
    taskSessionCount: number;
    ordinarySessionCount: number;
    errandSessionCount: number;
    resourceEntries: Array<Record<string, string | number>>;
    totalReconciledRowsAcrossTasksInStore: number;
  };
  responses: HttpResponseRecord[];
  gateway: GatewayTrafficSummary;
  gatewayFrames: WireDetail[];
  wsCallers: BrowserCapture["wsCalls"];
};

type WireDetail = {
  direction: "sent" | "received";
  action?: string;
  taskId?: string;
  sessionId?: string;
  bytes: number;
  timestamp: number;
  resultRowCount?: number;
};

const HISTORY_PAGE_SIZE = 25;
const HISTORY_CHILD_ID_PREFIX = "history-";

export async function createHistoryScenario(
  api: ApiClient,
  seed: SeedData,
  title: string,
  finishedCount: number,
  hasOutstandingControls: boolean,
): Promise<HistoryScenario> {
  expect(finishedCount).toBeGreaterThanOrEqual(0);
  expect(finishedCount).toBeLessThanOrEqual(COMPLETED_SESSION_LIMIT);
  const task = await api.createTask(seed.workspaceId, title, {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  const parentId = `history-parent-${randomUUID()}`;
  return {
    taskId: task.id,
    title,
    parentId,
    layoutScopeId: parentId,
    ordinaryIds: [`history-ordinary-a-${randomUUID()}`, `history-ordinary-b-${randomUUID()}`],
    finishedCount,
    hasOutstandingControls,
  };
}

export async function seedHistoryScenarios(
  backend: BackendContext,
  seed: SeedData,
  scenarios: HistoryScenario[],
): Promise<FixtureTaskProof[]> {
  const inputs: FixtureTaskInput[] = scenarios.map((scenario) => ({
    task_id: scenario.taskId,
    parent_session_id: scenario.parentId,
    ordinary_session_ids: scenario.ordinaryIds,
    agent_profile_id: seed.agentProfileId,
    finished_count: scenario.finishedCount,
    include_outstanding: scenario.hasOutstandingControls,
  }));
  expect(backend.tmpDir).toMatch(/(?:^|[\\/])kandev-e2e-[^\\/]+$/);
  const fixturePath = path.resolve(
    process.cwd(),
    `../backend/bin/e2e-send-now-fixture${process.platform === "win32" ? ".exe" : ""}`,
  );
  const output = await execute(
    fixturePath,
    ["-test.run=^TestErrandHistoryBrowserFixture$", "-test.timeout=4m", "-test.v"],
    {
      cwd: path.resolve(process.cwd(), "../backend"),
      env: {
        ...process.env,
        KANDEV_E2E_ERRAND_HISTORY_ROOT: backend.tmpDir,
        KANDEV_E2E_ERRAND_HISTORY_TASKS: JSON.stringify({ tasks: inputs }),
      },
      timeout: 240_000,
      maxBuffer: 2 << 20,
    },
  ).catch((failure: unknown) => {
    const result = failure as { stdout?: string; stderr?: string };
    throw new Error(
      `Errand history test-only fixture failed:\n${result.stdout ?? ""}\n${result.stderr ?? ""}`,
    );
  });
  const line = output.stdout.split("\n").find((value) => value.startsWith(FIXTURE_MARKER));
  if (!line)
    throw new Error(`Errand history fixture proof missing:\n${output.stdout}\n${output.stderr}`);
  const proof = (JSON.parse(line.slice(FIXTURE_MARKER.length)) as { tasks: FixtureTaskProof[] })
    .tasks;
  expect(proof).toHaveLength(scenarios.length);
  for (const [index, scenario] of scenarios.entries()) {
    expect(proof[index]).toEqual({
      task_id: scenario.taskId,
      finished_count: scenario.finishedCount,
      ordinary_session_count: 3,
      outstanding_control_count: scenario.hasOutstandingControls ? 2 : 0,
      recovery_attention: scenario.hasOutstandingControls,
      pending_result_state: scenario.hasOutstandingControls ? "pending" : "",
    });
  }
  return proof;
}

export async function installHistoryCapture(page: Page): Promise<{
  gateway: ReturnType<typeof attachGatewayTrafficCapture>;
  detailedFrames: WireDetail[];
  collect: (page: Page) => Promise<CapturedTransport>;
}> {
  const gateway = attachGatewayTrafficCapture(page);
  const detailedFrames: WireDetail[] = [];
  const responsePromises: Promise<void>[] = [];
  const responseRecords: Array<Omit<HttpResponseRecord, "callerStack">> = [];
  let responseCursor = 0;
  let detailedFrameCursor = 0;
  let gatewayFrameCursor = 0;
  let fetchCallCursor = 0;
  let wsCallCursor = 0;
  let panelMountCursor = 0;
  let panelRemovalCursor = 0;
  let panelIdentityCursor = 0;
  let resourceEntryCursor = 0;
  let documentStartedAt: number | null = null;
  await page.addInitScript(() => {
    type CaptureWindow = Window & {
      __historyCapture?: BrowserCapture;
      __historyCaptureScanPanels?: () => void;
    };
    const target = window as CaptureWindow;
    const capture: BrowserCapture = {
      documentStartedAt: performance.timeOrigin,
      fetchCalls: [],
      wsCalls: [],
      panels: { mounts: [], removals: [], identityTransitions: [] },
    };
    target.__historyCapture = capture;
    const originalFetch = window.fetch;
    window.fetch = function historyFetch(input: RequestInfo | URL, init?: RequestInit) {
      let url: string;
      if (typeof input === "string") url = input;
      else if (input instanceof URL) url = input.href;
      else url = input.url;
      capture.fetchCalls.push({
        url,
        timestamp: performance.now(),
        caller: new Error().stack ?? "",
      });
      return originalFetch.call(this, input, init);
    };
    const originalSend = WebSocket.prototype.send;
    WebSocket.prototype.send = function historyWebSocketSend(
      data: Parameters<WebSocket["send"]>[0],
    ) {
      let action: string | undefined;
      let sessionId: string | undefined;
      let taskId: string | undefined;
      if (typeof data === "string") {
        try {
          const message = JSON.parse(data) as {
            action?: string;
            payload?: { session_id?: string; task_id?: string };
          };
          action = message.action;
          sessionId = message.payload?.session_id;
          taskId = message.payload?.task_id;
        } catch {
          // Non-JSON socket frames remain represented by Playwright frame capture.
        }
      }
      const needsCaller = [
        "session.subscribe",
        "session.unsubscribe",
        "errand.list",
        "errand.browser_summary",
        "errand.history_page",
        "task.session.list",
      ].includes(action ?? "");
      capture.wsCalls.push({
        url: this.url,
        action,
        sessionId,
        taskId,
        caller: needsCaller ? (new Error().stack ?? "") : "",
      });
      return originalSend.call(this, data);
    };

    const currentPanels = new Map<
      string,
      { element: Element; identityKnown: boolean; role?: string }
    >();
    const recordPanelChanges = (
      next: Map<string, { element: Element; identityKnown: boolean; role?: string }>,
      now: number,
    ) => {
      for (const [id, panel] of next) {
        const previous = currentPanels.get(id);
        if (previous?.element !== panel.element) {
          capture.panels.mounts.push({
            sessionId: id,
            timestamp: now,
            identityKnown: panel.identityKnown,
            role: panel.role,
          });
        } else if (!previous.identityKnown && panel.identityKnown) {
          capture.panels.identityTransitions.push({
            sessionId: id,
            timestamp: now,
            identityKnown: true,
            role: panel.role,
          });
        }
      }
      for (const [id, panel] of currentPanels) {
        if (next.get(id)?.element !== panel.element)
          capture.panels.removals.push({
            sessionId: id,
            timestamp: now,
            identityKnown: panel.identityKnown,
            role: panel.role,
          });
      }
      currentPanels.clear();
      for (const [id, panel] of next) currentPanels.set(id, panel);
    };
    const scanPanels = () => {
      const taskSessions = (window as E2EStoreWindow).__KANDEV_E2E_STORE__?.getState()
        .taskSessionsByTask?.itemsByTaskId;
      const identities = Object.values(taskSessions ?? {}).flat();
      const identitiesById = new Map(identities.map((session) => [session.id, session]));
      const next = new Map<string, { element: Element; identityKnown: boolean; role?: string }>();
      for (const panel of document.querySelectorAll(
        '[data-testid="session-chat"][data-session-id]',
      )) {
        const id = panel.getAttribute("data-session-id");
        if (!id) continue;
        const identity = identitiesById.get(id);
        next.set(id, {
          element: panel,
          identityKnown: identity !== undefined,
          role: identity?.role,
        });
      }
      recordPanelChanges(next, performance.now());
    };
    const observer = new MutationObserver(scanPanels);
    target.__historyCaptureScanPanels = scanPanels;
    observer.observe(document, { childList: true, subtree: true });
    scanPanels();
  });
  page.on("websocket", (socket) => {
    if (!socket.url().endsWith("/ws")) return;
    socket.on("framesent", (event) => detailedFrames.push(parseWireDetail("sent", event.payload)));
    socket.on("framereceived", (event) =>
      detailedFrames.push(parseWireDetail("received", event.payload)),
    );
  });
  page.on("response", (response) => {
    const request = response.request();
    if (!response.url().includes("/api/") && !response.url().includes("/t/")) return;
    responsePromises.push(
      (async () => {
        const headers = response.headers();
        const contentLengthHeader = headers["content-length"] ?? null;
        const record: Omit<HttpResponseRecord, "callerStack"> = {
          url: response.url(),
          status: response.status(),
          encodedResponseBodyBytes: null,
          responseHeaderBytes: null,
          decodedResponseBodyBytes: null,
          encodedBodySizeSource: "unavailable",
          contentLengthHeader,
          contentEncoding: headers["content-encoding"] ?? null,
          resourceType: request.resourceType(),
          method: request.method(),
          timing: request.timing() as Record<string, number>,
        };
        const [bodyResult, sizesResult] = await Promise.allSettled([
          response.body(),
          request.sizes(),
        ]);
        if (bodyResult.status === "fulfilled") {
          record.decodedResponseBodyBytes = bodyResult.value.byteLength;
        } else {
          record.responseBodyCaptureError = String(bodyResult.reason);
        }
        if (sizesResult.status === "fulfilled") {
          record.encodedResponseBodyBytes = sizesResult.value.responseBodySize;
          record.responseHeaderBytes = sizesResult.value.responseHeadersSize;
          record.encodedBodySizeSource = "request-size";
        } else if (contentLengthHeader && /^\d+$/.test(contentLengthHeader)) {
          record.encodedResponseBodyBytes = Number(contentLengthHeader);
          record.encodedBodySizeSource = "content-length";
          record.responseBodyCaptureError = `${record.responseBodyCaptureError ?? ""}; ${String(sizesResult.reason)}`;
        } else {
          record.responseBodyCaptureError = `${record.responseBodyCaptureError ?? ""}; ${String(sizesResult.reason)}`;
        }
        responseRecords.push(record);
      })(),
    );
  });
  return {
    gateway,
    detailedFrames,
    async collect(page: Page) {
      await Promise.all(responsePromises);
      const browser = await page.evaluate(() => {
        const capture = (window as Window & { __historyCapture?: BrowserCapture }).__historyCapture;
        const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
        const state = store?.getState();
        const taskSessions = state?.taskSessionsByTask?.itemsByTaskId ?? {};
        const allSessions = Object.values(taskSessions).flat();
        const resourceEntries = performance.getEntriesByType(
          "resource",
        ) as PerformanceResourceTiming[];
        const taskId = new URL(location.href).pathname.match(/\/t\/([^/]+)/)?.[1] ?? "";
        const taskRows = taskSessions[taskId] ?? [];
        return {
          capture: capture ?? {
            documentStartedAt: performance.timeOrigin,
            fetchCalls: [],
            wsCalls: [],
            panels: { mounts: [], removals: [], identityTransitions: [] },
          },
          taskId,
          taskSessionCount: taskRows.length,
          ordinarySessionCount: taskRows.filter((row) => row.role !== "errand").length,
          errandSessionCount: taskRows.filter((row) => row.role === "errand").length,
          sessionIds: taskRows.map((row) => row.id),
          resourceEntries: resourceEntries.map((entry) => ({
            name: entry.name,
            transferSize: entry.transferSize,
            encodedBodySize: entry.encodedBodySize,
            decodedBodySize: entry.decodedBodySize,
            duration: entry.duration,
            initiatorType: entry.initiatorType,
          })),
          totalReconciledRowsAcrossTasksInStore: allSessions.length,
        };
      });
      if (documentStartedAt !== browser.capture.documentStartedAt) {
        fetchCallCursor = 0;
        wsCallCursor = 0;
        panelMountCursor = 0;
        panelRemovalCursor = 0;
        panelIdentityCursor = 0;
        resourceEntryCursor = 0;
        documentStartedAt = browser.capture.documentStartedAt;
      }
      const delta = <T>(values: T[], cursor: number, update: (length: number) => void) => {
        const start = cursor > values.length ? 0 : cursor;
        update(values.length);
        return values.slice(start);
      };
      const stageResponses = delta(responseRecords, responseCursor, (length) => {
        responseCursor = length;
      });
      const stageDetailedFrames = delta(detailedFrames, detailedFrameCursor, (length) => {
        detailedFrameCursor = length;
      });
      const stageGatewayFrames = delta(gateway.frames, gatewayFrameCursor, (length) => {
        gatewayFrameCursor = length;
      });
      const stageFetchCalls = delta(browser.capture.fetchCalls, fetchCallCursor, (length) => {
        fetchCallCursor = length;
      });
      const stageWsCalls = delta(browser.capture.wsCalls, wsCallCursor, (length) => {
        wsCallCursor = length;
      });
      const stageMounts = delta(browser.capture.panels.mounts, panelMountCursor, (length) => {
        panelMountCursor = length;
      });
      const stageRemovals = delta(browser.capture.panels.removals, panelRemovalCursor, (length) => {
        panelRemovalCursor = length;
      });
      const stageIdentityTransitions = delta(
        browser.capture.panels.identityTransitions,
        panelIdentityCursor,
        (length) => {
          panelIdentityCursor = length;
        },
      );
      const stageResourceEntries = delta(browser.resourceEntries, resourceEntryCursor, (length) => {
        resourceEntryCursor = length;
      });
      const stageBrowser = {
        ...browser,
        capture: {
          ...browser.capture,
          fetchCalls: stageFetchCalls,
          wsCalls: stageWsCalls,
          panels: {
            mounts: stageMounts,
            removals: stageRemovals,
            identityTransitions: stageIdentityTransitions,
          },
        },
        resourceEntries: stageResourceEntries,
      };
      return {
        browser: stageBrowser,
        responses: stageResponses.map((response) => {
          const url = response.url;
          const caller = stageFetchCalls.find((call) => call.url === url)?.caller ?? "";
          return { ...response, callerStack: caller };
        }),
        gateway: summarizeGatewayTraffic(stageGatewayFrames),
        gatewayFrames: stageDetailedFrames,
        wsCallers: stageWsCalls,
      };
    },
  };
}

function parseWireDetail(
  direction: WireDetail["direction"],
  payload: string | Buffer | Uint8Array,
): WireDetail {
  const raw = typeof payload === "string" ? payload : Buffer.from(payload).toString("utf8");
  const bytes = Buffer.byteLength(raw, "utf8");
  try {
    const message = JSON.parse(raw) as {
      action?: string;
      event_type?: string;
      payload?: Record<string, unknown>;
    };
    const body = message.payload ?? {};
    const rows = [body.errands, body.rows, body.items, body.history].find(Array.isArray);
    return {
      direction,
      action: message.action ?? message.event_type,
      taskId: typeof body.task_id === "string" ? body.task_id : undefined,
      sessionId: typeof body.session_id === "string" ? body.session_id : undefined,
      bytes,
      timestamp: Date.now(),
      resultRowCount: Array.isArray(rows) ? rows.length : undefined,
    };
  } catch {
    return { direction, bytes, timestamp: Date.now() };
  }
}

function summarizeTransport(capture: CapturedTransport) {
  const responses = capture.responses;
  const httpByRoute: Record<
    string,
    {
      count: number;
      encodedResponseBodyBytes: number;
      decodedResponseBodyBytes: number;
      responseHeaderBytes: number;
      statuses: number[];
    }
  > = {};
  for (const response of responses) {
    const route = new URL(response.url).pathname;
    const summary = (httpByRoute[route] ??= {
      count: 0,
      encodedResponseBodyBytes: 0,
      decodedResponseBodyBytes: 0,
      responseHeaderBytes: 0,
      statuses: [],
    });
    summary.count += 1;
    summary.encodedResponseBodyBytes += response.encodedResponseBodyBytes ?? 0;
    summary.decodedResponseBodyBytes += response.decodedResponseBodyBytes ?? 0;
    summary.responseHeaderBytes += response.responseHeaderBytes ?? 0;
    summary.statuses.push(response.status);
  }
  const websocketByAction: Record<
    string,
    { sent: number; received: number; totalBytes: number; resultRows: number[] }
  > = {};
  for (const frame of capture.gatewayFrames) {
    const summary = (websocketByAction[frame.action ?? "<non-json>"] ??= {
      sent: 0,
      received: 0,
      totalBytes: 0,
      resultRows: [],
    });
    summary[frame.direction] += 1;
    summary.totalBytes += frame.bytes;
    if (frame.resultRowCount !== undefined) summary.resultRows.push(frame.resultRowCount);
  }
  return {
    httpResponseCount: responses.length,
    httpEncodedResponseBodyBytes: responses.reduce(
      (total, response) => total + (response.encodedResponseBodyBytes ?? 0),
      0,
    ),
    httpDecodedResponseBodyBytes: responses.reduce(
      (total, response) => total + (response.decodedResponseBodyBytes ?? 0),
      0,
    ),
    httpResponseHeaderBytes: responses.reduce(
      (total, response) => total + (response.responseHeaderBytes ?? 0),
      0,
    ),
    httpEncodedResponseBodyBytesUnavailable: responses.filter(
      (response) => response.encodedResponseBodyBytes === null,
    ).length,
    httpDecodedResponseBodyBytesUnavailable: responses.filter(
      (response) => response.decodedResponseBodyBytes === null,
    ).length,
    httpEncodedResponseBodyBytesBySource: Object.fromEntries(
      ["request-size", "content-length", "unavailable"].map((source) => [
        source,
        responses.filter((response) => response.encodedBodySizeSource === source).length,
      ]),
    ),
    httpByRoute,
    websocket: capture.gateway,
    websocketByAction,
    browserTaskRows: {
      total: capture.browser.taskSessionCount,
      ordinary: capture.browser.ordinarySessionCount,
      errands: capture.browser.errandSessionCount,
    },
    panelLifecycle: capture.browser.capture.panels,
    requestCallers: {
      fetch: capture.browser.capture.fetchCalls,
      websocket: capture.wsCallers.filter((call) => call.caller),
      httpResponse: responses
        .filter((response) => response.callerStack)
        .map((response) => ({
          url: response.url,
          method: response.method,
          callerStack: response.callerStack,
        })),
    },
    resourceEntries: capture.browser.resourceEntries,
  };
}

export async function createRuntimeSampler(page: Page) {
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Performance.enable");
  await cdp.send("HeapProfiler.enable");
  await cdp.send("Emulation.setCPUThrottlingRate", { rate: 1 });
  const metricValue = (rows: { name: string; value: number }[], name: string) =>
    rows.find((metric) => metric.name === name)?.value ?? null;
  return {
    async snapshot() {
      return cdp.send("Performance.getMetrics");
    },
    async delta(before: { metrics: Array<{ name: string; value: number }> }) {
      const after = await cdp.send("Performance.getMetrics");
      const diff = (name: string) => {
        const first = metricValue(before.metrics, name);
        const last = metricValue(after.metrics, name);
        return first === null || last === null ? null : (last - first) * 1_000;
      };
      return {
        rendererScriptDurationDeltaMs: diff("ScriptDuration"),
        rendererTaskDurationDeltaMs: diff("TaskDuration"),
      };
    },
    async heapAfterForcedGc() {
      await cdp.send("HeapProfiler.collectGarbage");
      const heap = await cdp.send("Runtime.getHeapUsage");
      return {
        heapUsedBytesAfterForcedGc: heap.usedSize,
        heapTotalBytesAfterForcedGc: heap.totalSize,
      };
    },
    async detach() {
      await cdp.detach();
    },
  };
}

export async function attachRawHistorySample(testInfo: TestInfo, sample: unknown): Promise<void> {
  const body = `${JSON.stringify(sample, null, 2)}\n`;
  const output = testInfo.outputPath("errand-history-sample.json");
  fs.writeFileSync(output, body);
  await testInfo.attach("errand-history-sample.json", {
    path: output,
    contentType: "application/json",
  });
}

export function historyEnvironment(phase: string, browserVersion: string) {
  const sourceRoot = path.resolve(process.cwd(), "../..");
  const sourceSha = execSyncGitHead(sourceRoot);
  const backendDir = path.resolve(sourceRoot, "apps/backend");
  const frontendDist = path.resolve(sourceRoot, "apps/web/dist");
  const harnessFiles = [
    "apps/backend/internal/orchestrator/e2e_errand_history_fixture_test.go",
    "apps/web/e2e/global-setup.ts",
    "apps/web/e2e/helpers/errand-history-baseline.ts",
    "apps/web/e2e/tests/session/errand-history-baseline.spec.ts",
    "apps/web/e2e/tests/session/mobile-errand-history-baseline.spec.ts",
  ];
  const manifest = {
    phase,
    sourceSha,
    browserVersion,
    node: process.version,
    platform: `${os.platform()} ${os.release()} ${os.arch()}`,
    cpus: os.cpus().length,
    memoryBytes: os.totalmem(),
    timestamp: new Date().toISOString(),
    backendBinarySha256: hashFile(path.join(backendDir, "bin/kandev")),
    fixtureBinarySha256: hashFile(
      path.join(
        backendDir,
        "bin",
        `e2e-send-now-fixture${process.platform === "win32" ? ".exe" : ""}`,
      ),
    ),
    frontendDistSha256: hashTree(frontendDist),
    testHarnessSha256: Object.fromEntries(
      harnessFiles.map((file) => [file, hashFile(path.join(sourceRoot, file))]),
    ),
    changedFiles: execFileSync("git", ["status", "--short"], { cwd: sourceRoot, encoding: "utf8" })
      .trim()
      .split("\n")
      .filter(Boolean),
  };
  return {
    ...manifest,
    manifestSha256: createHash("sha256").update(JSON.stringify(manifest)).digest("hex"),
  };
}

function hashFile(filePath: string): string {
  return createHash("sha256").update(fs.readFileSync(filePath)).digest("hex");
}

function hashTree(directory: string): string {
  const hash = createHash("sha256");
  const walk = (current: string) => {
    for (const entry of fs
      .readdirSync(current, { withFileTypes: true })
      .sort((a, b) => a.name.localeCompare(b.name))) {
      const fullPath = path.join(current, entry.name);
      if (entry.isDirectory()) walk(fullPath);
      else if (entry.isFile()) {
        hash.update(path.relative(directory, fullPath));
        hash.update(fs.readFileSync(fullPath));
      }
    }
  };
  walk(directory);
  return hash.digest("hex");
}

function execSyncGitHead(root: string): string {
  return execFileSync("git", ["rev-parse", "HEAD"], { cwd: root, encoding: "utf8" }).trim();
}

export async function assertHistorySessions(page: Page, scenario: HistoryScenario) {
  await expect(
    page.locator(`[data-testid="session-chat"][data-session-id="${scenario.parentId}"]`),
  ).toBeVisible();
  const observed = await page.evaluate((taskId) => {
    const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
    const sessions = store?.getState().taskSessionsByTask?.itemsByTaskId?.[taskId] ?? [];
    return {
      total: sessions.length,
      ordinary: sessions.filter((session) => session.role !== "errand").length,
      errands: sessions.filter((session) => session.role === "errand").length,
      ordinaryIds: sessions
        .filter((session) => session.role !== "errand")
        .map((session) => session.id)
        .sort(),
    };
  }, scenario.taskId);
  expect(observed.ordinaryIds).toEqual([scenario.parentId, ...scenario.ordinaryIds].sort());
  return observed;
}

async function ordinarySessionIdsInStore(page: Page, taskId: string): Promise<string[]> {
  return page.evaluate((id) => {
    const store = (window as E2EStoreWindow).__KANDEV_E2E_STORE__;
    return (store?.getState().taskSessionsByTask?.itemsByTaskId?.[id] ?? [])
      .filter((session) => session.role !== "errand")
      .map((session) => session.id)
      .sort();
  }, taskId);
}

type RuntimeSampler = Awaited<ReturnType<typeof createRuntimeSampler>>;
type HistoryCapture = Awaited<ReturnType<typeof installHistoryCapture>>;

function finishedHistoryId(scenario: HistoryScenario, index: number): string {
  return `${scenario.parentId}-${HISTORY_CHILD_ID_PREFIX}${String(index).padStart(4, "0")}`;
}

function expectedFinishedHistoryIds(scenario: HistoryScenario): string[] {
  return Array.from({ length: scenario.finishedCount }, (_, index) =>
    finishedHistoryId(scenario, index),
  );
}

function historyPanelRows(surface: Locator): Locator {
  return surface.locator(
    '[data-testid^="errands-panel-row-"]:not([data-testid^="errands-panel-row-actions-"])',
  );
}

async function renderedFinishedHistoryIds(
  rows: Locator,
  scenario: HistoryScenario,
): Promise<string[]> {
  const idPrefix = `${scenario.parentId}-${HISTORY_CHILD_ID_PREFIX}`;
  return rows.evaluateAll(
    (nodes, prefix) =>
      nodes
        .map((node) => node.getAttribute("data-testid") ?? "")
        .map((testId) => testId.replace(/^errands-panel-row-/, ""))
        .filter((id) => id.startsWith(prefix)),
    idPrefix,
  );
}

async function activateBaselineControl(locator: Locator, mobile: boolean): Promise<void> {
  if (mobile) await locator.tap();
  else await locator.click();
}

async function measureHistoryAndChildNavigation(
  page: Page,
  scenario: HistoryScenario,
  mobile: boolean,
  capture: HistoryCapture,
  sampler: RuntimeSampler,
) {
  const parent = page.locator(
    `[data-testid="session-chat"][data-session-id="${scenario.parentId}"]:visible`,
  );
  const historyBefore = await sampler.snapshot();
  await activateBaselineControl(parent.getByTestId("errands-chip"), mobile);
  const surface = page.getByTestId(mobile ? "errands-drawer-content" : "errands-popover");
  await expect(surface).toBeVisible();
  const rows = historyPanelRows(surface);
  const historyPageSnapshots: Array<{
    page: number;
    firstId: string;
    lastId: string;
    identityCountVisited: number;
    retainedFinishedHistoryRows: number;
    retainedPanelRows: number;
  }> = [];
  const expectedHistoryIds = expectedFinishedHistoryIds(scenario);
  const logicalHistoryIdentityIdsVisited: string[] = [];
  let restoredFirstRange = scenario.finishedCount === 0;
  let loadMoreActions = 0;
  let startAgainUsed = false;
  if (scenario.finishedCount > 0) {
    await activateBaselineControl(
      surface.locator(
        `[data-testid="errands-completed-toggle"], [data-testid="errands-history-toggle-${scenario.parentId}"]`,
      ),
      mobile,
    );
    const logicalPageCount = Math.ceil(scenario.finishedCount / HISTORY_PAGE_SIZE);
    for (let pageIndex = 0; pageIndex < logicalPageCount; pageIndex += 1) {
      const pageStart = pageIndex * HISTORY_PAGE_SIZE;
      const pageEnd = Math.min(pageStart + HISTORY_PAGE_SIZE, scenario.finishedCount);
      const pageIds = expectedHistoryIds.slice(pageStart, pageEnd);
      const lastId = pageIds.at(-1);
      if (!lastId) throw new Error(`History page ${pageIndex + 1} had no fixture identities.`);
      if (pageIndex > 0) {
        const loadMore = surface.getByRole("button", { name: /load more|next page/i });
        if ((await loadMore.count()) > 0) {
          const retainedIdsBeforePage = await renderedFinishedHistoryIds(rows, scenario);
          expect(retainedIdsBeforePage).not.toContain(lastId);
          await expect(loadMore).toBeVisible({ timeout: 30_000 });
          await activateBaselineControl(loadMore, mobile);
          loadMoreActions += 1;
        }
      }
      await expect(surface.getByTestId(`errands-panel-row-${lastId}`)).toBeVisible({
        timeout: 30_000,
      });
      const retainedIds = await renderedFinishedHistoryIds(rows, scenario);
      expect(new Set(retainedIds).size).toBe(retainedIds.length);
      expect(retainedIds).toEqual(expect.arrayContaining(pageIds));
      logicalHistoryIdentityIdsVisited.push(...pageIds);
      historyPageSnapshots.push({
        page: pageIndex + 1,
        firstId: pageIds[0]!,
        lastId,
        identityCountVisited: logicalHistoryIdentityIdsVisited.length,
        retainedFinishedHistoryRows: retainedIds.length,
        retainedPanelRows: await rows.count(),
      });
    }
    const firstPageIds = expectedHistoryIds.slice(0, HISTORY_PAGE_SIZE);
    const retainedIdsAtHistoryEnd = await renderedFinishedHistoryIds(rows, scenario);
    if (!firstPageIds.every((id) => retainedIdsAtHistoryEnd.includes(id))) {
      const startAgain = surface.getByRole("button", { name: /start again/i });
      await expect(startAgain).toBeVisible({ timeout: 30_000 });
      await activateBaselineControl(startAgain, mobile);
      startAgainUsed = true;
    }
    await expect
      .poll(() => renderedFinishedHistoryIds(rows, scenario), {
        timeout: 30_000,
        message: "Waiting for the first finished-history range to be restored",
      })
      .toEqual(expect.arrayContaining(firstPageIds));
    restoredFirstRange = true;
  }
  const renderedHistoryIdsBeforeChildNavigation = await renderedFinishedHistoryIds(rows, scenario);
  const retainedFinishedHistoryRowsBeforeChildNavigation =
    renderedHistoryIdsBeforeChildNavigation.length;
  const retainedPanelRowsBeforeChildNavigation = await rows.count();
  const historyExpandedTransport = await capture.collect(page);
  const childOpenDurations: number[] = [];
  for (let index = 0; index < Math.min(5, scenario.finishedCount); index += 1) {
    const childId = finishedHistoryId(scenario, index);
    const row = surface.getByTestId(`errands-panel-row-${childId}`);
    await expect(row).toBeVisible();
    const openStarted = performance.now();
    await activateBaselineControl(mobile ? row : row.getByTestId(`errand-open-${childId}`), mobile);
    await expect(
      page.locator(`[data-testid="session-chat"][data-session-id="${childId}"]:visible`),
    ).toBeVisible({ timeout: 30_000 });
    childOpenDurations.push(performance.now() - openStarted);
    const child = page.locator(
      `[data-testid="session-chat"][data-session-id="${childId}"]:visible`,
    );
    await activateBaselineControl(child.getByTestId("errand-back-to-parent"), mobile);
    await expect(parent).toBeVisible({ timeout: 30_000 });
    await activateBaselineControl(parent.getByTestId("errands-chip"), mobile);
    await expect(surface).toBeVisible();
    await activateBaselineControl(
      surface.locator(
        `[data-testid="errands-completed-toggle"], [data-testid="errands-history-toggle-${scenario.parentId}"]`,
      ),
      mobile,
    );
    await expect(row).toBeVisible();
  }
  return {
    surface,
    logicalHistoryPageCount: historyPageSnapshots.length,
    logicalHistoryIdentityIdsVisited,
    historyPageSnapshots,
    loadMoreActions,
    startAgainUsed,
    retainedFinishedHistoryRowsBeforeChildNavigation,
    retainedPanelRowsBeforeChildNavigation,
    restoredFirstRange,
    childOpenDurations,
    historyExpandedTransport,
    afterChildNavigationTransport: await capture.collect(page),
    historyCpu: await sampler.delta(historyBefore),
    repeatedNavigationHeap: await sampler.heapAfterForcedGc(),
  };
}

async function measureSavedMaximizedLayoutRestore(
  page: Page,
  scenario: HistoryScenario,
  mobile: boolean,
  sampler: RuntimeSampler,
): Promise<Record<string, unknown>> {
  if (scenario.finishedCount === 0)
    return { attempted: false, reason: "No finished child exists in this condition." };
  if (mobile) {
    return {
      attempted: false,
      reason:
        "Phone navigation uses the native child drawer and has no desktop dockview maximize control.",
    };
  }
  const childId = `${scenario.parentId}-history-0000`;
  const maximizeStorage = await page.evaluate(
    ({ parentId, childId, taskId, layoutScopeId }) => {
      type DockviewWindow = Window & {
        __dockviewApi__?: { toJSON(): object; panels: Array<{ id: string }> };
      };
      const api = (window as DockviewWindow).__dockviewApi__;
      if (!api) throw new Error("Dockview was not ready for saved-layout seeding.");
      const storeEnvironmentId = (window as E2EStoreWindow).__KANDEV_E2E_STORE__?.getState()
        .environmentIdBySessionId?.[parentId];
      const expectedLayoutScopeId = storeEnvironmentId ?? parentId;
      if (expectedLayoutScopeId !== layoutScopeId)
        throw new Error(`Unexpected Dockview layout scope: ${expectedLayoutScopeId}`);
      const serializedLayout = JSON.stringify(api.toJSON());
      if (!serializedLayout.includes(parentId))
        throw new Error("The live Dockview layout did not include the parent session.");
      const storageKey = `kandev.dockview.env-maximize-v3.${layoutScopeId}`;
      const preMaximizePanelId = `session:${parentId}`;
      const preMaximizeLayout = {
        columns: [
          {
            id: "center",
            groups: [
              {
                id: "group-history-baseline-pre-maximize",
                panels: [
                  {
                    id: preMaximizePanelId,
                    component: "chat",
                    title: "Agent",
                    tabComponent: "sessionTab",
                    params: { sessionId: parentId },
                  },
                ],
                activePanel: preMaximizePanelId,
              },
            ],
          },
        ],
      };
      const maximizedDockviewJson = JSON.parse(
        serializedLayout.replaceAll(parentId, childId),
      ) as object;
      sessionStorage.setItem(
        storageKey,
        JSON.stringify({ preMaximizeLayout, maximizedDockviewJson }),
      );
      return {
        storageKey,
        sourcePanelIds: api.panels.map((panel) => panel.id),
        childTargeted: JSON.stringify(maximizedDockviewJson).includes(childId),
        taskId,
      };
    },
    {
      parentId: scenario.parentId,
      childId,
      taskId: scenario.taskId,
      layoutScopeId: scenario.layoutScopeId,
    },
  );
  expect(maximizeStorage.childTargeted).toBe(true);
  const before = await sampler.snapshot();
  const restoreStarted = performance.now();
  await page.goto(`/t/${scenario.taskId}?sessionId=${scenario.parentId}`);
  await expect(
    page.locator(`[data-testid="session-chat"][data-session-id="${scenario.parentId}"]`),
  ).toHaveCount(1, { timeout: 30_000 });
  const parentPanelReadyMs = performance.now() - restoreStarted;
  const panelSessionIdsAtParentReady = await page
    .locator('[data-testid="session-chat"][data-session-id]')
    .evaluateAll((nodes) => nodes.map((node) => node.getAttribute("data-session-id")));
  await expect
    .poll(() => ordinarySessionIdsInStore(page, scenario.taskId), {
      timeout: 30_000,
      message: "Waiting for complete ordinary session identities to enter the store",
    })
    .toEqual([scenario.parentId, ...scenario.ordinaryIds].sort());
  await page.evaluate(() => {
    (window as Window & { __historyCaptureScanPanels?: () => void }).__historyCaptureScanPanels?.();
  });
  const ordinaryIdentityReadyMs = performance.now() - restoreStarted;
  const panelSessionIdsAfterIdentity = await page
    .locator('[data-testid="session-chat"][data-session-id]')
    .evaluateAll((nodes) => nodes.map((node) => node.getAttribute("data-session-id")));
  const errandChildIdPrefix = `${scenario.parentId}-history-`;
  const finishedChildPanelsAtParentReady = panelSessionIdsAtParentReady.filter((id) =>
    id?.startsWith(errandChildIdPrefix),
  );
  const finishedChildPanelsAfterIdentity = panelSessionIdsAfterIdentity.filter((id) =>
    id?.startsWith(errandChildIdPrefix),
  );
  expect(finishedChildPanelsAtParentReady).toEqual([]);
  expect(finishedChildPanelsAfterIdentity).toEqual([]);
  return {
    attempted: true,
    parentPanelReadyMs,
    ordinaryIdentityReadyMs,
    ordinaryIdentityLagAfterParentPanelMs: Math.max(
      0,
      ordinaryIdentityReadyMs - parentPanelReadyMs,
    ),
    maximizeStorage,
    ordinarySessionIdentityCount: scenario.ordinaryIds.length + 1,
    ordinarySessionIds: [scenario.parentId, ...scenario.ordinaryIds].sort(),
    panelSessionIdsAtParentReady,
    panelSessionIdsAfterIdentity,
    finishedChildPanelsAtParentReady,
    finishedChildPanelsAfterIdentity,
    rendererCpu: await sampler.delta(before),
  };
}

export async function exerciseErrandHistoryBaseline({
  page,
  browser,
  api,
  seed,
  backend,
  testInfo,
  finishedCount,
  mobile,
}: ExerciseOptions): Promise<void> {
  testInfo.setTimeout(240_000);
  const phase = process.env.KANDEV_HISTORY_MEASUREMENT_PHASE ?? "unlabeled";
  const release = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  let runtime: Awaited<ReturnType<typeof createRuntimeSampler>> | undefined;
  let hasPrimaryError = false;
  let primaryError: unknown;
  try {
    const suffix = `${mobile ? "phone" : "desktop"}-${finishedCount}-${testInfo.repeatEachIndex}-${randomUUID().slice(0, 8)}`;
    const mixedTitle = `History baseline mixed ${suffix}`;
    const ordinaryTitle = `History baseline ordinary ${suffix}`;
    const mixed = await createHistoryScenario(api, seed, mixedTitle, finishedCount, true);
    const ordinary = await createHistoryScenario(api, seed, ordinaryTitle, 0, false);
    const fixtureProof = await seedHistoryScenarios(backend, seed, [mixed, ordinary]);
    const capture = await installHistoryCapture(page);
    const sampler = await createRuntimeSampler(page);
    runtime = sampler;
    const browserVersion = browser.version();
    const environment = historyEnvironment(phase, browserVersion);
    const expectedOrdinary = { total: 3, ordinary: 3, errands: 0 };
    const composer = (sessionId: string) =>
      page.locator(
        `[data-testid="session-chat"][data-session-id="${sessionId}"] .tiptap.ProseMirror[contenteditable="true"]`,
      );
    const visit = async (scenario: HistoryScenario) => {
      const before = await sampler.snapshot();
      const startedAt = performance.now();
      await page.goto(`/t/${scenario.taskId}?sessionId=${scenario.parentId}`);
      const parent = page.locator(
        `[data-testid="session-chat"][data-session-id="${scenario.parentId}"]:visible`,
      );
      await expect(parent).toBeVisible({ timeout: 30_000 });
      const editor = composer(scenario.parentId);
      await expect(editor).toBeVisible({ timeout: 30_000 });
      await expect(editor).toBeEnabled();
      const draft = `history baseline accepts input ${scenario.taskId}`;
      await editor.fill(draft);
      await expect(editor).toContainText(draft);
      const readyMs = performance.now() - startedAt;
      await editor.fill("");
      const observed = await assertHistorySessions(page, scenario);
      return {
        parentReadyAndInputAcceptedMs: readyMs,
        rendererCpu: await sampler.delta(before),
        storeSessionRows: observed,
      };
    };

    const coldMixed = await visit(mixed);
    expect(coldMixed.storeSessionRows.ordinary).toBe(3);
    const coldMixedTransport = await capture.collect(page);

    const reloadBefore = await sampler.snapshot();
    const reloadStarted = performance.now();
    await page.reload();
    await expect(composer(mixed.parentId)).toBeVisible({ timeout: 30_000 });
    await expect(composer(mixed.parentId)).toBeEnabled();
    await composer(mixed.parentId).fill(`history reload accepts input ${mixed.taskId}`);
    await expect(composer(mixed.parentId)).toContainText(
      `history reload accepts input ${mixed.taskId}`,
    );
    const reloadReadyMs = performance.now() - reloadStarted;
    await composer(mixed.parentId).fill("");
    const reload = {
      parentReadyAndInputAcceptedMs: reloadReadyMs,
      rendererCpu: await sampler.delta(reloadBefore),
      storeSessionRows: await assertHistorySessions(page, mixed),
    };
    const reloadTransport = await capture.collect(page);

    const switchToOrdinaryCpuBefore = await sampler.snapshot();
    const switchToOrdinaryStarted = performance.now();
    await selectBaselineTask(page, ordinaryTitle, mobile);
    await expect(composer(ordinary.parentId)).toBeVisible({ timeout: 30_000 });
    await expect(composer(ordinary.parentId)).toBeEnabled();
    await composer(ordinary.parentId).fill(`ordinary switch accepts input ${ordinary.taskId}`);
    await expect(composer(ordinary.parentId)).toContainText(
      `ordinary switch accepts input ${ordinary.taskId}`,
    );
    const switchToOrdinaryMs = performance.now() - switchToOrdinaryStarted;
    const switchToOrdinaryCpu = await sampler.delta(switchToOrdinaryCpuBefore);
    await composer(ordinary.parentId).fill("");
    expect(await assertHistorySessions(page, ordinary)).toMatchObject(expectedOrdinary);
    const switchToOrdinaryTransport = await capture.collect(page);

    const switchToMixedCpuBefore = await sampler.snapshot();
    const switchToMixedStarted = performance.now();
    await selectBaselineTask(page, mixedTitle, mobile);
    await expect(composer(mixed.parentId)).toBeVisible({ timeout: 30_000 });
    await expect(composer(mixed.parentId)).toBeEnabled();
    await composer(mixed.parentId).fill(`mixed switch accepts input ${mixed.taskId}`);
    await expect(composer(mixed.parentId)).toContainText(
      `mixed switch accepts input ${mixed.taskId}`,
    );
    const switchToMixedMs = performance.now() - switchToMixedStarted;
    const switchToMixedCpu = await sampler.delta(switchToMixedCpuBefore);
    await composer(mixed.parentId).fill("");
    const switchedBackRows = await assertHistorySessions(page, mixed);
    expect(switchedBackRows.ordinary).toBe(3);
    const switchBackToMixedTransport = await capture.collect(page);

    const history = await measureHistoryAndChildNavigation(page, mixed, mobile, capture, sampler);
    const navigationTransport = history.afterChildNavigationTransport;
    const retainedChildDetail = await page.evaluate((prefix) => {
      const state = (window as E2EStoreWindow).__KANDEV_E2E_STORE__?.getState();
      const count = (entries: Record<string, unknown> | undefined) =>
        Object.keys(entries ?? {}).filter((id) => id.startsWith(prefix)).length;
      return {
        sessionRows: count(state?.taskSessions?.items),
        transcripts: count(state?.messages?.bySession),
        turnCaches: count(state?.turns?.bySession),
      };
    }, `${mixed.parentId}-history-`);
    const savedLayoutRestore = await measureSavedMaximizedLayoutRestore(
      page,
      mixed,
      mobile,
      sampler,
    );
    const finalTransport = await capture.collect(page);
    const finalHeap = await sampler.heapAfterForcedGc();
    const activeRichSubscriptions = activeSubscriptions(capture.gateway.frames);
    const isFinishedChildPanel = (event: PanelEvent) =>
      event.sessionId.startsWith(`${mixed.parentId}-history-`);
    const savedLayoutRestoreWithLifecycle = {
      ...savedLayoutRestore,
      panelLifecycle: finalTransport.browser.capture.panels,
      finishedChildPanelLifecycle: {
        mounts: finalTransport.browser.capture.panels.mounts.filter(isFinishedChildPanel),
        removals: finalTransport.browser.capture.panels.removals.filter(isFinishedChildPanel),
        identityTransitions:
          finalTransport.browser.capture.panels.identityTransitions.filter(isFinishedChildPanel),
      },
    };
    const finishedIds = expectedFinishedHistoryIds(mixed);
    const errandListRows = (transport: CapturedTransport) =>
      transport.gatewayFrames
        .filter((frame) => frame.direction === "received" && frame.action === "errand.list")
        .map((frame) => frame.resultRowCount ?? 0);
    const transportStages = {
      initial: coldMixedTransport,
      reload: reloadTransport,
      switchToZeroErrandTask: switchToOrdinaryTransport,
      switchBackToHistoryTask: switchBackToMixedTransport,
      afterHistoryOpen: history.historyExpandedTransport,
      afterRepeatedChildNavigation: navigationTransport,
      savedMaximizedLayoutRestore: finalTransport,
    };
    const maximumErrandListRows = Math.max(
      0,
      ...Object.values(transportStages).flatMap((stage) =>
        errandListRows(stage).map((count) => count),
      ),
    );
    const historyPageResultRows = (transport: CapturedTransport) =>
      transport.gatewayFrames
        .filter((frame) => frame.direction === "received" && frame.action === "errand.history_page")
        .map((frame) => frame.resultRowCount ?? 0);
    const sample = {
      schemaVersion: 2,
      environment,
      condition: {
        finishedHistoryCount: finishedCount,
        ordinarySessionsPerTask: 3,
        outstandingErrandControls: 2,
        browser: mobile ? "mobile-chrome" : "chromium",
        navigation: "fresh Playwright context, new route, regular browser cache policy",
      },
      fixtureProof,
      seedRecipe: {
        mixedTask:
          "3 ordinary siblings plus one terminal-pending result and one recovery-attention control, followed by the finished delivered children.",
        ordinaryControlTask: "3 ordinary siblings and no Errand rows.",
        finishedPayload:
          "Each delivered child has 5 repeated objective sentences and 19 repeated result sentences.",
        agentProcessesLaunched: 0,
      },
      measurements: {
        coldNavigation: coldMixed,
        coldReload: reload,
        warmSwitchToZeroErrandTask: {
          elapsedMs: switchToOrdinaryMs,
          rendererCpu: switchToOrdinaryCpu,
          transport: summarizeTransport(switchToOrdinaryTransport),
        },
        warmSwitchBackToHistoryTask: {
          elapsedMs: switchToMixedMs,
          rendererCpu: switchToMixedCpu,
          transport: summarizeTransport(switchBackToMixedTransport),
        },
        logicalHistoryNavigation: {
          pageSize: HISTORY_PAGE_SIZE,
          expectedLogicalPageCount: Math.ceil(finishedCount / HISTORY_PAGE_SIZE),
          logicalPagesVisited: history.logicalHistoryPageCount,
          loadMoreActions: history.loadMoreActions,
          startAgainUsed: history.startAgainUsed,
          logicalHistoryIdentityCountVisited: history.logicalHistoryIdentityIdsVisited.length,
          logicalHistoryIdentityIdsVisited: history.logicalHistoryIdentityIdsVisited,
          pages: history.historyPageSnapshots,
          retainedFinishedHistoryRowsBeforeChildNavigation:
            history.retainedFinishedHistoryRowsBeforeChildNavigation,
          retainedPanelRowsBeforeChildNavigation: history.retainedPanelRowsBeforeChildNavigation,
          firstRangeRestoredBeforeChildNavigation: history.restoredFirstRange,
        },
        historyExpansionRendererCpu: history.historyCpu,
        childOpenMs: history.childOpenDurations,
        retainedChildDetailAfterBack: retainedChildDetail,
        childRichSubscriptionIdsAfterRestore: activeRichSubscriptions,
        ordinaryRichSubscriptionIdsAfterRestore: mixed.ordinaryIds.filter((id) =>
          activeRichSubscriptions.includes(id),
        ),
        parentRichSubscriptionRetained: activeRichSubscriptions.includes(mixed.parentId),
        finishedChildRichSubscriptions: finishedIds.filter((id) =>
          activeRichSubscriptions.includes(id),
        ),
        savedMaximizedLayoutRestore: savedLayoutRestoreWithLifecycle,
        postGcHeapAfterRepeatedNavigation: history.repeatedNavigationHeap,
        postGcHeapAfterAllMeasurements: finalHeap,
        sessionListStoreRows: {
          expectedOrdinaryIds: [mixed.parentId, ...mixed.ordinaryIds].sort(),
          coldNavigation: coldMixed.storeSessionRows,
          reload: reload.storeSessionRows,
          switchBackToHistoryTask: switchedBackRows,
        },
        errandListResultRows: {
          initialNavigation: errandListRows(coldMixedTransport),
          afterExplicitHistoryOpen: errandListRows(history.historyExpandedTransport),
          maximumAcrossRun: maximumErrandListRows,
        },
        historyPageResultRows: {
          afterExplicitHistoryOpen: historyPageResultRows(history.historyExpandedTransport),
          afterRepeatedChildNavigation: historyPageResultRows(navigationTransport),
          savedLayoutRestore: historyPageResultRows(finalTransport),
        },
        transport: {
          stages: {
            initial: summarizeTransport(coldMixedTransport),
            reload: summarizeTransport(reloadTransport),
            switchToZeroErrandTask: summarizeTransport(switchToOrdinaryTransport),
            switchBackToHistoryTask: summarizeTransport(switchBackToMixedTransport),
            afterHistoryOpen: summarizeTransport(history.historyExpandedTransport),
            afterRepeatedChildNavigation: summarizeTransport(navigationTransport),
            savedMaximizedLayoutRestore: summarizeTransport(finalTransport),
          },
          rawStages: transportStages,
        },
        measurementLimits: [
          "The boot session store exposes rows materialized for task sessions; it does not expose SQLite rows scanned, joined, or enriched.",
          "CDP ScriptDuration and TaskDuration cover renderer work across the sampled interval, not an isolated session reconciliation function.",
          "Initial server-rendered boot identity has no test-only delay seam; this baseline captures natural panel and identity timing. A controlled delayed-identity comparison needs an explicit server-fixture boundary to avoid changing the production route contract.",
        ],
      },
    };
    await attachRawHistorySample(testInfo, sample);
    expect(coldMixed.storeSessionRows.ordinary).toBe(3);
    expect(finalHeap.heapUsedBytesAfterForcedGc).toBeGreaterThan(0);
    expect(history.logicalHistoryIdentityIdsVisited).toEqual(finishedIds);
    if (finishedCount > 0) {
      expect(history.logicalHistoryPageCount).toBe(Math.ceil(finishedCount / HISTORY_PAGE_SIZE));
      expect(history.restoredFirstRange).toBe(true);
    }
  } catch (error) {
    primaryError = error;
    hasPrimaryError = true;
  } finally {
    const cleanupErrors: unknown[] = [];
    try {
      try {
        await runtime?.detach();
      } catch (error) {
        if (!isClosedCdpTargetError(error)) cleanupErrors.push(error);
      }
    } finally {
      try {
        await release();
      } catch (error) {
        cleanupErrors.push(error);
      }
    }
    if (hasPrimaryError && cleanupErrors.length > 0) {
      throw new AggregateError(
        [primaryError, ...cleanupErrors],
        "Errand history measurement failed and cleanup also failed",
        { cause: primaryError },
      );
    }
    if (hasPrimaryError) throw primaryError;
    if (cleanupErrors.length > 0)
      throw new AggregateError(cleanupErrors, "Errand history measurement cleanup failed");
  }
}

function isClosedCdpTargetError(error: unknown): boolean {
  if (!(error instanceof Error)) return false;
  return /Target closed|Session closed\. Most likely the page has been closed|No session with given id/i.test(
    error.message,
  );
}

function activeSubscriptions(
  frames: ReturnType<typeof attachGatewayTrafficCapture>["frames"],
): string[] {
  const active = new Set<string>();
  for (const frame of frames) {
    if (frame.direction !== "sent" || !frame.sessionId) continue;
    if (frame.action === "session.subscribe") active.add(frame.sessionId);
    if (frame.action === "session.unsubscribe") active.delete(frame.sessionId);
  }
  return [...active].sort();
}

async function selectBaselineTask(page: Page, title: string, mobile: boolean): Promise<void> {
  if (mobile) {
    await page.getByTestId("mobile-task-picker-trigger").tap();
    const sheet = page.getByRole("dialog", { name: "Tasks", exact: true });
    await sheet
      .getByTestId("sidebar-task-item")
      .filter({ has: page.getByText(title, { exact: true }) })
      .tap();
    await expect(sheet).not.toBeVisible();
    return;
  }
  const row = page
    .getByTestId("sidebar-task-item")
    .filter({ has: page.getByText(title, { exact: true }) });
  await expect(row).toBeVisible({ timeout: 15_000 });
  await row.click();
}

export { COMPLETED_SESSION_LIMIT };
