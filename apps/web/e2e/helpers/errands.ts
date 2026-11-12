import { SessionPage } from "../pages/session-page";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { randomUUID } from "node:crypto";
import { resolve } from "node:path";
import { readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { expect, type Page, type Locator } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";
import type { Errand } from "../../lib/types/errands";
import type { TaskSessionState } from "../../lib/types/http";
import { mockTurnGate } from "./mock-turn-gate";

export type ErrandContextWindowFixture = {
  size: number;
  used: number;
  remaining: number;
  efficiency: number;
  source: "acp" | "api";
} | null;

export type ErrandBrowserSocket = {
  waitForInitialConnection: () => Promise<void>;
  setOfflineAndClose: () => Promise<void>;
  waitForReconnect: () => Promise<void>;
};

export type ErrandUpdateBrowserSocket = {
  isConnected: () => boolean;
  notifyErrandUpdated: (errand: Errand) => void;
  refuseNextPrompt: () => void;
  refusedActions: () => string[];
};

/** Send an existing errand.updated event through the browser's real handler path. */
export async function routeErrandUpdateBrowserSocket(
  page: Page,
): Promise<ErrandUpdateBrowserSocket> {
  let notify: ((errand: Errand) => void) | undefined;
  let sequence = 0;
  let refuseNextPrompt = false;
  const refused: string[] = [];

  await page.routeWebSocket(/\/ws(?:\?.*)?$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) => {
      if (typeof message !== "string") {
        server.send(message);
        return;
      }
      const forwarded: string[] = [];
      for (const part of message.split("\n")) {
        let frame: Record<string, unknown> | undefined;
        try {
          const parsed: unknown = JSON.parse(part);
          if (typeof parsed === "object" && parsed !== null) {
            frame = parsed as Record<string, unknown>;
          }
        } catch {
          forwarded.push(part);
          continue;
        }
        if (
          refuseNextPrompt &&
          frame?.type === "request" &&
          (frame.action === "message.add" || frame.action === "message.queue.add") &&
          typeof frame.id === "string"
        ) {
          refuseNextPrompt = false;
          refused.push(frame.action);
          socket.send(
            JSON.stringify({
              type: "error",
              id: frame.id,
              action: frame.action,
              payload: {
                code: "CONFLICT",
                message: "Errand recovery is unresolved",
                details: { reason: "errand_recovery_unresolved" },
              },
            }),
          );
          continue;
        }
        forwarded.push(part);
      }
      const next = forwarded.join("\n");
      if (next.trim()) server.send(next);
    });
    server.onMessage((message) => socket.send(message));
    notify = (errand) => {
      sequence += 1;
      socket.send(
        JSON.stringify({
          id: `errand-update-ui-${sequence}`,
          type: "notification",
          action: "errand.updated",
          payload: errand,
        }),
      );
    };
  });

  return {
    isConnected: () => Boolean(notify),
    notifyErrandUpdated: (errand) => {
      if (!notify) throw new Error("Expected the browser WebSocket to connect first");
      notify(errand);
    },
    refuseNextPrompt: () => {
      refuseNextPrompt = true;
    },
    refusedActions: () => [...refused],
  };
}

export async function routeErrandBrowserSocket(page: Page): Promise<ErrandBrowserSocket> {
  let resolveInitialConnection!: () => void;
  const initialConnection = new Promise<void>((resolve) => {
    resolveInitialConnection = resolve;
  });
  let resolveReconnect: (() => void) | undefined;
  let closeCurrentSocket: (() => Promise<void>) | undefined;

  await page.routeWebSocket(/\/ws(?:\?|$)/, (socket) => {
    socket.connectToServer();
    closeCurrentSocket = () =>
      socket.close({ code: 1001, reason: "errand context reconnect acceptance" });
    resolveInitialConnection();
    resolveReconnect?.();
    resolveReconnect = undefined;
  });

  return {
    waitForInitialConnection: () => initialConnection,
    setOfflineAndClose: async () => {
      if (!closeCurrentSocket) throw new Error("Expected the browser WebSocket to be open");
      await Promise.all([page.context().setOffline(true), closeCurrentSocket()]);
    },
    waitForReconnect: () =>
      new Promise<void>((resolve) => {
        resolveReconnect = resolve;
      }),
  };
}

