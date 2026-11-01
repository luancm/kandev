import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Errand } from "@/lib/types/errands";
import { KandevToolMessage } from "../kandev-tool-message";
import { kandevToolCall } from "../kandev-tool-message.test-helpers";

const TASK_ID = "task-1";
const PARENT_ID = "session-parent";
const ERRAND_ID = "errand-1";
const START_TOOL = "mcp__kandev__start_errand_kandev";
const AWAIT_TOOL = "kandev/await_errands_kandev";
const CANCEL_TOOL = "mcp__kandev__cancel_errand_kandev";
const START_CARD = "errand-start-card";
const AWAIT_CARD = "errand-await-card";
const STATUS_ATTR = "data-errand-status";
const CHANGED_TITLE = "API audit v2";
const SPINNER_SELECTOR = ".spinner-grid";

const mocks = vi.hoisted(() => ({
  isMobile: false,
  openSession: vi.fn(),
  errands: {} as Record<string, Errand>,
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: mocks.isMobile, usesDesktopWorkbench: true }),
}));
vi.mock("../../errands/use-open-task-session", () => ({
  useOpenTaskSession: () => mocks.openSession,
}));
vi.mock("@/components/agent-logo", () => ({
  AgentLogo: ({ agentName }: { agentName: string }) => <span data-testid={`logo-${agentName}`} />,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      tasks: { activeTaskId: "task-other" },
      taskSessions: { items: { [PARENT_ID]: { id: PARENT_ID, task_id: TASK_ID } } },
      agentProfiles: {
        items: [
          {
            id: "profile-1",
            label: "Codex • Deep review",
            agent_id: "agent-codex",
            agent_name: "codex",
            cli_passthrough: false,
            model: "gpt-5.6",
          },
        ],
      },
      errands: { byTaskId: { [TASK_ID]: mocks.errands } },
    }),
}));

function errand(overrides: Partial<Errand> = {}): Errand {
  return {
    errand_id: ERRAND_ID,
    task_id: TASK_ID,
    parent_session_id: PARENT_ID,
    title: "API audit",
    objective: "Audit every /api/v1 handler for missing authorize calls",
    agent_profile_id: "profile-1",
    status: "running",
    result_state: "none",
    result_preview: "",
    result_truncated: false,
    created_at: "2026-10-26T10:00:00Z",
    updated_at: "2026-10-26T10:00:00Z",
    settled_at: null,
    ...overrides,
  };
}

function renderTool(message: ReturnType<typeof kandevToolCall>) {
  return render(<KandevToolMessage comment={message} sessionId={PARENT_ID} />);
}

function startCall(overrides: Parameters<typeof kandevToolCall>[0] | object = {}) {
  return kandevToolCall({
    toolName: START_TOOL,
    input: { title: "API audit", prompt: "Audit /api/v1", agent_profile: "Deep review" },
    resultJson: {
      errand_id: ERRAND_ID,
      session_id: ERRAND_ID,
      status: "running",
      agent_profile_id: "profile-1",
      agent_profile_name: "Deep review",
    },
    ...overrides,
  });
}

afterEach(cleanup);

beforeEach(() => {
  mocks.isMobile = false;
  mocks.openSession.mockReset();
  mocks.errands = {};
});

