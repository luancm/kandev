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
const STATUS_ATTR = "data-errand-status";

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

  it("follows the live errand row instead of the start snapshot", () => {
    mocks.errands = { [ERRAND_ID]: errand({ status: "needs_attention", title: "API audit v2" }) };
    renderTool(startCall());

    const card = screen.getByTestId(START_CARD);
    expect(card.textContent).toContain("API audit v2");
    expect(within(card).getByTestId("errand-start-status").getAttribute(STATUS_ATTR)).toBe(
      "needs_attention",
    );
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

    const open = screen.getByRole("button", { name: "Open errand API audit" }) as HTMLButtonElement;
    expect(open.disabled).toBe(true);
  });

  it("makes the whole card the Open target on phones", () => {
    mocks.isMobile = true;
    renderTool(startCall());

    const card = screen.getByTestId(START_CARD);
    expect(card.tagName).toBe("BUTTON");
    expect(card.className).toContain("min-h-11");
    expect(card.getAttribute("aria-label")).toBe("Open errand API audit, Running");
    fireEvent.click(card);
    expect(mocks.openSession).toHaveBeenCalledWith(
      expect.objectContaining({ sessionId: ERRAND_ID }),
    );
  });

  it("never exposes the decorative running spinner as a status region", () => {
    renderTool(startCall());

    expect(screen.queryAllByRole("status")).toHaveLength(0);
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

    const card = screen.getByTestId("errand-await-card");
    expect(card.textContent).toContain("Waiting for 2 errands");
    const first = screen.getByTestId(`errand-await-row-${ERRAND_ID}`);
    expect(first.textContent).toContain("Schema review");
    expect(first.querySelector(`[${STATUS_ATTR}]`)?.getAttribute(STATUS_ATTR)).toBe(
      "needs_attention",
    );
    const second = screen.getByTestId("errand-await-row-errand-2");
    expect(second.querySelector(`[${STATUS_ATTR}]`)?.getAttribute(STATUS_ATTR)).toBe("running");
  });

  it("summarizes a finished await from its result without rendering result text", () => {
    renderTool(
      kandevToolCall({
        toolName: AWAIT_TOOL,
        input: { errand_ids: [ERRAND_ID], mode: "any" },
        resultJson: { errands: [{ errand_id: ERRAND_ID, status: "done", result: "SECRET" }] },
      }),
    );

    const card = screen.getByTestId("errand-await-card");
    expect(card.textContent).toContain("Waited for 1 errand");
    expect(card.textContent).not.toContain("SECRET");
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

    expect(screen.getByTestId("errand-await-card").textContent).toContain(
      "Waiting for any of 2 errands",
    );
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