export async function listErrands(api: ApiClient, taskId: string): Promise<Errand[]> {
  return (await api.wsRequest<{ errands: Errand[] }>("errand.list", { task_id: taskId })).errands;
}

export async function seedErrandContextWindow(
  api: ApiClient,
  taskId: string,
  sessionId: string,
  contextWindow: ErrandContextWindowFixture,
): Promise<void> {
  const session = (await api.listTaskSessions(taskId)).sessions.find((row) => row.id === sessionId);
  if (!session) throw new Error(`Expected session ${sessionId} to exist before context seeding`);

  await api.seedTaskSession(taskId, {
    sessionId,
    state: session.state as TaskSessionState,
    agentProfileId: session.agent_profile_id,
    startedAt: session.started_at,
    completedAt: session.completed_at ?? undefined,
    errorMessage: session.error_message ?? undefined,
    metadata: {
      ...(session.metadata ?? {}),
      context_window: contextWindow,
    },
  });
}

export function parentChat(page: Page, sessionId: string) {
  return page
    .locator(`[data-testid="session-chat"][data-session-id="${sessionId}"]:visible`)
    .first();
}

export async function waitParentIdle(api: ApiClient, taskId: string, sessionId: string) {
  await expect
    .poll(
      async () => {
        const session = (await api.listTaskSessions(taskId)).sessions.find(
          (row) => row.id === sessionId,
        );
        return (
          session?.state === "WAITING_FOR_INPUT" ||
          (session?.role === "errand" && session.state === "COMPLETED")
        );
      },
      { message: "Parent turn settles while errands remain independent", timeout: 60_000 },
    )
    .toBe(true);
}

export async function startErrands(
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
  options: {
    count?: number;
    await?: boolean;
    after?: string;
    waitForParentIdle?: boolean;
    errandAgentProfileId?: string;
  } = {},
) {
  const count = options.count ?? 2;
  const gates = Array.from({ length: count }, (_, index) =>
    mockTurnGate(backend.tmpDir, `ERRAND_RESULT_${index + 1}`),
  );
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Errands daily-use proof",
    seed.agentProfileId,
    {
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
      executor_profile_id: seed.worktreeExecutorProfileId,
      start_agent: false,
    },
  );
  const request = JSON.stringify({
    profile: options.errandAgentProfileId ?? seed.agentProfileId,
    prompts: gates.map((gate) => gate.prompt),
    after: options.after,
  });
  const launched = await api.launchSession({
    task_id: task.id,
    agent_profile_id: seed.agentProfileId,
    executor_profile_id: seed.worktreeExecutorProfileId,
    prompt: `/e2e:errands${options.await ? "-await" : ""} ${request}`,
  });
  try {
    await expect
      .poll(async () => (await listErrands(api, task.id)).length, {
        timeout: 60_000,
        message: "Errands admitted through real MCP start responses",
      })
      .toBe(Math.min(count, 5));
  } catch (error) {
    const savedBackendLogs: string[] = [];
    for (const [name, source] of [
      ["backend", backend.logPath],
      ["process", join(backend.tmpDir, "backend-process.log")],
    ] as const) {
      try {
        const contents = await readFile(source);
        const destination = join("/tmp", `errands-task04c-admission-${task.id}-${name}.log`);
        await writeFile(destination, contents);
        savedBackendLogs.push(`${destination} (${contents.byteLength} bytes)`);
      } catch {
        savedBackendLogs.push(`${source} (unavailable)`);
      }
    }
    const savedBackendLogSummary = savedBackendLogs.join(", ");
    const [taskSessions, messages] = await Promise.all([
      api.listTaskSessions(task.id),
      api.listSessionMessages(launched.session_id),
    ]).catch((diagnosticError: unknown) => {
      throw new Error(
        `Backend fixture logs: ${savedBackendLogSummary}\n` +
          `Parent state/transcript lookup failed: ${String(diagnosticError)}`,
        { cause: diagnosticError },
      );
    });
    const parent = taskSessions.sessions.find((session) => session.id === launched.session_id);
    const recentMessages = messages.messages.slice(-8).map((message) => ({
      author_type: message.author_type,
      type: message.type,
      content: message.content.slice(0, 400),
      tool: message.metadata?.title,
      status: message.metadata?.status,
      normalized: message.metadata?.normalized
        ? JSON.stringify(message.metadata.normalized).slice(0, 400)
        : undefined,
    }));
    throw new Error(
      `${error instanceof Error ? error.message : String(error)}\n` +
        `Parent session: ${JSON.stringify({
          state: parent?.state,
          error_message: parent?.error_message,
          completed_at: parent?.completed_at,
        })}\nRecent parent messages: ${JSON.stringify(recentMessages)}\n` +
        `Backend fixture logs: ${savedBackendLogSummary}`,
    );
  }
  if (!options.await && options.waitForParentIdle !== false) {
    await waitParentIdle(api, task.id, launched.session_id);
  }
  await api.setQueueAutoRun(await api.getQueueSessionIdentity(task.id, launched.session_id), true);
  const errands = await listErrands(api, task.id);
  return { taskId: task.id, parentId: launched.session_id, errands, gates };
}

