import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { useSyncExternalStore } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Errand } from "@/lib/types/errands";
import { ErrandsChip } from "./errands-chip";

const TASK_ID = "task-1";
const MAIN = "session-main";
const SECOND = "session-second";
const CHIP = "errands-chip";
const STOP_ALL = "errands-stop-all";
const POPOVER = "errands-popover";
const OPEN_E1 = "Open errand Errand e1";

const mocks = vi.hoisted(() => {
  const listeners = new Set<() => void>();
  return {
    state: {} as Record<string, unknown>,
    listeners,
    errands: [] as Errand[],
    isLoaded: true,
    useTaskErrands: vi.fn(),
    isMobile: false,
    finePointer: true,
    openSession: vi.fn(),
    cancelAllErrands: vi.fn(),
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
vi.mock("@/hooks/domains/session/use-task-errands", () => ({
  useTaskErrands: (taskId: string) => {
    mocks.useTaskErrands(taskId);
    const errandsByParent: Record<string, Errand[]> = {};
    for (const item of mocks.errands) (errandsByParent[item.parent_session_id] ??= []).push(item);
    return {
      errands: mocks.errands,
      errandsByParent,
      liveCount: mocks.errands.filter((item) =>
        ["queued", "running", "needs_attention"].includes(item.status),
      ).length,
      isLoaded: mocks.isLoaded,
    };
  },
}));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    isMobile: mocks.isMobile,
    isFinePointer: mocks.finePointer,
    usesDesktopWorkbench: !mocks.isMobile,
  }),
}));
vi.mock("./use-open-task-session", () => ({ useOpenTaskSession: () => mocks.openSession }));
vi.mock("@/hooks/domains/session/use-errand-actions", () => ({
  useErrandActions: () => ({
    cancelErrand: vi.fn(),
    sendErrandResult: vi.fn(),
    cancelAllErrands: mocks.cancelAllErrands,
    isPending: (action: string, id: string) => mocks.pending.has(`${action}:${id}`),
  }),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: mocks.toast }) }));

function errand(id: string, overrides: Partial<Errand> = {}): Errand {
  return {
    errand_id: id,
    task_id: TASK_ID,
    parent_session_id: MAIN,
    title: `Errand ${id}`,
    objective: "Do the thing",
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

function setState(enabled = true) {
  mocks.state = {
    features: { errands: enabled },
    contextWindow: { bySessionId: {} },
    taskSessions: {
      items: {
        [MAIN]: { id: MAIN, task_id: TASK_ID, name: "Main" },
        [SECOND]: { id: SECOND, task_id: TASK_ID, name: "Reviewer" },
      },
    },
    errands: { byTaskId: {} },
    agentProfiles: { items: [] },
    sessionModels: { bySessionId: {} },
    activeModel: { bySessionId: {} },
    connection: { status: "connected" },
    setContextWindow: vi.fn(),
  };
}

function openChip() {
  fireEvent.click(screen.getByTestId(CHIP));
}

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  mocks.errands = [errand("e1"), errand("e2", { status: "done" })];
  mocks.isLoaded = true;
  mocks.isMobile = false;
  mocks.finePointer = true;
  mocks.pending.clear();
  mocks.cancelAllErrands.mockResolvedValue(1);
  setState();
});

describe("ErrandsChip visibility and label", () => {
  it("renders and loads nothing while the errands flag is off", () => {
    setState(false);
    render(<ErrandsChip taskId={TASK_ID} />);
    expect(screen.queryByTestId(CHIP)).toBeNull();
    expect(mocks.useTaskErrands).not.toHaveBeenCalled();
  });

  it("renders nothing for a task without errands", () => {
    mocks.errands = [];
    render(<ErrandsChip taskId={TASK_ID} />);
    expect(screen.queryByTestId(CHIP)).toBeNull();
  });

  it("is labelled with the live count", () => {
    render(<ErrandsChip taskId={TASK_ID} />);
    const chip = screen.getByTestId(CHIP);
    expect(chip.textContent).toBe("Errands: 1 running");
    expect(chip.getAttribute("data-tone")).toBe("live");
  });

  it("turns amber and says so when an errand needs attention", () => {
    mocks.errands = [errand("e1", { status: "needs_attention" }), errand("e2")];
    render(<ErrandsChip taskId={TASK_ID} />);
    const chip = screen.getByTestId(CHIP);
    expect(chip.getAttribute("data-tone")).toBe("attention");
    expect(chip.textContent).toBe("Errands: 2 running");
    expect(chip.getAttribute("aria-label")).toBe("Errands: 2 running, needs attention");
  });

  it("falls back to a total with plural forms when nothing is live", () => {
    mocks.errands = [errand("e1", { status: "done" })];
    const { unmount } = render(<ErrandsChip taskId={TASK_ID} />);
    expect(screen.getByTestId(CHIP).textContent).toBe("1 errand");
    expect(screen.getByTestId(CHIP).getAttribute("data-tone")).toBe("idle");
    unmount();

    mocks.errands = [errand("e1", { status: "done" }), errand("e2", { status: "failed" })];
    render(<ErrandsChip taskId={TASK_ID} />);
    expect(screen.getByTestId(CHIP).textContent).toBe("2 errands");
  });
});