describe("start_errand_kandev card", () => {
  it("shows title, profile, status, and Open from the tool result", () => {
    renderTool(startCall());

    const card = screen.getByTestId(START_CARD);
    expect(card.textContent).not.toContain("API audit");
    fireEvent.click(within(card).getByRole("button", { expanded: false }));
    expect(card.textContent).toContain("API audit");
    expect(within(card).getByTestId("errand-profile").textContent).toBe("Deep review·gpt-5.6");
    expect(within(card).getByTestId("errand-start-status").getAttribute(STATUS_ATTR)).toBe(
      "running",
    );
    fireEvent.click(within(card).getByRole("button", { name: "Open errand API audit" }));
    expect(mocks.openSession).toHaveBeenCalledWith({
      taskId: TASK_ID,
      sessionId: ERRAND_ID,
      title: "API audit",
    });
  });

  it("keeps the completed start snapshot historical despite a changed live child", () => {
    mocks.errands = { [ERRAND_ID]: errand({ status: "done", title: CHANGED_TITLE }) };
    renderTool(startCall());
    const card = screen.getByTestId(START_CARD);
    expect(card.textContent).not.toContain("API audit");
    fireEvent.click(within(card).getByRole("button", { expanded: false }));
    expect(card.textContent).toContain("API audit");
    expect(card.textContent).not.toContain(CHANGED_TITLE);
    expect(card.textContent).toContain("Was running");
    expect(card.querySelector(SPINNER_SELECTOR)).toBeNull();
  });

  it("reveals the objective on demand", () => {
    renderTool(startCall());

    const toggle = screen.getByRole("button", { expanded: false });
    fireEvent.click(toggle);

    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText("Audit /api/v1")).toBeTruthy();
  });

  it("renders a pending call as a starting card without Open", () => {
    renderTool(
      kandevToolCall({ toolName: START_TOOL, input: { title: "API audit" }, status: "running" }),
    );

    const card = screen.getByTestId(START_CARD);
    expect(within(card).getByTestId("errand-start-status").getAttribute(STATUS_ATTR)).toBe(
      "starting",
    );
    expect(within(card).queryByRole("button", { name: /Open errand/ })).toBeNull();
  });

  it("renders an error result as a plain row with its text, never a card", () => {
    renderTool(
      kandevToolCall({
        toolName: START_TOOL,
        input: { title: "API audit" },
        output: "<b>errand_limit_reached</b>",
        status: "error",
      }),
    );

    expect(screen.queryByTestId(START_CARD)).toBeNull();
    const row = screen.getByTestId("errand-tool-row");
    expect(row.textContent).toContain("Kandev: Start Errand");
    expect(row.querySelector("b")).toBeNull();
  });

  it("disables Open while the errand waits for capacity", () => {
    mocks.errands = { [ERRAND_ID]: errand({ status: "queued" }) };
    renderTool(startCall());

    fireEvent.click(screen.getByRole("button", { expanded: false }));
    const open = screen.getByRole("button", { name: "Open errand API audit" }) as HTMLButtonElement;
    expect(open.disabled).toBe(true);
  });

  it("expands historical start details before opening on phones", () => {
    mocks.isMobile = true;
    renderTool(startCall());
    const card = screen.getByTestId(START_CARD);
    expect(card.textContent).not.toContain("API audit");
    fireEvent.click(within(card).getByRole("button", { expanded: false }));
    fireEvent.click(within(card).getByRole("button", { name: "Open errand API audit" }));
    expect(mocks.openSession).toHaveBeenCalledWith(
      expect.objectContaining({ sessionId: ERRAND_ID }),
    );
  });

  it("never exposes the decorative running spinner as a status region", () => {
    renderTool(startCall());

    expect(screen.queryAllByRole("status")).toHaveLength(0);
  });
});

describe("await_errands_kandev recorded statuses", () => {
  // @covers AC-TASKS-ERRAND-AWAIT-DURATION-001.4
  it.each([
    ["pending", "Not finished"],
    ["running", "Was running"],
    ["queued", "Was queued"],
    ["needs_attention", "Needed attention"],
    ["done", "Done"],
    ["failed", "Failed"],
    ["cancelled", "Cancelled"],
  ])("keeps a completed %s snapshot static when the live child has finished", (status, label) => {
    mocks.errands = { [ERRAND_ID]: errand({ status: "done" }) };
    renderTool(
      kandevToolCall({
        toolName: AWAIT_TOOL,
        input: { errand_ids: [ERRAND_ID] },
        resultJson: {
          duration_ms: 30000,
          end_reason: "timeout",
          errands: [{ errand_id: ERRAND_ID, status }],
        },
      }),
    );

    fireEvent.click(screen.getByRole("button", { expanded: false }));
    const row = screen.getByTestId(`errand-await-row-${ERRAND_ID}`);
    expect(row.textContent).toContain(label);
    expect(row.querySelector(SPINNER_SELECTOR)).toBeNull();
    expect(row.querySelector(`[${STATUS_ATTR}]`)?.getAttribute(STATUS_ATTR)).toBe(status);
  });
});