export async function waitDelivered(api: ApiClient, taskId: string, ids: string[]) {
  await expect
    .poll(
      async () =>
        (await listErrands(api, taskId))
          .filter((row) => ids.includes(row.errand_id))
          .map((row) => row.result_state),
      { timeout: 60_000, message: "Settled errand results reach their parent" },
    )
    .toEqual(ids.map(() => "delivered"));
}

/** A claimed result precedes provider execution; wait for the new notification turn to finish. */
export async function waitNewParentTurnSettled(
  api: ApiClient,
  parentId: string,
  previousTurnIds: string[],
) {
  const previous = new Set(previousTurnIds);
  await expect
    .poll(
      async () => {
        const turns = (await api.listSessionTurns(parentId)).turns.filter(
          (turn) => !previous.has(turn.id),
        );
        return turns.length === 1 && Boolean(turns[0].completed_at);
      },
      { message: "The result notification's new parent turn completes", timeout: 60_000 },
    )
    .toBe(true);
}

export async function deliverySnapshots(api: ApiClient, parentId: string) {
  return (await api.listSessionMessages(parentId)).messages.flatMap((message) => {
    const envelope = message.metadata?.errand_delivery as { items?: unknown[] } | undefined;
    return envelope?.items ?? [];
  });
}

/** Persist the normalized MCP envelope, including unfinished outcomes at the end of a wait. */
export async function seedTimedOutAwait(api: ApiClient, parentId: string, errands: Errand[]) {
  const toolName = "mcp__kandev__await_errands_kandev";
  const result = {
    duration_ms: 30000,
    end_reason: "timeout",
    errands: errands.map((row, index) => ({
      errand_id: row.errand_id,
      status: index === 0 ? "pending" : "running",
    })),
  };
  const completedAt = new Date();
  return api.seedSessionMessage(parentId, {
    type: "tool_call",
    content: toolName,
    newTurn: true,
    turnStartedAt: new Date(completedAt.getTime() - result.duration_ms).toISOString(),
    turnCompletedAt: completedAt.toISOString(),
    metadata: {
      tool_call_id: `await-history-${parentId}`,
      title: toolName,
      status: "complete",
      normalized: {
        kind: "generic",
        generic: {
          name: "other",
          input: { errand_ids: errands.map((row) => row.errand_id), mode: "all" },
          output: {
            _meta: null,
            content: [{ type: "text", text: JSON.stringify(result) }],
            structuredContent: null,
          },
        },
      },
    },
  });
}

export async function expectAwaitSnapshot(card: Locator, errands: Errand[]) {
  await expect(card).toContainText("At the end of this wait");
  for (const [index, errand] of errands.entries()) {
    const row = card.getByTestId(`errand-await-row-${errand.errand_id}`);
    await expect(row).toContainText(errand.title);
    await expect(row).toContainText(index === 0 ? "Not finished" : "Was running");
    await expect(row.locator("[data-errand-status]")).toHaveAttribute(
      "data-errand-status",
      index === 0 ? "pending" : "running",
    );
    await expect(row.getByTestId(`errand-open-${errand.errand_id}`)).toBeEnabled();
  }
  await expect(card.locator(".spinner-grid")).toHaveCount(0);
  await expect(card).not.toContainText("Result returned");
}

