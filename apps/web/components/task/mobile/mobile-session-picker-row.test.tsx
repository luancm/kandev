import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Errand } from "@/lib/types/errands";
import { MobileSessionPickerRow } from "./mobile-session-picker-row";

const TASK_ID = "task-1";
const PARENT_ID = "session-parent";
const ERRAND_ID = "session-errand";
const PILL = "mobile-sessions-pill";

const mocks = vi.hoisted(() => ({
  state: {} as Record<string, unknown>,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: <T,>(selector: (state: Record<string, unknown>) => T) => selector(mocks.state),
}));
vi.mock("./mobile-sessions-section", () => ({
  MobileSessionsPicker: ({ sessionId }: { sessionId: string | null }) => (
    <button type="button" data-testid={PILL} data-session-id={sessionId} />
  ),
}));

function errandRow(): Errand {
  return {
    errand_id: ERRAND_ID,
    task_id: TASK_ID,
    parent_session_id: PARENT_ID,
    title: "API audit",
    objective: "",
    agent_profile_id: "profile-1",
    status: "running",
    result_state: "none",
    result_preview: "",
    result_truncated: false,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    settled_at: null,
  };
}

function setState({
  enabled = true,
  errandRole = true,
  row = true,
}: { enabled?: boolean; errandRole?: boolean; row?: boolean } = {}) {
  mocks.state = {
    features: { errands: enabled },
    tasks: { activeTaskId: TASK_ID },
    taskSessions: {
      items: {
        [PARENT_ID]: { id: PARENT_ID, task_id: TASK_ID },
        [ERRAND_ID]: {
          id: ERRAND_ID,
          task_id: TASK_ID,
          role: errandRole ? "errand" : undefined,
          parent_session_id: PARENT_ID,
        },
      },
    },
    errands: { byTaskId: { [TASK_ID]: row ? { [ERRAND_ID]: errandRow() } : {} } },
    agentProfiles: { items: [] },
    sessionModels: { bySessionId: {} },
    activeModel: { bySessionId: {} },
  };
}

afterEach(cleanup);
beforeEach(() => setState());

describe("MobileSessionPickerRow", () => {
  it.each([true, false])(
    "shows the sessions pill for an ordinary session with errands=%s",
    (enabled) => {
      setState({ enabled });
      render(<MobileSessionPickerRow taskId={TASK_ID} sessionId={PARENT_ID} />);
      expect(screen.getByTestId("mobile-session-picker-row")).toBeTruthy();
      expect(screen.getByTestId(PILL).getAttribute("data-session-id")).toBe(PARENT_ID);
    },
  );

  it("shows the pill when there is no session yet", () => {
    render(<MobileSessionPickerRow taskId={TASK_ID} sessionId={null} />);
    expect(screen.getByTestId(PILL)).toBeTruthy();
  });

  it.each([true, false])("hides the row for a persisted errand with errands=%s", (enabled) => {
    setState({ enabled });
    render(<MobileSessionPickerRow taskId={TASK_ID} sessionId={ERRAND_ID} />);
    expect(screen.queryByTestId("mobile-session-picker-row")).toBeNull();
    expect(screen.queryByTestId(PILL)).toBeNull();
  });

  it("hides the row for an errand known only by its errand row", () => {
    setState({ errandRole: false });
    render(<MobileSessionPickerRow taskId={TASK_ID} sessionId={ERRAND_ID} />);
    expect(screen.queryByTestId(PILL)).toBeNull();
  });

  it("hides the row for a retained errand without a cached row while the flag is off", () => {
    setState({ enabled: false, row: false });
    render(<MobileSessionPickerRow taskId={TASK_ID} sessionId={ERRAND_ID} />);
    expect(screen.queryByTestId("mobile-session-picker-row")).toBeNull();
    expect(screen.queryByTestId(PILL)).toBeNull();
  });
});
