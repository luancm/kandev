import { describe, expect, it } from "vitest";
import { workflowMoveOptionsPayload } from "./workflow-move-options";

describe("workflowMoveOptionsPayload", () => {
  it("trims values and omits empty overrides", () => {
    expect(
      workflowMoveOptionsPayload({
        resetContext: true,
        instructions: "  create the PR ready for review  ",
        agentProfileId: "  qa-profile ",
        model: "   ",
      }),
    ).toEqual({
      reset_context: true,
      instructions: "create the PR ready for review",
      agent_profile_id: "qa-profile",
    });
  });

  it("returns undefined for a destination-only move", () => {
    expect(
      workflowMoveOptionsPayload({
        resetContext: false,
        instructions: "",
        agentProfileId: "",
        model: "",
      }),
    ).toBeUndefined();
  });
});
