import { describe, expect, it } from "vitest";
import {
  hasCausalReplaySettlement,
  replaySettlementSessionId,
} from "./workflow-step-move-overrides-replay";

describe("workflow move replay settlement", () => {
  it("settles the destination session before checking its prompt", () => {
    expect(replaySettlementSessionId({ targetSessionId: "target-session" }, "source-session")).toBe(
      "target-session",
    );
  });

  it("falls back to the source session when no destination is supplied", () => {
    expect(replaySettlementSessionId({}, "source-session")).toBe("source-session");
  });

  it("does not treat a pre-existing settled state as replay settlement", () => {
    expect(hasCausalReplaySettlement({ state: "IDLE", backendRestarted: false })).toBe(false);
  });

  it("accepts settled state only after the restart boundary", () => {
    expect(hasCausalReplaySettlement({ state: "IDLE", backendRestarted: true })).toBe(true);
  });
});