describe("ErrandsChip popover", () => {
  it("groups rows by parent session with status and elapsed time", async () => {
    mocks.errands = [
      errand("e1"),
      errand("e2", { parent_session_id: SECOND, status: "done" }),
      errand("e3", { status: "needs_attention" }),
    ];
    render(<ErrandsChip taskId={TASK_ID} />);
    openChip();

    const popover = await screen.findByTestId(POPOVER);
    expect(within(popover).getByText("3 total")).toBeTruthy();
    const groups = within(popover).getAllByTestId("errands-panel-group");
    expect(groups).toHaveLength(2);
    expect(within(groups[0]).getByRole("heading", { name: "Main" })).toBeTruthy();
    expect(within(groups[0]).getAllByRole("listitem")).toHaveLength(2);
    expect(within(groups[1]).getByRole("heading", { name: "Reviewer" })).toBeTruthy();
    const row = within(popover).getByTestId("errands-panel-row-e1");
    expect(row.textContent).toContain("Errand e1");
    expect(row.textContent).toContain("Running");
    expect(row.textContent).toContain("2m");
    expect(within(popover).getByTestId("errands-panel-row-e3").textContent).toContain(
      "Needs attention",
    );
  });

  it("opens an errand without a primary change and closes the popover", async () => {
    render(<ErrandsChip taskId={TASK_ID} />);
    openChip();
    const popover = await screen.findByTestId(POPOVER);

    fireEvent.click(within(popover).getByRole("button", { name: OPEN_E1 }));

    expect(mocks.openSession).toHaveBeenCalledWith({
      taskId: TASK_ID,
      sessionId: "e1",
      title: "Errand e1",
    });
    await waitFor(() => expect(screen.queryByTestId(POPOVER)).toBeNull());
  });

  it("disables Open for a queued errand that has no session yet", async () => {
    mocks.errands = [errand("e1", { status: "queued" })];
    render(<ErrandsChip taskId={TASK_ID} />);
    openChip();
    const popover = await screen.findByTestId(POPOVER);
    const open = within(popover).getByRole("button", { name: OPEN_E1 });
    expect((open as HTMLButtonElement).disabled).toBe(true);
  });

  it("stops every errand of the task with Stop all errands", async () => {
    render(<ErrandsChip taskId={TASK_ID} />);
    openChip();
    fireEvent.click(await screen.findByTestId(STOP_ALL));
    expect(mocks.cancelAllErrands).toHaveBeenCalledWith(TASK_ID);
    expect(mocks.toast).not.toHaveBeenCalled();
  });

  it("reports a failed Stop all errands", async () => {
    mocks.cancelAllErrands.mockRejectedValue(new Error("boom"));
    render(<ErrandsChip taskId={TASK_ID} />);
    openChip();
    fireEvent.click(await screen.findByTestId(STOP_ALL));
    await waitFor(() =>
      expect(mocks.toast).toHaveBeenCalledWith({
        title: "Could not stop the errands",
        variant: "error",
      }),
    );
  });

  it("disables Stop all errands while the request is in flight", async () => {
    mocks.pending.add(`cancel_all:${TASK_ID}`);
    render(<ErrandsChip taskId={TASK_ID} />);
    openChip();
    const stop = await screen.findByTestId(STOP_ALL);
    expect((stop as HTMLButtonElement).disabled).toBe(true);
  });

  it("omits Stop all errands when no errand is live", async () => {
    mocks.errands = [errand("e1", { status: "done" }), errand("e2", { status: "cancelled" })];
    render(<ErrandsChip taskId={TASK_ID} />);
    openChip();
    await screen.findByTestId(POPOVER);
    expect(screen.queryByTestId(STOP_ALL)).toBeNull();
  });

  it("shows a loading line until the errand list arrives", async () => {
    mocks.isLoaded = false;
    render(<ErrandsChip taskId={TASK_ID} />);
    openChip();
    expect(await screen.findByTestId("errands-panel-loading")).toBeTruthy();
  });
});

describe("ErrandsChip drawer", () => {
  beforeEach(() => {
    mocks.isMobile = true;
    mocks.finePointer = false;
  });

  it("opens a drawer with full-width rows and a Stop all errands footer", async () => {
    render(<ErrandsChip taskId={TASK_ID} />);
    openChip();

    const drawer = await screen.findByTestId("errands-drawer-content");
    expect(screen.queryByTestId(POPOVER)).toBeNull();
    expect(within(drawer).getByRole("heading", { name: "Errands" })).toBeTruthy();
    const row = within(drawer).getByRole("button", { name: "Open errand Errand e1, Running" });
    expect(row.className).toContain("min-h-11");
    expect(within(drawer).getByTestId(STOP_ALL).className).toContain("min-h-11");
    expect(within(drawer).getByTestId("errands-panel-list").className).toContain("overflow-y-auto");
  });

  it("opens the tapped errand and closes the drawer", async () => {
    render(<ErrandsChip taskId={TASK_ID} />);
    openChip();
    const drawer = await screen.findByTestId("errands-drawer-content");

    fireEvent.click(within(drawer).getByRole("button", { name: "Open errand Errand e1, Running" }));

    expect(mocks.openSession).toHaveBeenCalledWith({
      taskId: TASK_ID,
      sessionId: "e1",
      title: "Errand e1",
    });
    // vaul keeps the exiting content mounted until its animation ends.
    await waitFor(() =>
      expect(screen.getByTestId(CHIP).getAttribute("aria-expanded")).toBe("false"),
    );
  });

  it("omits the footer once nothing is live", async () => {
    mocks.errands = [errand("e1", { status: "done" })];
    render(<ErrandsChip taskId={TASK_ID} />);
    openChip();
    await screen.findByTestId("errands-drawer-content");
    expect(screen.queryByTestId(STOP_ALL)).toBeNull();
  });
});
