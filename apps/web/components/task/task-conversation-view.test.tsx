import { useState } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { ErrandInspection } from "@/lib/types/errands";
import { LinkedOwnedErrandContext } from "./chat/errands/owned-errand-navigation-context";
import { TaskConversationPanel, TaskConversationView } from "./task-conversation-view";

vi.mock("./chat/errands/owned-errand-conversation", () => ({
  OwnedErrandConversation: ({ errandId, onClose }: { errandId: string; onClose?: () => void }) => (
    <button type="button" data-testid={`owned-${errandId}`} onClick={onClose}>
      Back
    </button>
  ),
}));

afterEach(cleanup);
const PARENT_ID = "parent";
const CHILD_ID = "child";

function Conversation({ id, inactive }: { id: string | null; inactive: boolean }) {
  const [draft, setDraft] = useState("");
  return (
    <input
      data-testid={id ?? "empty"}
      data-inactive={String(inactive)}
      value={draft}
      onChange={(event) => setDraft(event.target.value)}
    />
  );
}

function View({
  sessionId,
  parentSessionId = null,
}: {
  sessionId: string;
  parentSessionId?: string | null;
}) {
  return (
    <TaskConversationView sessionId={sessionId} parentSessionId={parentSessionId}>
      {(id, inactive) => <Conversation id={id} inactive={inactive} />}
    </TaskConversationView>
  );
}

describe("temporary conversation view retention", () => {
  it("uses an authorized owned link only in its parent's panel and retains the parent on Back", () => {
    const linked = {
      scope: { task_id: "task", child_session_id: CHILD_ID },
      errand: { task_id: "task", errand_id: CHILD_ID, parent_session_id: PARENT_ID },
      child_session_exists: false,
      owns_conversation: true,
      parent: { available: true, id: PARENT_ID },
    } as ErrandInspection;
    render(
      <StateProvider>
        <LinkedOwnedErrandContext.Provider value={linked}>
          <TaskConversationPanel taskId="task" sessionId="sibling">
            {(id, inactive) => <Conversation id={id} inactive={inactive} />}
          </TaskConversationPanel>
          <TaskConversationPanel taskId="task" sessionId={PARENT_ID}>
            {(id, inactive) => <Conversation id={id} inactive={inactive} />}
          </TaskConversationPanel>
        </LinkedOwnedErrandContext.Provider>
      </StateProvider>,
    );
    const parent = screen.getByTestId(PARENT_ID);
    expect(parent.dataset.inactive).toBe("true");
    expect(screen.getByTestId("sibling").dataset.inactive).toBe("false");
    expect(screen.queryByTestId(CHILD_ID)).toBeNull();
    fireEvent.click(screen.getByTestId(`owned-${CHILD_ID}`));
    expect(screen.queryByTestId(`owned-${CHILD_ID}`)).toBeNull();
    expect(screen.getByTestId(PARENT_ID)).toBe(parent);
    expect(parent.dataset.inactive).toBe("false");
  });
  it("retains the parent's element and local state through child Open and Back", () => {
    const view = render(<View sessionId={PARENT_ID} />);
    const parent = screen.getByTestId(PARENT_ID);
    fireEvent.change(parent, { target: { value: "Unsent parent draft" } });
    view.rerender(<View sessionId={CHILD_ID} parentSessionId={PARENT_ID} />);
    expect(screen.getByTestId(PARENT_ID)).toBe(parent);
    expect(parent.dataset.inactive).toBe("true");
    expect(parent.parentElement?.hasAttribute("inert")).toBe(true);
    expect(parent.parentElement?.getAttribute("aria-hidden")).toBe("true");
    expect(screen.getByTestId(CHILD_ID).dataset.inactive).toBe("false");

    view.rerender(<View sessionId={PARENT_ID} />);
    expect(screen.getByTestId(PARENT_ID)).toBe(parent);
    expect((parent as HTMLInputElement).value).toBe("Unsent parent draft");
    expect(parent.dataset.inactive).toBe("false");
    expect(parent.parentElement?.hasAttribute("inert")).toBe(false);
    expect(screen.queryByTestId(CHILD_ID)).toBeNull();
  });

  it("does not carry local conversation state into a different normal session", () => {
    const view = render(<View sessionId={PARENT_ID} />);
    fireEvent.change(screen.getByTestId(PARENT_ID), { target: { value: "Parent draft" } });
    view.rerender(<View sessionId="other-parent" />);
    expect(screen.queryByTestId(PARENT_ID)).toBeNull();
    expect((screen.getByTestId("other-parent") as HTMLInputElement).value).toBe("");
  });

  it("keeps a direct child with no retained parent as the only conversation", () => {
    render(<View sessionId="orphan" />);
    expect(screen.getAllByRole("textbox")).toHaveLength(1);
    expect(screen.getByTestId("orphan").dataset.inactive).toBe("false");
  });

  it("retains the parent while replacing local state between different children", () => {
    const view = render(<View sessionId={CHILD_ID} parentSessionId={PARENT_ID} />);
    const parent = screen.getByTestId(PARENT_ID);
    fireEvent.change(screen.getByTestId(CHILD_ID), { target: { value: "Child draft" } });
    view.rerender(<View sessionId="next-child" parentSessionId={PARENT_ID} />);
    expect(screen.getByTestId(PARENT_ID)).toBe(parent);
    expect(screen.queryByTestId(CHILD_ID)).toBeNull();
    expect((screen.getByTestId("next-child") as HTMLInputElement).value).toBe("");
  });
});
