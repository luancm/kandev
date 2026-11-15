import { useState } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TaskConversationView } from "./task-conversation-view";

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
