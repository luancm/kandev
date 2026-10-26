import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { useSyncExternalStore } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Errand } from "@/lib/types/errands";
import { ErrandHeader } from "./errand-header";
import { ErrandStatusAnnouncer } from "./errand-status-announcer";

const TASK_ID = "task-1";
const PARENT_ID = "session-parent";
const ERRAND_ID = "errand-1";
const HEADER = "errand-header";
const STOP = "errand-stop-button";
const SEND = "errand-send-result-button";
const BACK_NAME = "Back to parent session Main";

const mocks = vi.hoisted(() => {
  const listeners = new Set<() => void>();
  return {
    state: {} as Record<string, unknown>,
    listeners,
    emit: () => listeners.forEach((listener) => listener()),
    isMobile: false,
    openSession: vi.fn(),
    cancelErrand: vi.fn(),
    sendErrandResult: vi.fn(),
    pending: new Set<string>(),
    toast: vi.fn(),
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: <T,>(selector: (state: Record<string, unknown>) => T) =>
    useSyncExternalStore(
      (listener) => {
        mocks.listeners.add(listener);
        return () => mocks.listeners.delete(listener);
      },
      () => selector(mocks.state),
    ),
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: mocks.isMobile, usesDesktopWorkbench: true }),
}));
vi.mock("./use-open-task-session", () => ({ useOpenTaskSession: () => mocks.openSession }));
vi.mock("@/hooks/domains/session/use-errand-actions", () => ({
  useErrandActions: () => ({
    cancelErrand: mocks.cancelErrand,
    sendErrandResult: mocks.sendErrandResult,
    cancelAllErrands: vi.fn(),
    isPending: (action: string, id: string) => mocks.pending.has(`${action}:${id}`),
  }),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: mocks.toast }) }));
vi.mock("@/components/agent-logo", () => ({ AgentLogo: () => <span data-testid="agent-logo" /> }));

function errand(overrides: Partial<Errand> = {}): Errand {
  return {
    errand_id: ERRAND_ID,
    task_id: TASK_ID,
    parent_session_id: PARENT_ID,
    title: "API audit",
    objective: "Audit /api/v1 handlers for missing authorize calls",
    agent_profile_id: "profile-1",
    status: "running",
    result_state: "none",
    result_preview: "",
    result_truncated: false,
    created_at: new Date(Date.now() - 2 * 60_000).toISOString(),
    updated_at: new Date().toISOString(),
    settled_at: null,
    ...overrides,
  };
}

function setState({
  enabled = true,
  row = errand(),
  role = "errand" as string | undefined,
}: { enabled?: boolean; row?: Errand | null; role?: string } = {}) {
  mocks.state = {
    features: { errands: enabled },
    tasks: { activeTaskId: TASK_ID },
    taskSessions: {
      items: {
        [ERRAND_ID]: {
          id: ERRAND_ID,
          task_id: TASK_ID,
          role,
          parent_session_id: PARENT_ID,
          workspace_path: "/home/me/.kandev/tasks/auth",
          worktree_branch: "feat/auth",
        },
        [PARENT_ID]: { id: PARENT_ID, task_id: TASK_ID, name: "Main" },
      },
    },
    errands: { byTaskId: { [TASK_ID]: row ? { [ERRAND_ID]: row } : {} } },
    agentProfiles: {
      items: [
        { id: "profile-1", label: "Codex • Deep review", agent_name: "codex", model: "gpt-5.6" },
      ],
    },
    sessionModels: { bySessionId: {} },
    activeModel: { bySessionId: {} },
  };
}

function renderHeader(sessionId = ERRAND_ID) {
  return render(<ErrandHeader sessionId={sessionId} taskId={TASK_ID} />);
}

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  mocks.isMobile = false;
  mocks.pending.clear();
  mocks.cancelErrand.mockResolvedValue(undefined);
  mocks.sendErrandResult.mockResolvedValue(undefined);
  setState();
});

describe("ErrandHeader visibility", () => {
  it("renders nothing for an ordinary session", () => {
    renderHeader(PARENT_ID);
    expect(screen.queryByTestId(HEADER)).toBeNull();
  });

  it("renders nothing while the errands flag is off", () => {
    setState({ enabled: false });
    renderHeader();
    expect(screen.queryByTestId(HEADER)).toBeNull();
  });
});