export async function expectTouchTarget(control: Locator) {
  await expect
    .poll(async () => {
      const box = await control.boundingBox();
      return box !== null && box.height >= 44 && box.width >= 44;
    })
    .toBe(true);
}

export async function expectNoHorizontalOverflow(page: Page) {
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    ),
  ).toBe(true);
}

/** Parent-owned history and a second parent with active work, all through real MCP starts. */
export async function startMixedParentErrands(
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
) {
  const first = await startErrands(api, seed, backend, { count: 5 });
  await seedTimedOutAwait(api, first.parentId, first.errands.slice(0, 2));
  for (const gate of first.gates) await gate.release();
  await waitDelivered(
    api,
    first.taskId,
    first.errands.map((row) => row.errand_id),
  );
  await waitParentIdle(api, first.taskId, first.parentId);
  const gates = [
    mockTurnGate(backend.tmpDir, "SECOND_PARENT_A"),
    mockTurnGate(backend.tmpDir, "SECOND_PARENT_B"),
  ];
  const second = await api.launchSession({
    task_id: first.taskId,
    agent_profile_id: seed.agentProfileId,
    executor_profile_id: seed.worktreeExecutorProfileId,
    prompt: `/e2e:errands ${JSON.stringify({ profile: seed.agentProfileId, prompts: gates.map((gate) => gate.prompt) })}`,
  });
  await expect.poll(async () => (await listErrands(api, first.taskId)).length).toBe(7);
  await waitParentIdle(api, first.taskId, second.session_id);
  const spare = await api.launchSession({
    task_id: first.taskId,
    agent_profile_id: seed.agentProfileId,
    executor_profile_id: seed.worktreeExecutorProfileId,
    prompt: 'e2e:message("NORMAL_CONVERSATION")',
  });
  await waitParentIdle(api, first.taskId, spare.session_id);
  const secondErrands = (await listErrands(api, first.taskId)).filter(
    (row) => row.parent_session_id === second.session_id,
  );
  return {
    ...first,
    secondParentId: second.session_id,
    secondErrands,
    spareId: spare.session_id,
    secondGates: gates,
  };
}

/** Recreates a saved pre-Task29 child tab without mounting a second live conversation. */
export async function seedLegacyErrandTab(page: Page, parentId: string, childId: string) {
  await expect
    .poll(() =>
      page.evaluate(
        (parent) =>
          Object.keys(sessionStorage).some((key) => {
            if (!key.startsWith("kandev.dockview.env-layout-v3.")) return false;
            const layout = JSON.parse(sessionStorage.getItem(key)!);
            return Boolean(layout.panels?.[`session:${parent}`]);
          }),
        parentId,
      ),
    )
    .toBe(true);
  const written = await page.evaluate(
    ({ parent, child }) => {
      type Node = { type: string; data: Node[] | { views: string[]; activeView?: string } };
      const parentPanel = `session:${parent}`;
      const childPanel = `session:${child}`;
      let count = 0;
      for (const key of Object.keys(sessionStorage)) {
        if (!key.startsWith("kandev.dockview.env-layout-v3.")) continue;
        const layout = JSON.parse(sessionStorage.getItem(key)!) as {
          panels: Record<string, { id: string; params?: Record<string, unknown> }>;
          grid: { root: Node };
        };
        const original = layout.panels?.[parentPanel];
        if (!original) continue;
        layout.panels[childPanel] = {
          ...original,
          id: childPanel,
          params: { ...original.params, sessionId: child },
        };
        const visit = (node: Node) => {
          if (Array.isArray(node.data)) {
            node.data.forEach(visit);
            return;
          }
          if (node.data.views.includes(parentPanel)) {
            node.data.views.push(childPanel);
            node.data.activeView = childPanel;
          }
        };
        visit(layout.grid.root);
        sessionStorage.setItem(key, JSON.stringify(layout));
        count++;
      }
      return count;
    },
    { parent: parentId, child: childId },
  );
  expect(written).toBeGreaterThan(0);
}

