import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { expect, type Browser, type CDPSession, type Page, type TestInfo } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { BackendContext } from "../fixtures/backend";
import type { SeedData } from "../fixtures/test-base";

type Scenario = { id: string; title: string; sessions: string[] };
type StoreWindow = Window & {
  __KANDEV_E2E_STORE__?: {
    getState(): {
      taskSessionsByTask?: {
        itemsByTaskId?: Record<string, { id: string; session_role?: string }[]>;
      };
    };
  };
};

async function seedScenario(api: ApiClient, seed: SeedData, label: string): Promise<Scenario> {
  const task = await api.createTask(seed.workspaceId, `Ordinary comparison ${label}`, {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  const sessions: string[] = [];
  for (let index = 2; index >= 0; index--) {
    const { session_id: id } = await api.seedTaskSession(task.id, {
      sessionId: `ordinary-${task.id}-${index}`,
      state: "WAITING_FOR_INPUT",
      agentProfileId: seed.agentProfileId,
    });
    sessions.unshift(id);
    for (let message = 0; message < 10; message++) {
      await api.seedSessionMessage(id, {
        type: "message",
        content: `Ordinary transcript ${label} session ${index} message ${message}. This is fixed benchmark content.`,
        authorType: "agent",
        createdAt: new Date(Date.UTC(2026, 0, 1, 0, index, message)).toISOString(),
      });
    }
  }
  await api.setPrimarySession(sessions[0]);
  return { id: task.id, title: task.title, sessions };
}

async function selectTask(page: Page, scenario: Scenario, mobile: boolean) {
  if (mobile) {
    await page.getByTestId("mobile-task-picker-trigger").tap();
    const sheet = page.getByRole("dialog", { name: "Tasks", exact: true });
    await sheet
      .getByTestId("sidebar-task-item")
      .filter({ has: page.getByText(scenario.title, { exact: true }) })
      .tap();
    await expect(sheet).not.toBeVisible();
  } else {
    await page
      .getByTestId("sidebar-task-item")
      .filter({ has: page.getByText(scenario.title, { exact: true }) })
      .click();
  }
}

async function verifyMembership(page: Page, scenario: Scenario) {
  await expect
    .poll(() =>
      page.evaluate((id) => {
        return (
          (window as StoreWindow).__KANDEV_E2E_STORE__?.getState().taskSessionsByTask
            ?.itemsByTaskId?.[id] ?? []
        )
          .map((row) => row.id)
          .sort();
      }, scenario.id),
    )
    .toEqual([...scenario.sessions].sort());
}

export async function exerciseOrdinaryMainComparison({
  page,
  browser,
  api,
  seed,
  backend,
  testInfo,
  mobile,
}: {
  page: Page;
  browser: Browser;
  api: ApiClient;
  seed: SeedData;
  backend: BackendContext;
  testInfo: TestInfo;
  mobile: boolean;
}) {
  testInfo.setTimeout(180_000);
  const flag = process.env.KANDEV_COMPARISON_ERRANDS === "true";
  const release = await backend.useEnv({ KANDEV_FEATURES_ERRANDS: String(flag) });
  let cdp: CDPSession | undefined;
  let primaryError: unknown;
  const tasks: Scenario[] = [];
  try {
    const first = await seedScenario(api, seed, "A");
    tasks.push(first);
    const second = await seedScenario(api, seed, "B");
    tasks.push(second);
    cdp = await page.context().newCDPSession(page);
    await cdp.send("Performance.enable");
    await cdp.send("HeapProfiler.enable");
    await cdp.send("Network.enable");
    await cdp.send("Network.setCacheDisabled", { cacheDisabled: true });
    await cdp.send("Emulation.setCPUThrottlingRate", { rate: 1 });
    const websocketRequests: Record<string, number> = {};
    page.on("websocket", (socket) =>
      socket.on("framesent", ({ payload }) => {
        for (const line of String(payload).split("\n")) {
          try {
            const frame = JSON.parse(line) as { type?: string; action?: string };
            if (frame.type === "request" && frame.action) {
              websocketRequests[frame.action] = (websocketRequests[frame.action] ?? 0) + 1;
            }
          } catch {
            /* Transport heartbeats are not request frames. */
          }
        }
      }),
    );
    const editor = (scenario: Scenario) =>
      page.locator(
        `[data-testid="session-chat"][data-session-id="${scenario.sessions[0]}"]:visible .tiptap.ProseMirror[contenteditable="true"]`,
      );
    const measure = async (scenario: Scenario, action: () => Promise<unknown>) => {
      const before = await cdp!.send("Performance.getMetrics");
      const started = performance.now();
      await action();
      await expect(editor(scenario)).toBeVisible({ timeout: 30_000 });
      await expect(editor(scenario)).toBeEnabled();
      await editor(scenario).fill("Ordinary input accepted");
      await expect(editor(scenario)).toHaveText("Ordinary input accepted");
      const readyMs = performance.now() - started;
      await editor(scenario).fill("");
      await verifyMembership(page, scenario);
      await expect(
        page.locator(
          `[data-testid="session-chat"][data-session-id="${scenario.sessions[0]}"]:visible`,
        ),
      ).toContainText(
        `Ordinary transcript ${scenario.title.endsWith("A") ? "A" : "B"} session 0 message 9.`,
      );
      const after = await cdp!.send("Performance.getMetrics");
      const metric = (name: string) =>
        ((after.metrics.find((row) => row.name === name)?.value ?? 0) -
          (before.metrics.find((row) => row.name === name)?.value ?? 0)) *
        1_000;
      return {
        readyMs,
        scriptMs: metric("ScriptDuration"),
        rendererTaskMs: metric("TaskDuration"),
      };
    };
    const resources = () =>
      page.evaluate(() =>
        performance.getEntriesByType("resource").map((entry) => {
          const row = entry as PerformanceResourceTiming;
          return {
            path: new URL(row.name).pathname,
            durationMs: row.duration,
            transferSize: row.transferSize,
            encodedBodySize: row.encodedBodySize,
            decodedBodySize: row.decodedBodySize,
          };
        }),
      );
    const cold = await measure(first, () =>
      page.goto(`/t/${first.id}?sessionId=${first.sessions[0]}`),
    );
    const coldResources = await resources();
    const reload = await measure(first, () => page.reload());
    const reloadResources = await resources();
    const timeOrigin = await page.evaluate(() => performance.timeOrigin);
    const switchAway = await measure(second, () => selectTask(page, second, mobile));
    const switchBack = await measure(first, () => selectTask(page, first, mobile));
    const repeated = [];
    for (let index = 0; index < 5; index++) {
      repeated.push(await measure(second, () => selectTask(page, second, mobile)));
      repeated.push(await measure(first, () => selectTask(page, first, mobile)));
    }
    expect(await page.evaluate(() => performance.timeOrigin)).toBe(timeOrigin);
    await cdp.send("HeapProfiler.collectGarbage");
    const heap = await cdp.send("Runtime.getHeapUsage");
    const root = path.resolve(process.cwd(), "../..");
    const binary = path.join(root, "apps/backend/bin/kandev");
    const sample = {
      arm: process.env.KANDEV_COMPARISON_ARM ?? "unlabelled",
      flag,
      mobile,
      repeat: testInfo.repeatEachIndex,
      browser: browser.version(),
      viewport: page.viewportSize(),
      sourceHead: execFileSync("git", ["rev-parse", "HEAD"], {
        cwd: root,
        encoding: "utf8",
      }).trim(),
      backendSha256: createHash("sha256").update(fs.readFileSync(binary)).digest("hex"),
      fixture: {
        tasks: 2,
        ordinarySessionsPerTask: 3,
        messagesPerSession: 10,
        errands: 0,
        executions: 0,
      },
      cold,
      reload,
      switchAway,
      switchBack,
      repeated,
      heap,
      websocketRequests,
      coldResources,
      reloadResources,
    };
    const output = testInfo.outputPath("ordinary-main-sample.json");
    fs.writeFileSync(output, JSON.stringify(sample, null, 2) + "\n");
    await testInfo.attach("ordinary-main-sample", {
      path: output,
      contentType: "application/json",
    });
  } catch (error) {
    primaryError = error;
    throw error;
  } finally {
    try {
      await cdp?.detach().catch((error: unknown) => {
        if (!primaryError) throw error;
      });
    } finally {
      try {
        for (const task of tasks) await api.deleteTask(task.id);
      } finally {
        await release();
      }
    }
  }
}
