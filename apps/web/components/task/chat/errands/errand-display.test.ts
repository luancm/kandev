import { describe, expect, it } from "vitest";
import type { AgentProfileOption } from "@/lib/state/slices";
import { errandElapsedLabel, resolveErrandProfile } from "./errand-display";

const CREATED = "2026-10-26T10:00:00Z";
const CREATED_MS = Date.parse(CREATED);

describe("errandElapsedLabel", () => {
  it("counts a live errand up to now", () => {
    expect(
      errandElapsedLabel(
        { status: "running", created_at: CREATED, settled_at: null, updated_at: CREATED },
        CREATED_MS + 2 * 60_000 + 5_000,
      ),
    ).toBe("2m");
  });

  it("freezes a settled errand at its settlement time", () => {
    expect(
      errandElapsedLabel(
        {
          status: "done",
          created_at: CREATED,
          settled_at: "2026-10-26T10:06:00Z",
          updated_at: "2026-10-26T10:09:00Z",
        },
        CREATED_MS + 60 * 60_000,
      ),
    ).toBe("6m");
  });

  it("uses updated_at when a settled errand has no settlement time", () => {
    expect(
      errandElapsedLabel(
        {
          status: "cancelled",
          created_at: CREATED,
          settled_at: null,
          updated_at: "2026-10-26T10:00:40Z",
        },
        CREATED_MS + 60 * 60_000,
      ),
    ).toBe("40s");
  });

  it.each(["", "0", "2026-02-30T10:00:00Z", "not a time"])(
    "returns nothing for a malformed creation time (%s)",
    (created) => {
      expect(
        errandElapsedLabel(
          { status: "running", created_at: created, settled_at: null, updated_at: CREATED },
          CREATED_MS,
        ),
      ).toBe("");
    },
  );
});

function profile(overrides: Partial<AgentProfileOption> = {}): AgentProfileOption {
  return {
    id: "profile-1",
    label: "Codex • Deep review",
    agent_id: "agent-codex",
    agent_name: "codex",
    cli_passthrough: false,
    model: "gpt-5.6",
    ...overrides,
  };
}

describe("resolveErrandProfile", () => {
  it("uses the profile name, model, and agent from the store", () => {
    expect(resolveErrandProfile([profile()], "profile-1", "Fallback")).toEqual({
      name: "Deep review",
      model: "gpt-5.6",
      agentName: "codex",
    });
  });

  it("omits an empty model and falls back to the agent part of the label", () => {
    expect(resolveErrandProfile([profile({ label: "Claude", model: "" })], "profile-1")).toEqual({
      name: "Claude",
      model: null,
      agentName: "codex",
    });
  });

  it("falls back to the reported name when the profile is not loaded", () => {
    expect(resolveErrandProfile([], "profile-9", "Reviewer")).toEqual({
      name: "Reviewer",
      model: null,
      agentName: null,
    });
  });

  it("reports no name when nothing is known", () => {
    expect(resolveErrandProfile([], null)).toEqual({ name: null, model: null, agentName: null });
  });
});