/** Independent timestamp/identity oracle; deliberately does not import the UI sorter. */
function orderedErrandIds(rows: Errand[]): string[] {
  return [...rows]
    .sort((left, right) => {
      const leftTime = Date.parse(left.created_at);
      const rightTime = Date.parse(right.created_at);
      const difference =
        (Number.isNaN(rightTime) ? -Infinity : rightTime) -
        (Number.isNaN(leftTime) ? -Infinity : leftTime);
      return difference || left.errand_id.localeCompare(right.errand_id);
    })
    .map((row) => `errands-panel-row-${row.errand_id}`);
}

/** Exercise both real parent scopes with settlement while their management surface is open. */
export async function expectMixedParentOrdering(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
  phone: boolean,
) {
  const first = await startErrands(api, seed, backend, { count: 3 });
  const secondGates = [
    mockTurnGate(backend.tmpDir, "SECOND_A"),
    mockTurnGate(backend.tmpDir, "SECOND_B"),
  ];
  const allGates = [...first.gates, ...secondGates];
  try {
    const second = await api.launchSession({
      task_id: first.taskId,
      agent_profile_id: seed.agentProfileId,
      executor_profile_id: seed.worktreeExecutorProfileId,
      prompt: `/e2e:errands ${JSON.stringify({ profile: seed.agentProfileId, prompts: secondGates.map((gate) => gate.prompt) })}`,
    });
    await expect.poll(async () => (await listErrands(api, first.taskId)).length).toBe(5);
    await waitParentIdle(api, first.taskId, second.session_id);
    await api.setQueueAutoRun(
      await api.getQueueSessionIdentity(first.taskId, second.session_id),
      true,
    );
    const rows = await listErrands(api, first.taskId);
    await expect
      .poll(async () => {
        const sessions = (await api.listTaskSessions(first.taskId)).sessions;
        return rows.every(
          (row) => sessions.find((session) => session.id === row.errand_id)?.state === "RUNNING",
        );
      })
      .toBe(true);
    const secondRows = rows.filter((row) => row.parent_session_id === second.session_id);
    const rowFor = (gate: ReturnType<typeof mockTurnGate>) => {
      const matches = rows.filter((row) => row.objective === gate.prompt);
      expect(matches).toHaveLength(1);
      return matches[0];
    };
    const firstDone = first.gates.slice(0, 2).map(rowFor);
    const secondDone = rowFor(secondGates[0]);
    const socket = await routeErrandBrowserSocket(page);
    await openDefaultErrandParent(page, api, first.taskId, first.parentId, true);
    await socket.waitForInitialConnection();
    const surface = page.getByTestId(phone ? "errands-drawer-content" : "errands-popover");
    const activate = async (control: Locator) => {
      if (phone) await control.tap();
      else await control.click();
    };
    const open = async (parentId: string) => {
      await expect(parentChat(page, parentId)).toBeVisible();
      await activate(parentChat(page, parentId).getByTestId("errands-chip"));
      await expect(surface).toBeVisible();
    };
    const assertOrder = async (parentId: string, expanded: boolean) => {
      const current = (await listErrands(api, first.taskId)).filter(
        (row) => row.parent_session_id === parentId,
      );
      const active = current.filter((row) =>
        ["queued", "running", "needs_attention"].includes(row.status),
      );
      const finished = current.filter((row) => !active.includes(row));
      await expect(surface.locator('[data-testid^="errands-panel-row-"]')).toHaveCount(
        active.length + (expanded ? finished.length : 0),
      );
      await expect
        .poll(() =>
          surface
            .locator('[data-testid^="errands-panel-row-"]')
            .evaluateAll((nodes) => nodes.map((node) => node.getAttribute("data-testid"))),
        )
        .toEqual([...orderedErrandIds(active), ...(expanded ? orderedErrandIds(finished) : [])]);
    };
    const switchParent = async (id: string) => {
      await page.keyboard.press("Escape");
      if (phone) {
        await page.getByTestId("mobile-sessions-pill").tap();
        await page.getByTestId(`mobile-session-row-${id}`).tap();
      } else await page.getByTestId(`session-tab-${id}`).click();
      await open(id);
    };
    await open(first.parentId);
    await assertOrder(first.parentId, false);
    await switchParent(second.session_id);
    await assertOrder(second.session_id, false);
    await switchParent(first.parentId);
    for (const gate of [...first.gates.slice(0, 2), secondGates[0]]) await gate.release();
    await waitDelivered(
      api,
      first.taskId,
      [...firstDone, secondDone].map((row) => row.errand_id),
    );
    await waitParentIdle(api, first.taskId, first.parentId);
    await waitParentIdle(api, first.taskId, second.session_id);
    await expect(surface.getByTestId("errands-completed-toggle")).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    await assertOrder(first.parentId, false);
    await activate(surface.getByTestId("errands-completed-toggle"));
    await assertOrder(first.parentId, true);
    await first.gates[2].release();
    await waitDelivered(api, first.taskId, [rowFor(first.gates[2]).errand_id]);
    await assertOrder(first.parentId, true);
    await expect(surface.getByTestId("errands-stop-all")).toBeVisible();
    await seedErrandContextWindow(api, first.taskId, firstDone[0].errand_id, {
      size: 100000,
      used: 68000,
      remaining: 32000,
      efficiency: 0.68,
      source: "acp",
    });
    await expect(surface.getByTestId(`errands-panel-row-${firstDone[0].errand_id}`)).toContainText(
      "68.0K",
    );
    await assertOrder(first.parentId, true);
    if (phone) {
      const list = surface.getByTestId("errands-panel-list");
      await expect
        .poll(() => list.evaluate((node) => node.scrollHeight > node.clientHeight))
        .toBe(true);
      const footer = surface.getByTestId("errands-stop-all");
      await expectTouchTarget(footer);
      await expect(footer).toBeInViewport();
      const beforeScroll = await footer.boundingBox();
      await list.evaluate((node) => {
        node.scrollTop = node.scrollHeight;
      });
      const afterScroll = await footer.boundingBox();
      expect(afterScroll?.y).toBe(beforeScroll?.y);
      expect(afterScroll!.y + afterScroll!.height).toBeLessThanOrEqual(page.viewportSize()!.height);
      expect(
        await page.evaluate(
          () => document.documentElement.scrollHeight <= document.documentElement.clientHeight + 1,
        ),
      ).toBe(true);
      await expectNoHorizontalOverflow(page);
    }
    await switchParent(second.session_id);
    await expect(surface.getByTestId("errands-completed-toggle")).toHaveAttribute(
      "aria-expanded",
      "false",
    );
    await assertOrder(second.session_id, false);
    await activate(surface.getByTestId("errands-completed-toggle"));
    await assertOrder(second.session_id, true);
    const child = secondRows[0];
    const target = surface.getByTestId(
      phone ? `errands-panel-row-${child.errand_id}` : `errand-open-${child.errand_id}`,
    );
    await target.scrollIntoViewIfNeeded();
    await activate(target);
    await expect(parentChat(page, child.errand_id)).toBeVisible();
    await activate(parentChat(page, child.errand_id).getByTestId("errand-back-to-parent"));
    await page.reload();
    await open(second.session_id);
    await activate(surface.getByTestId("errands-completed-toggle"));
    await assertOrder(second.session_id, true);
    await socket.setOfflineAndClose();
    const reconnected = socket.waitForReconnect();
    await page.context().setOffline(false);
    await reconnected;
    await assertOrder(second.session_id, true);
    await secondGates[1].release();
    await waitDelivered(api, first.taskId, [rowFor(secondGates[1]).errand_id]);
    await assertOrder(second.session_id, true);
    await expect(surface.getByTestId("errands-stop-all")).toHaveCount(0);
    await switchParent(first.parentId);
    const toggle = surface.getByTestId("errands-completed-toggle");
    if ((await toggle.getAttribute("aria-expanded")) === "false") await activate(toggle);
    await assertOrder(first.parentId, true);
    await expect(surface.getByTestId("errands-stop-all")).toHaveCount(0);
    for (const row of rows)
      expect((await api.getTaskSession(row.errand_id)).session.id).toBe(row.errand_id);
  } finally {
    await page.context().setOffline(false);
    for (const gate of allGates) await gate.release();
  }
}

