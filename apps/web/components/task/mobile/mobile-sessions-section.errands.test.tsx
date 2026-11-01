import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { MobileSessionsPicker } from "./mobile-sessions-section";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { Errand } from "@/lib/types/errands";
import type { TaskSession } from "@/lib/types/http";

const mocks = vi.hoisted(() => ({
  activeSessionId: "session-a" as string | null,
  sessions: [] as TaskSession[],
  setActiveSession: vi.fn(),
  errandsEnabled: false,
  errands: [] as Errand[],
}));

const PROFILES: AgentProfileOption[] = [
  {
    id: "profile-a",
    label: "Mock Agent • Alpha",
    agent_id: "agent-claude",
    agent_name: "claude",
    cli_passthrough: false,
  },
  {
    id: "profile-b",
    label: "Mock Agent • Beta",
    agent_id: "agent-codex",
    agent_name: "codex",
    cli_passthrough: false,
  },
];

vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: () => ({ sessions: mocks.sessions, isLoading: false, isLoaded: true }),
}));

vi.mock("@/hooks/domains/session/use-task-errands", () => ({
  useTaskErrands: () => ({
    errands: mocks.errandsEnabled ? mocks.errands : [],
    errandsByParent: {},
    liveCount: 0,
    isLoaded: true,
  }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      features: { dynamicAgentRouting: false, errands: mocks.errandsEnabled },
      taskSessions: {
        items: Object.fromEntries(mocks.sessions.map((session) => [session.id, session])),
      },
      tasks: { activeSessionId: mocks.activeSessionId },
      agentProfiles: { items: PROFILES },
      kanban: { tasks: [{ id: "task-1", primarySessionId: "session-a" }] },
      repositories: { itemsByWorkspaceId: {} },
      messages: { bySession: {} },
      turns: { bySession: {} },
      executors: { items: [] },
      setActiveSession: mocks.setActiveSession,
    }),
}));

vi.mock("@/components/agent-logo", () => ({
  AgentLogo: ({ agentName }: { agentName: string }) => (
    <span data-testid={`agent-logo-${agentName}`} />
  ),
}));

vi.mock("@/hooks/domains/session/use-session-actions", () => ({
  useSessionActions: () => ({
    setPrimary: vi.fn(),
    stop: vi.fn(),
    resume: vi.fn(),
    restart: vi.fn(),
    isRestarting: false,
    remove: vi.fn(),
  }),
  isSessionStoppable: (state: string) =>
    ["RUNNING", "STARTING", "WAITING_FOR_INPUT"].includes(state),
  isSessionDeletable: () => true,
  isSessionResumable: () => false,
}));

const PILL_TESTID = "mobile-sessions-pill";
const SESSION_ACTIONS_LABEL = "Session actions";
const SESSION_B_ROW = "mobile-session-row-session-b";
const SESSION_A = "session-a";
const TASK_ID = "task-1";
const START_TIME = "2026-01-01T00:00:00Z";
const SECOND_TIME = "2026-01-01T00:01:00Z";

function session(
  id: string,
  profileId: string,
  startedAt: string,
  overrides: Partial<TaskSession> = {},
): TaskSession {
  return {
    id,
    task_id: TASK_ID,
    agent_profile_id: profileId,
    state: "WAITING_FOR_INPUT",
    started_at: startedAt,
    updated_at: startedAt,
    ...overrides,
  } as TaskSession;
}

afterEach(cleanup);

beforeEach(() => {
  mocks.activeSessionId = SESSION_A;
  mocks.sessions = [];
  mocks.setActiveSession.mockReset();
  mocks.errandsEnabled = false;
  mocks.errands = [];
});

const ERRAND_ID = "session-errand";
const ERRAND_ROW = `mobile-errand-row-${ERRAND_ID}`;
const THIRD_TIME = "2026-01-01T00:02:00Z";

function errandSession(overrides: Partial<TaskSession> = {}): TaskSession {
  return session(ERRAND_ID, "profile-b", THIRD_TIME, {
    role: "errand",
    parent_session_id: SESSION_A as TaskSession["parent_session_id"],
    state: "RUNNING",
    ...overrides,
  });
}

function errandRow(overrides: Partial<Errand> = {}): Errand {
  return {
    errand_id: ERRAND_ID,
    task_id: TASK_ID,
    parent_session_id: SESSION_A,
    title: "API audit",
    objective: "Audit the handlers",
    agent_profile_id: "profile-b",
    status: "running",
    result_state: "none",
    result_preview: "",
    result_truncated: false,
    created_at: THIRD_TIME,
    updated_at: THIRD_TIME,
    settled_at: null,
    ...overrides,
  };
}

function openPicker() {
  render(<MobileSessionsPicker taskId={TASK_ID} fullWidth />);
  fireEvent.click(screen.getByTestId(PILL_TESTID));
}

describe("MobileSessionsPicker errands", () => {
  beforeEach(() => {
    mocks.errandsEnabled = true;
    mocks.sessions = [
      session(SESSION_A, "profile-a", START_TIME),
      session("session-b", "profile-b", SECOND_TIME),
      errandSession(),
    ];
    mocks.errands = [errandRow()];
  });

  it("lists and numbers only normal conversations while errands stay in the parent panel", () => {
    openPicker();
    expect(screen.queryByTestId(ERRAND_ROW)).toBeNull();
    expect(screen.queryByTestId(`mobile-session-row-${ERRAND_ID}`)).toBeNull();
    expect(screen.getByText("2 sessions")).toBeTruthy();
    expect(within(screen.getByTestId(SESSION_B_ROW)).getByText("2")).toBeTruthy();
    fireEvent.click(screen.getByTestId(SESSION_B_ROW));
    expect(mocks.setActiveSession).toHaveBeenCalledExactlyOnceWith(TASK_ID, "session-b");
  });

  it("keeps the parent identity in the session picker while the child is displayed", () => {
    mocks.activeSessionId = ERRAND_ID;
    render(<MobileSessionsPicker taskId={TASK_ID} sessionId={ERRAND_ID} fullWidth />);
    expect(screen.getByTestId(PILL_TESTID).textContent).toContain("Alpha");
  });

  it("hides the sessions of the errand from the pill count", () => {
    render(<MobileSessionsPicker taskId={TASK_ID} sessionId={SESSION_A} fullWidth />);
    expect(screen.getByTestId(PILL_TESTID).textContent).toContain("1/2");
  });
});

describe("MobileSessionsPicker errand sessions with the errands flag off", () => {
  beforeEach(() => {
    mocks.sessions = [session(SESSION_A, "profile-a", START_TIME), errandSession()];
  });

  async function openMenu(rowTestId: string) {
    openPicker();
    fireEvent.pointerDown(
      within(screen.getByTestId(rowTestId)).getByRole("button", { name: SESSION_ACTIONS_LABEL }),
      { button: 0, ctrlKey: false },
    );
    return screen.findByRole("menu");
  }

  it("keeps retained errands out of normal rows while the flag is off", () => {
    openPicker();
    expect(screen.queryByTestId(`mobile-session-row-${ERRAND_ID}`)).toBeNull();
  });

  it("keeps Set as Primary on an ordinary session", async () => {
    const menu = await openMenu("mobile-session-row-session-a");
    expect(within(menu).getByRole("menuitem", { name: "Set as Primary" })).toBeTruthy();
    expect(within(menu).getByTestId("session-handoff-submenu")).toBeTruthy();
  });
});
