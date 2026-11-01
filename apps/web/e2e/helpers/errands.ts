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
};

/** Send an existing errand.updated event through the browser's real handler path. */
export async function routeErrandUpdateBrowserSocket(
  page: Page,
): Promise<ErrandUpdateBrowserSocket> {
  let notify: ((errand: Errand) => void) | undefined;
  let sequence = 0;

  await page.routeWebSocket(/\/ws(?:\?.*)?$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) => server.send(message));
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
      async () =>
        (await api.listTaskSessions(taskId)).sessions.find((session) => session.id === sessionId)
          ?.state,
      { message: "Parent turn settles while errands remain independent", timeout: 60_000 },
    )
    .toBe("WAITING_FOR_INPUT");
}

export async function startErrands(
  api: ApiClient,
  seed: SeedData,
  backend: BackendContext,
  options: { count?: number; await?: boolean } = {},
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
    profile: seed.agentProfileId,
    prompts: gates.map((gate) => gate.prompt),
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
  if (!options.await) await waitParentIdle(api, task.id, launched.session_id);
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