async function mutateAwaitFixture(backend: BackendContext, parentId: string, action: string) {
  const { stdout } = await promisify(execFile)(
    resolve("../backend/bin/e2e-delivery-fixture"),
    ["-test.run=^TestDeliveryRecordRecoveryFixture$", "-test.timeout=30s", "-test.v"],
    {
      env: {
        ...process.env,
        KANDEV_E2E_RECORD_ROOT: backend.tmpDir,
        KANDEV_E2E_RECORD_SESSION: parentId,
        KANDEV_E2E_RECORD_ACTION: action,
      },
      timeout: 40000,
    },
  );
  expect(stdout).toContain("KANDEV_E2E_ERRAND_FIXTURE:");
}

export async function expectDurableAwaitRecovery(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
  phone: boolean,
) {
  const parentGate = mockTurnGate(backend.tmpDir, "PARENT_RECEIPT_COMPLETE");
  const fixture = await startErrands(api, seed, backend, {
    count: 1,
    await: true,
    after: parentGate.prompt,
  });
  const { taskId, parentId, errands, gates } = fixture;
  try {
    await gates[0].release();
    await waitDelivered(api, taskId, [errands[0].errand_id]);
    await expect
      .poll(async () =>
        (await api.listSessionMessages(parentId)).messages.some((message) =>
          message.content.includes("Errands parent returned"),
        ),
      )
      .toBe(true);
    expect((await api.getTaskSession(parentId)).session.state).toBe("RUNNING");
    await mutateAwaitFixture(backend, parentId, "errand_offered");
    const activate = async (control: Locator) => {
      if (phone) await control.tap();
      else await control.click();
    };
    const surface = page.getByTestId(phone ? "errands-drawer-content" : "errands-popover");
    const row = surface.getByTestId(`errands-panel-row-${errands[0].errand_id}`);
    const open = async () => {
      await openDefaultErrandParent(page, api, taskId, parentId, true);
      await expect(parentChat(page, parentId)).toBeVisible();
      await activate(parentChat(page, parentId).getByTestId("errands-chip"));
      await activate(surface.getByTestId("errands-completed-toggle"));
      await expect(row).toBeVisible();
    };
    await open();
    await expect(row.getByTestId("errand-delivery-uncertain")).toHaveCount(0);
    await mutateAwaitFixture(backend, parentId, "errand_recover");
    await open();
    await expect(row.getByTestId("errand-delivery-uncertain")).toBeVisible();
    await activate(phone ? row : row.getByTestId(`errand-open-${errands[0].errand_id}`));
    const child = parentChat(page, errands[0].errand_id);
    await expect(child.getByTestId("errand-header")).toBeVisible();
    await activate(child.getByTestId("errand-back-to-parent"));
    for (const action of ["errand_status", "errand_zero"]) {
      await mutateAwaitFixture(backend, parentId, action);
      expect((await listErrands(api, taskId))[0].delivery_state).toBe("uncertain");
      await open();
      await expect(row.getByTestId("errand-delivery-uncertain")).toBeVisible();
    }
    await activate(phone ? row : row.getByTestId(`errand-open-${errands[0].errand_id}`));
    await api.addUserMessage(taskId, errands[0].errand_id, 'e2e:message("SUCCESSOR_RESULT")');
    await waitParentIdle(api, taskId, errands[0].errand_id);
    if (phone) {
      await child.getByTestId("errand-actions-menu").tap();
      await page.getByRole("menuitem", { name: "Send result to parent", exact: true }).tap();
    } else await child.getByTestId("errand-send-result-button").click();
    await expect.poll(async () => (await listErrands(api, taskId))[0].result_state).toBe("pending");
    await mutateAwaitFixture(backend, parentId, "errand_supersede");
    expect((await listErrands(api, taskId))[0].delivery_state).toBe("uncertain");
    await open();
    await expect(row.getByTestId("errand-delivery-uncertain")).toBeVisible();
    expect((await listErrands(api, taskId))[0].result_preview).toContain("SUCCESSOR_RESULT");
    await mutateAwaitFixture(backend, parentId, "errand_receipt");
    expect((await listErrands(api, taskId))[0].delivery_state).toBe("received");
    await open();
    await expect(row.getByTestId("errand-delivery-uncertain")).toHaveCount(0);
    await page.reload();
    await activate(parentChat(page, parentId).getByTestId("errands-chip"));
    await activate(surface.getByTestId("errands-completed-toggle"));
    await expect(row).toBeVisible();
    await expect(row.getByTestId("errand-delivery-uncertain")).toHaveCount(0);
    await expectNoHorizontalOverflow(page);
  } finally {
    for (const gate of [...gates, parentGate]) await gate.release();
  }
}

