import { describe, expect, it } from "vitest";
import { WebSocketRequestError } from "@/lib/ws/request-error";
import {
  isErrandRecoveryRefusal,
  isMessageSendError,
  MessageSendError,
} from "./message-send-error";

describe("isMessageSendError", () => {
  it("classifies session-unavailable so submission shows a deterministic not-sent error", () => {
    const error = new MessageSendError(
      "session-unavailable",
      "The selected session is not available for input.",
    );

    expect(isMessageSendError(error)).toBe(true);
    expect(error.message).toBe("The selected session is not available for input.");
  });

  it("does not classify an arbitrary transport error as deterministically not sent", () => {
    expect(isMessageSendError(new Error("timeout"))).toBe(false);
  });
});

describe("isErrandRecoveryRefusal", () => {
  it("recognizes only the stable conflict reason", () => {
    expect(
      isErrandRecoveryRefusal(
        new WebSocketRequestError("refused", "CONFLICT", {
          reason: "errand_recovery_unresolved",
        }),
      ),
    ).toBe(true);
  });

  it.each([
    new WebSocketRequestError("refused", "CONFLICT", { reason: "other_conflict" }),
    new WebSocketRequestError("refused", "UNAVAILABLE", {
      reason: "errand_recovery_unresolved",
    }),
    new Error("errand_recovery_unresolved"),
  ])("does not classify an unrelated error as an Errand recovery refusal", (error) => {
    expect(isErrandRecoveryRefusal(error)).toBe(false);
  });
});