describe("ErrandHeader desktop", () => {
  it("shows the back link, title, profile, status, elapsed time, and workspace", () => {
    renderHeader();

    const header = screen.getByTestId(HEADER);
    expect(header.getAttribute("data-layout")).toBe("desktop");
    expect(within(header).getByRole("button", { name: BACK_NAME })).toBeTruthy();
    expect(within(header).getByRole("heading", { name: "API audit" })).toBeTruthy();
    expect(within(header).getByTestId("errand-profile").textContent).toBe("Deep review·gpt-5.6");
    expect(
      within(header).getByTestId("errand-header-status").getAttribute("data-errand-status"),
    ).toBe("running");
    expect(within(header).getByTestId("errand-header-elapsed").textContent).toBe("2m");
    const workspace = within(header).getByTestId("errand-workspace");
    expect(workspace.textContent).toContain("/home/me/.kandev/tasks/auth");
    expect(workspace.textContent).toContain("feat/auth");
    expect(workspace.getAttribute("title")).toBe("/home/me/.kandev/tasks/auth (feat/auth)");
  });

  it("offers Stop only while live and sends errand.cancel", () => {
    renderHeader();

    expect(screen.queryByTestId(SEND)).toBeNull();
    fireEvent.click(screen.getByTestId(STOP));
    expect(mocks.cancelErrand).toHaveBeenCalledWith(ERRAND_ID);
  });

  it("disables Stop while its request is pending", () => {
    mocks.pending.add(`cancel:${ERRAND_ID}`);
    renderHeader();

    expect((screen.getByTestId(STOP) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByTestId(STOP).textContent).toBe("Stop");
  });

  it("offers Send result to parent only when eligible, never with Stop", async () => {
    setState({ row: errand({ status: "done", can_send_result: true }) });
    renderHeader();

    expect(screen.queryByTestId(STOP)).toBeNull();
    fireEvent.click(screen.getByTestId(SEND));
    expect(mocks.sendErrandResult).toHaveBeenCalledWith(ERRAND_ID);
    await vi.waitFor(() =>
      expect(mocks.toast).toHaveBeenCalledWith(expect.objectContaining({ variant: "success" })),
    );
  });

  it("shows neither action for a settled errand that is not eligible", () => {
    setState({ row: errand({ status: "done", can_send_result: false }) });
    renderHeader();

    expect(screen.queryByTestId(STOP)).toBeNull();
    expect(screen.queryByTestId(SEND)).toBeNull();
  });

  it("reports a failed Stop with an error toast", async () => {
    mocks.cancelErrand.mockRejectedValue(new Error("nope"));
    renderHeader();

    fireEvent.click(screen.getByTestId(STOP));
    await vi.waitFor(() =>
      expect(mocks.toast).toHaveBeenCalledWith(
        expect.objectContaining({ title: "Could not stop the errand", variant: "error" }),
      ),
    );
  });

  it("goes back to the parent without changing the primary session", () => {
    renderHeader();

    fireEvent.click(screen.getByRole("button", { name: BACK_NAME }));
    expect(mocks.openSession).toHaveBeenCalledWith({
      taskId: TASK_ID,
      sessionId: PARENT_ID,
      title: "Main",
    });
  });

  it("discloses the full objective on demand", () => {
    renderHeader();

    const toggle = screen.getByTestId("errand-objective-toggle");
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    fireEvent.click(toggle);
    expect(toggle.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText("Audit /api/v1 handlers for missing authorize calls")).toBeTruthy();
  });

  it("keeps the back link for an errand whose row has not loaded yet", () => {
    setState({ row: null });
    renderHeader();

    expect(screen.getByRole("button", { name: BACK_NAME })).toBeTruthy();
    expect(screen.queryByTestId(STOP)).toBeNull();
  });
});

describe("ErrandHeader phone", () => {
  it("puts Stop in a visible 44px actions menu", () => {
    mocks.isMobile = true;
    renderHeader();

    const header = screen.getByTestId(HEADER);
    expect(header.getAttribute("data-layout")).toBe("phone");
    const back = within(header).getByRole("button", { name: BACK_NAME });
    expect(back.className).toContain("h-11");
    const trigger = within(header).getByRole("button", { name: "Errand actions" });
    expect(trigger.className).toContain("h-11");
    fireEvent.pointerDown(trigger, { pointerType: "mouse", button: 0, ctrlKey: false });
    fireEvent.click(screen.getByRole("menuitem", { name: "Stop" }));
    expect(mocks.cancelErrand).toHaveBeenCalledWith(ERRAND_ID);
    expect(screen.queryByRole("menuitem", { name: "Send result to parent" })).toBeNull();
  });

  it("offers Send result to parent from the menu when eligible", () => {
    mocks.isMobile = true;
    setState({ row: errand({ status: "failed", can_send_result: true }) });
    renderHeader();

    const trigger = screen.getByRole("button", { name: "Errand actions" });
    fireEvent.pointerDown(trigger, { pointerType: "mouse", button: 0, ctrlKey: false });
    fireEvent.click(screen.getByRole("menuitem", { name: "Send result to parent" }));
    expect(mocks.sendErrandResult).toHaveBeenCalledWith(ERRAND_ID);
  });

  it("hides the actions menu when no action applies", () => {
    mocks.isMobile = true;
    setState({ row: errand({ status: "cancelled" }) });
    renderHeader();

    expect(screen.queryByRole("button", { name: "Errand actions" })).toBeNull();
  });
});

describe("ErrandStatusAnnouncer", () => {
  function renderAnnouncer(sessionId: string, active = true) {
    return render(<ErrandStatusAnnouncer taskId={TASK_ID} sessionId={sessionId} active={active} />);
  }

  it("is one polite status region that stays silent on first render", () => {
    renderAnnouncer(PARENT_ID);

    const regions = screen.getAllByRole("status");
    expect(regions).toHaveLength(1);
    expect(regions[0].getAttribute("aria-live")).toBe("polite");
    expect(regions[0].textContent).toBe("");
  });

  it("announces a status change of an errand the session started", () => {
    renderAnnouncer(PARENT_ID);

    act(() => {
      setState({ row: errand({ status: "done" }) });
      mocks.emit();
    });

    expect(screen.getByRole("status").textContent).toBe("Errand API audit: Done");
  });

  it("stays silent for a hidden panel and for unrelated sessions", () => {
    renderAnnouncer("session-other");
    act(() => {
      setState({ row: errand({ status: "failed" }) });
      mocks.emit();
    });
    expect(screen.getByRole("status").textContent).toBe("");
    cleanup();

    setState();
    renderAnnouncer(PARENT_ID, false);
    act(() => {
      setState({ row: errand({ status: "failed" }) });
      mocks.emit();
    });
    expect(screen.getByRole("status").textContent).toBe("");
  });

  it("renders no region while the errands flag is off", () => {
    setState({ enabled: false });
    renderAnnouncer(PARENT_ID);
    expect(screen.queryByRole("status")).toBeNull();
  });
});
