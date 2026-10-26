import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { MessageRenderer } from "../message-renderer";

const openSession = vi.hoisted(() => vi.fn());
const STATUS_BLOCK = "<kandev-system>Errand status: e-1 running</kandev-system>";
const FINISHED_CARD = "errand-finished-card";
const USER_BUBBLE = "user-bubble";

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
  title: "Schema review",
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
    expect(cards[0].textContent).toContain("Errand finished");
    expect(cards[0].textContent).toContain("Schema review");
    expect(within(cards[0]).getByTestId("errand-finished-preview").textContent).toBe(
      SCHEMA_REVIEW.preview,
    );
    expect(within(cards[1]).getByTestId("errand-finished-status").textContent).toBe(
      "Cancelled by you",
    );
    expect(within(cards[1]).queryByTestId("errand-finished-preview")).toBeNull();
    expect(screen.getByRole("region", { name: "Errand finished: Schema review" })).toBe(cards[0]);
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

    fireEvent.click(screen.getByRole("button", { name: "Open errand Schema review" }));
    expect(openSession).toHaveBeenCalledWith({
      taskId: "task-1",
      sessionId: "e-1",
      title: "Schema review",
    });
  });

  it("renders untrusted preview markup as text", () => {
    renderMessage(
      userMessage({
        metadata: delivery([{ ...SCHEMA_REVIEW, preview: "<img src=x onerror=alert(1)>" }]),
      }),
    );

    const preview = screen.getByTestId("errand-finished-preview");
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