describe("await_errands_kandev card", () => {
  it("lists every awaited errand with its live status while waiting", () => {
    mocks.errands = {
      [ERRAND_ID]: errand({ status: "needs_attention", title: "Schema review" }),
    };
    renderTool(
      kandevToolCall({
        toolName: AWAIT_TOOL,
        input: { errand_ids: [ERRAND_ID, "errand-2"], mode: "all" },
        status: "running",
      }),
    );

    const card = screen.getByTestId(AWAIT_CARD);
    expect(card.textContent).toContain("Waiting for 2 errands");
    const first = screen.getByTestId(`errand-await-row-${ERRAND_ID}`);
    expect(first.textContent).toContain("Schema review");
    expect(first.querySelector(`[${STATUS_ATTR}]`)?.getAttribute(STATUS_ATTR)).toBe(
      "needs_attention",
    );
    const second = screen.getByTestId("errand-await-row-errand-2");
    expect(second.querySelector(`[${STATUS_ATTR}]`)?.getAttribute(STATUS_ATTR)).toBe("running");
  });

  it("keeps returned await text hidden until historical details are expanded", () => {
    mocks.errands = { [ERRAND_ID]: errand({ status: "done" }) };
    renderTool(
      kandevToolCall({
        toolName: AWAIT_TOOL,
        input: { errand_ids: [ERRAND_ID], mode: "any" },
        resultJson: {
          duration_ms: 12000,
          end_reason: "any_ready",
          errands: [{ errand_id: ERRAND_ID, status: "done", result: "SECRET" }],
        },
      }),
    );

    const card = screen.getByTestId(AWAIT_CARD);
    expect(card.textContent).not.toContain("API audit");
    expect(card.textContent).toContain("Waited 12s for 1 errand");
    expect(card.textContent).toContain("Wait condition met");
    expect(card.textContent).not.toContain("Result returned");
    expect(card.textContent).not.toContain("SECRET");
    const disclosure = within(card).getByTestId("errand-await-disclosure");
    expect(disclosure).toBe(within(card).getByRole("button", { expanded: false }));
    disclosure.focus();
    fireEvent.click(disclosure);
    expect(disclosure.getAttribute("aria-expanded")).toBe("true");
    const details = document.getElementById(disclosure.getAttribute("aria-controls")!);
    expect(details?.contains(within(card).getByTestId("errand-await-result"))).toBe(true);
    expect(document.activeElement).toBe(disclosure);
    expect(card.textContent).toContain("At the end of this wait");
    expect(card.textContent).toContain("Result returned");
    expect(card.textContent).toContain("SECRET");
    fireEvent.click(disclosure);
    expect(disclosure.getAttribute("aria-expanded")).toBe("false");
    expect(document.activeElement).toBe(disclosure);
    expect(card.textContent).toContain("Waited 12s for 1 errand");
    expect(card.textContent).toContain("Wait condition met");
    expect(within(card).queryByTestId("errand-await-result")).toBeNull();
    expect(within(card).queryByTestId(`errand-await-row-${ERRAND_ID}`)).toBeNull();
    fireEvent.click(disclosure);
    fireEvent.click(within(card).getByRole("button", { name: /Open errand/ }));
    expect(mocks.openSession).toHaveBeenCalledWith(
      expect.objectContaining({ sessionId: ERRAND_ID }),
    );
  });

  it("names an any-mode wait", () => {
    renderTool(
      kandevToolCall({
        toolName: AWAIT_TOOL,
        input: { errand_ids: ["a", "b"], mode: "any" },
        status: "in_progress",
      }),
    );

    expect(screen.getByTestId(AWAIT_CARD).textContent).toContain("Waiting for any of 2 errands");
  });
});

describe("await_errands_kandev compatibility", () => {
  it.each([
    [{}, "Waited for 1 errand"],
    [{ duration_ms: 0 }, "Waited 0s for 1 errand"],
    [{ duration_ms: -10, end_reason: "other" }, "Waited for 1 errand"],
  ])("keeps legacy and zero-duration completed waits readable: %j", (fields, label) => {
    renderTool(
      kandevToolCall({
        toolName: AWAIT_TOOL,
        input: { errand_ids: [ERRAND_ID] },
        resultJson: fields,
      }),
    );
    const card = screen.getByTestId(AWAIT_CARD);
    expect(card.textContent).toBe(label);
    fireEvent.click(within(card).getByRole("button", { expanded: false }));
    expect(card.textContent).toContain("At the end of this wait");
    expect(card.querySelector(SPINNER_SELECTOR)).toBeNull();
    expect(card.querySelector(`[${STATUS_ATTR}]`)).toBeNull();
  });

  it("preserves a recorded wait when live status changes after expansion", () => {
    const message = kandevToolCall({
      toolName: AWAIT_TOOL,
      input: { errand_ids: [ERRAND_ID] },
      resultJson: { errands: [{ errand_id: ERRAND_ID, status: "pending" }] },
    });
    mocks.errands = { [ERRAND_ID]: errand() };
    const { rerender } = renderTool(message);
    fireEvent.click(screen.getByRole("button", { expanded: false }));
    const row = screen.getByTestId(`errand-await-row-${ERRAND_ID}`);
    expect(row.textContent).toContain("Not finished");

    mocks.errands = { [ERRAND_ID]: errand({ status: "done" }) };
    rerender(<KandevToolMessage comment={{ ...message }} sessionId={PARENT_ID} />);
    expect(row.textContent).toContain("Not finished");
    expect(row.querySelector(SPINNER_SELECTOR)).toBeNull();
    expect(row.textContent).not.toContain("Done");
  });

  it("preserves failed awaits as error rows instead of completed disclosures", () => {
    renderTool(
      kandevToolCall({
        toolName: AWAIT_TOOL,
        input: { errand_ids: [ERRAND_ID] },
        output: "errand_not_found",
        status: "error",
      }),
    );
    expect(screen.queryByTestId(AWAIT_CARD)).toBeNull();
    const row = screen.getByTestId("errand-tool-row");
    fireEvent.click(within(row).getByText("Kandev: Await Errands"));
    expect(row.textContent).toContain("errand_not_found");
  });
});

describe("cancel_errand_kandev row", () => {
  it("names the stopped errand", () => {
    mocks.errands = { [ERRAND_ID]: errand({ status: "cancelled" }) };
    renderTool(
      kandevToolCall({
        toolName: CANCEL_TOOL,
        input: { errand_id: ERRAND_ID },
        resultJson: { errand_id: ERRAND_ID, status: "cancelled" },
      }),
    );

    const row = screen.getByTestId("errand-cancel-row");
    expect(row.textContent).toContain("Kandev: Stop Errand");
    expect(row.textContent).toContain("API audit");
  });
});
