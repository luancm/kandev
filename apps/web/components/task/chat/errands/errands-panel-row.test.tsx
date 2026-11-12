import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Errand } from "@/lib/types/errands";
import { ErrandsPanelRow } from "./errands-panel-row";

const TASK_ID = "task-1";
const ERRAND_ID = "errand-1";
const STOP_BUTTON_NAME = "Stop Review acceptance";

const mocks = vi.hoisted(() => ({
  errandId: "errand-1",
  cancelErrand: vi.fn(),
  openSession: vi.fn(),
  toast: vi.fn(),
}));

vi.mock("@/hooks/domains/session/use-session-context-window", () => ({
  useSessionContextWindow: () => undefined,
}));
vi.mock("@/hooks/domains/session/use-errand-actions", () => ({
  useErrandActions: () => {
    const [pending, setPending] = useState(false);
    return {
      cancelErrand: async (sessionId: string) => {
        setPending(true);
        try {
          await mocks.cancelErrand(sessionId);
        } finally {
          setPending(false);
        }
      },
      isPending: (action: string, targetId: string) =>
        action === "cancel" && targetId === mocks.errandId && pending,
    };
  },
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: mocks.toast }) }));
vi.mock("./use-open-task-session", () => ({ useOpenTaskSession: () => mocks.openSession }));
vi.mock("./errand-profile-label", () => ({
  ErrandProfileLabel: () => <span>Agent</span>,
}));

function errand(status: Errand["status"] = "running", errandId = ERRAND_ID): Errand {
  return {
    errand_id: errandId,
    task_id: TASK_ID,
    parent_session_id: "parent-1",
    title: "Review acceptance",
    objective: "Review the change",
    agent_profile_id: "profile-1",
    status,
    result_state: "none",
    result_preview: "",
    result_truncated: false,
    created_at: new Date(Date.now() - 2 * 60_000).toISOString(),
    updated_at: new Date().toISOString(),
    settled_at: null,
  };
}

function renderRow({
  status = "running",
  drawer = false,
}: {
  status?: Errand["status"];
  drawer?: boolean;
} = {}) {
  return render(
    <ul>
      <ErrandsPanelRow
        errand={errand(status)}
        taskId={TASK_ID}
        nowMs={Date.now()}
        drawer={drawer}
        onOpened={vi.fn()}
      />
    </ul>,
  );
}

function renderTwoRows() {
  return render(
    <ul>
      {[ERRAND_ID, "errand-2"].map((errandId) => (
        <ErrandsPanelRow
          key={errandId}
          errand={errand("running", errandId)}
          taskId={TASK_ID}
          nowMs={Date.now()}
          drawer={false}
          onOpened={vi.fn()}
        />
      ))}
    </ul>,
  );
}

function panelRow(drawer = false) {
  return screen.getByTestId(
    drawer ? `errands-panel-row-actions-${ERRAND_ID}` : `errands-panel-row-${ERRAND_ID}`,
  );
}

function deferred<T>() {
  let resolve: (value: T) => void = () => {};
  let reject: (reason: Error) => void = () => {};
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

afterEach(cleanup);

beforeEach(() => {
  vi.clearAllMocks();
  mocks.cancelErrand.mockResolvedValue(undefined);
});

describe("ErrandsPanelRow Stop action", () => {
  it.each(["running", "queued", "needs_attention"] as const)(
    "offers a named Stop for a %s errand without nesting actions on phones",
    (status) => {
      renderRow({ status, drawer: true });

      const row = panelRow(true);
      const open = within(row).getByRole("button", { name: /Open errand Review acceptance/ });
      const stop = within(row).getByRole("button", { name: STOP_BUTTON_NAME });

      expect(open).toBeTruthy();
      expect((open as HTMLButtonElement).disabled).toBe(status === "queued");
      expect(stop.textContent).toContain("Stop");
      expect(row.querySelector("button button")).toBeNull();
    },
  );

  it("omits Stop for settled errands while retaining Open", () => {
    renderRow({ status: "done", drawer: true });

    const row = panelRow(true);
    expect(within(row).getByRole("button", { name: /Open errand Review acceptance/ })).toBeTruthy();
    expect(within(row).queryByRole("button", { name: /Stop/ })).toBeNull();
  });

  it("cancels only the selected errand and leaves Open and the row available", async () => {
    renderRow();
    const row = panelRow();

    fireEvent.click(within(row).getByRole("button", { name: STOP_BUTTON_NAME }));

    await waitFor(() => expect(mocks.cancelErrand).toHaveBeenCalledExactlyOnceWith(ERRAND_ID));
    expect(mocks.openSession).not.toHaveBeenCalled();
    expect(panelRow()).toBeTruthy();
    expect(within(row).getByRole("button", { name: /Open errand Review acceptance/ })).toBeTruthy();
  });

  it("disables only the selected Stop while cancellation is pending", async () => {
    const request = deferred<void>();
    mocks.cancelErrand.mockReturnValue(request.promise);
    renderTwoRows();
    const stop = screen.getByTestId(`errand-stop-${ERRAND_ID}`);
    const siblingStop = screen.getByTestId("errand-stop-errand-2");

    fireEvent.click(stop);

    await waitFor(() => expect((stop as HTMLButtonElement).disabled).toBe(true));
    expect((siblingStop as HTMLButtonElement).disabled).toBe(false);
    expect(stop.getAttribute("aria-busy")).toBe("true");
    expect(stop.textContent).toContain("Stop");
    const row = stop.closest("li")!;
    const status = Array.from(row.querySelectorAll('[role="status"]')).find(
      (region) => region.textContent?.trim() === "Stopping Review acceptance",
    );
    expect(status?.getAttribute("aria-live")).toBe("polite");
    expect(status?.textContent?.trim()).toBe("Stopping Review acceptance");
    expect(within(row).getByRole("button", { name: STOP_BUTTON_NAME })).toBe(stop);
    fireEvent.click(stop);
    expect(mocks.cancelErrand).toHaveBeenCalledExactlyOnceWith(ERRAND_ID);
    await act(async () => {
      request.resolve();
      await request.promise;
    });
  });

  it("reports cancellation failure and keeps the live row available", async () => {
    mocks.cancelErrand.mockRejectedValue(new Error("cancel refused"));
    renderRow();
    const row = panelRow();

    fireEvent.click(within(row).getByRole("button", { name: STOP_BUTTON_NAME }));

    await waitFor(() =>
      expect(mocks.toast).toHaveBeenCalledWith({
        title: "Could not stop the errand",
        variant: "error",
      }),
    );
    expect(panelRow()).toBeTruthy();
    expect(
      (
        within(row).getByRole("button", {
          name: STOP_BUTTON_NAME,
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false);
  });
});