/** Preserve the complete default-route evidence before fixture teardown or any retry. */
export async function openDefaultErrandParent(
  page: Page,
  api: ApiClient,
  taskId: string,
  parentId: string,
  explicit = false,
) {
  const evidence: Record<string, unknown> = {
    expectedParent: parentId,
    task: await api.getTask(taskId),
    sessions: await api.listTaskSessions(taskId),
  };
  const responses: Promise<unknown>[] = [];
  const errors: string[] = [];
  page.on("response", (response) => {
    if (new URL(response.url()).pathname.startsWith(`/api/v1/tasks/${taskId}`)) {
      responses.push(
        response.text().then(
          (body) => ({ url: response.url(), status: response.status(), body }),
          (error: unknown) => ({
            url: response.url(),
            status: response.status(),
            error: String(error),
          }),
        ),
      );
    }
  });
  page.on("console", (message) => {
    if (["error", "warning"].includes(message.type())) errors.push(message.text());
  });
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(`/t/${taskId}${explicit ? `?sessionId=${parentId}` : ""}`);
  try {
    await expect(parentChat(page, parentId)).toBeVisible({ timeout: 15000 });
  } catch (error) {
    const path = `/tmp/errands-route-${randomUUID()}`;
    evidence.responses = await Promise.allSettled(responses);
    evidence.errors = errors;
    evidence.browser = await page.evaluate(() => {
      const target = window as typeof window & {
        __KANDEV_E2E_STORE__?: { getState: () => unknown };
        __KANDEV_BOOT_PAYLOAD__?: unknown;
      };
      return {
        url: location.href,
        chats: Array.from(document.querySelectorAll('[data-testid="session-chat"]')).map(
          (node) => ({
            id: node.getAttribute("data-session-id"),
            active: node.getAttribute("data-detail-active"),
          }),
        ),
        store: target.__KANDEV_E2E_STORE__?.getState(),
        boot: target.__KANDEV_BOOT_PAYLOAD__,
      };
    });
    await writeFile(`${path}.json`, JSON.stringify(evidence, null, 2));
    await page.screenshot({ path: `${path}.png`, fullPage: true });
    throw new Error(`Default errand route failed; evidence: ${path}.json`, { cause: error });
  }
}

