import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { MessageRenderer } from "../message-renderer";

const openSession = vi.hoisted(() => vi.fn());
const STATUS_BLOCK = "<kandev-system>Errand status: e-1 running</kandev-system>";
const FINISHED_CARD = "errand-finished-card";
const FINISHED_PREVIEW = "errand-finished-preview";
const FINISHED_STATUS = "errand-finished-status";
const USER_BUBBLE = "user-bubble";
const SCHEMA_TITLE = "Schema review";

vi.mock("@/components/state-provider", () => ({ useAppStore: () => null }));
vi.mock("./use-open-task-session", () => ({ useOpenTaskSession: () => openSession }));
vi.mock("@/components/task/chat/messages/chat-message", () => ({
  ChatMessage: ({ comment }: { comment: Message }) => (
    <div data-testid="user-bubble">{comment.content}</div>
  ),
}));

function userMessage(overrides: Partial<Message> = {}): Message {
  return {
    id: "msg-1",
    session_id: toSessionId("session-parent"),
    task_id: toTaskId("task-1"),
    author_type: "user",
    type: "message",
    content: "",
    created_at: "2026-10-26T12:00:00Z",
    ...overrides,
  };
}

function delivery(items: unknown[], route = "notification") {
  return { errand_delivery: { token: "tok-1", route, items } };
}

const SCHEMA_REVIEW = {
  errand_id: "e-1",
  title: SCHEMA_TITLE,
  status: "done",
  preview: "No breaking changes; two nullable columns added.",
};
const TEST_PLAN = { errand_id: "e-2", title: "Test plan", status: "cancelled", preview: "" };

function renderMessage(comment: Message) {
  return render(<MessageRenderer comment={comment} isTaskDescription={false} />);
}

afterEach(cleanup);
beforeEach(() => openSession.mockReset());

describe("Errand finished cards", () => {
  it("renders one card per result of a notification turn, with no empty user bubble", () => {
    renderMessage(
      userMessage({
        raw_content: STATUS_BLOCK,
        metadata: { ...delivery([SCHEMA_REVIEW, TEST_PLAN]), has_hidden_prompts: true },
      }),
    );

    expect(screen.queryByTestId(USER_BUBBLE)).toBeNull();
    const cards = screen.getAllByTestId(FINISHED_CARD);
    expect(cards.map((card) => card.getAttribute("data-errand-id"))).toEqual(["e-1", "e-2"]);
    expect(cards[0].textContent).toContain("1 errand finished");
    expect(within(cards[0]).queryByTestId("errand-finished-summary-duration")).toBeNull();
    expect(cards[0].textContent).not.toContain(SCHEMA_TITLE);
    for (const card of cards)
      fireEvent.click(within(card).getByRole("button", { expanded: false }));
    expect(cards[0].textContent).toContain(SCHEMA_TITLE);
    expect(within(cards[0]).getByTestId(FINISHED_PREVIEW).textContent).toBe(SCHEMA_REVIEW.preview);
    expect(within(cards[1]).getByTestId(FINISHED_STATUS).textContent).toBe("Cancelled by you");
    expect(within(cards[1]).queryByTestId(FINISHED_PREVIEW)).toBeNull();
  });

  it("shows the saved elapsed snapshot while collapsed and beside the expanded outcome", () => {
    renderMessage(
      userMessage({
        metadata: delivery([{ ...SCHEMA_REVIEW, duration_ms: 134000 }]),
      }),
    );

    const card = screen.getByTestId(FINISHED_CARD);
    expect(card.textContent).toContain("1 errand finished");
    expect(within(card).getByTestId("errand-finished-summary-duration").textContent).toBe("2m 14s");
    expect(within(card).queryByTestId(FINISHED_STATUS)).toBeNull();

    fireEvent.click(within(card).getByRole("button", { expanded: false }));
    expect(within(card).getByTestId(FINISHED_STATUS).textContent).toBe("Done");
    expect(within(card).getByTestId("errand-finished-duration").textContent).toBe("2m 14s");
    expect(within(card).getByTestId(FINISHED_PREVIEW).textContent).toBe(SCHEMA_REVIEW.preview);
  });

  it("keeps a recorded zero duration visible", () => {
    renderMessage(userMessage({ metadata: delivery([{ ...SCHEMA_REVIEW, duration_ms: 0 }]) }));

    expect(screen.getByTestId("errand-finished-summary-duration").textContent).toBe("0s");
  });

  it("places the cards below the user's own text", () => {
    renderMessage(
      userMessage({ content: "Any news on the API audit?", metadata: delivery([SCHEMA_REVIEW]) }),
    );

    const bubble = screen.getByTestId(USER_BUBBLE);
    const card = screen.getByTestId(FINISHED_CARD);
    expect(bubble.compareDocumentPosition(card) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("never renders a status block that reached content unstripped", () => {
    renderMessage(userMessage({ content: STATUS_BLOCK, metadata: delivery([SCHEMA_REVIEW]) }));

    expect(screen.queryByTestId(USER_BUBBLE)).toBeNull();
    expect(document.body.textContent).not.toContain("Errand status:");
  });

  it("opens the delivered errand from its card", () => {
    renderMessage(userMessage({ metadata: delivery([SCHEMA_REVIEW]) }));

    fireEvent.click(screen.getByRole("button", { expanded: false }));
    fireEvent.click(screen.getByRole("button", { name: "Open errand Schema review" }));
    expect(openSession).toHaveBeenCalledWith({
      taskId: "task-1",
      sessionId: "e-1",
      title: SCHEMA_TITLE,
    });
  });

  it("renders untrusted preview markup as text", () => {
    renderMessage(
      userMessage({
        metadata: delivery([{ ...SCHEMA_REVIEW, preview: "<img src=x onerror=alert(1)>" }]),
      }),
    );

    fireEvent.click(screen.getByRole("button", { expanded: false }));
    const preview = screen.getByTestId(FINISHED_PREVIEW);
    expect(preview.querySelector("img")).toBeNull();
    expect(preview.textContent).toBe("<img src=x onerror=alert(1)>");
  });

  it("leaves ordinary user messages and malformed delivery metadata alone", () => {
    renderMessage(
      userMessage({ content: "Hello", metadata: { errand_delivery: { items: [{}] } } }),
    );

    expect(screen.getByTestId(USER_BUBBLE).textContent).toBe("Hello");
    expect(screen.queryByTestId(FINISHED_CARD)).toBeNull();
  });
});
