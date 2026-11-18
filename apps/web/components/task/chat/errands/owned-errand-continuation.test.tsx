import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { WebSocketRequestError } from "@/lib/ws/request-error";
import { OwnedErrandContinuation } from "./owned-errand-continuation";

const request = vi.fn();

vi.mock("@/components/state-provider", () => ({
  useAppStore: () => "enter",
}));
vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request }),
}));
vi.mock("../tiptap-input", () => ({
  TipTapInput: ({
    value,
    disabled,
    onChange,
  }: {
    value: string;
    disabled: boolean;
    onChange: (value: string) => void;
  }) => (
    <input
      aria-label="Message"
      value={value}
      disabled={disabled}
      onChange={(event) => onChange(event.target.value)}
    />
  ),
}));

afterEach(() => {
  cleanup();
  request.mockReset();
});

it("retains a refused draft and retries the same owned message identity", async () => {
  const sent = vi.fn();
  request
    .mockRejectedValueOnce(
      new WebSocketRequestError("blocked", "CONFLICT", { reason: "errand_recovery_unresolved" }),
    )
    .mockResolvedValueOnce({ turn_id: "turn-2" });
  render(
    <OwnedErrandContinuation
      taskId="task-1"
      errandId="errand-1"
      available
      connected
      onSent={sent}
    />,
  );

  const draft = screen.getByRole<HTMLInputElement>("textbox", { name: "Message" });
  const send = screen.getByRole("button", { name: "Send" });
  fireEvent.change(draft, { target: { value: "Follow up" } });
  fireEvent.click(send);
  expect((await screen.findByRole("alert")).textContent).toContain("Recovery must verify");
  expect(draft.value).toBe("Follow up");
  expect(sent).not.toHaveBeenCalled();
  fireEvent.click(send);
  fireEvent.click(send);

  await waitFor(() => expect(sent).toHaveBeenCalledOnce());
  expect(request).toHaveBeenCalledTimes(2);
  expect(request.mock.calls[0]).toEqual([
    "errand.continue",
    {
      task_id: "task-1",
      errand_id: "errand-1",
      client_message_id: expect.any(String),
      content: "Follow up",
    },
  ]);
  expect(request.mock.calls[1]).toEqual(request.mock.calls[0]);
  expect(draft.value).toBe("");
});