/** Summary context must use normal conversations while child transcripts stay retained. */
export async function expectNormalContextSources(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
  mobile: boolean,
) {
  const restore = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: "true" });
  let gates: Array<ReturnType<typeof mockTurnGate>> = [];
  try {
    const fixture = await startErrands(api, seed, backend, { count: 2 });
    gates = fixture.gates;
    await page.goto(`/t/${fixture.taskId}?sessionId=${fixture.parentId}`);
    await expect(parentChat(page, fixture.parentId)).toBeVisible();
    const session = new SessionPage(page);
    if (mobile) await session.openMobileNewSessionDialog();
    else await session.openNewSessionDialog();
    const dialog = session.newSessionDialog();
    await expect(dialog).toHaveCount(1);
    const context = dialog.getByRole("combobox").filter({ hasText: "Blank" });
    await expect(context).toHaveCount(1);
    if (mobile) await context.tap();
    else await context.click();
    const options = page.getByRole("listbox").getByRole("option");
    await expect(options).toHaveCount(3);
    await expect(options.nth(2)).toContainText("1");
    await expectNoHorizontalOverflow(page);
    await page.goto(`/t/${fixture.taskId}?sessionId=${fixture.errands[0].errand_id}`);
    await expect(
      parentChat(page, fixture.errands[0].errand_id).getByTestId("errand-header"),
    ).toBeVisible();
    expect((await api.listTaskSessions(fixture.taskId)).sessions.map((row) => row.id)).toEqual(
      expect.arrayContaining(fixture.errands.map((row) => row.errand_id)),
    );
  } finally {
    for (const gate of gates) await gate.release();
    await restore();
  }
}
