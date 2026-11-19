import { describe, expect, it } from "vitest";
import { kandevToolStemOf } from "@/components/task/chat/types";
import { kandevToolCall } from "../kandev-tool-message.test-helpers";
import { getKandevRenderer } from "./registry";
import {
  AwaitErrandsRenderer,
  CancelErrandRenderer,
  StartErrandRenderer,
} from "./errand-renderers";
import {
  ProposeMessageRenderer,
  ProposeMoveRenderer,
  ProposeResumeRenderer,
  ProposeTaskRenderer,
} from "./propose-task-renderer";

describe("getKandevRenderer proposal stems", () => {
  it("attaches the proposal card renderer to every propose tool", () => {
    expect(getKandevRenderer("propose_task")).toBe(ProposeTaskRenderer);
    expect(getKandevRenderer("propose_resume")).toBe(ProposeResumeRenderer);
    expect(getKandevRenderer("propose_message")).toBe(ProposeMessageRenderer);
    expect(getKandevRenderer("propose_move")).toBe(ProposeMoveRenderer);
  });
});

describe("getKandevRenderer errand stems", () => {
  it("registers the start, await, and cancel errand renderers", () => {
    expect(getKandevRenderer("start_errand")).toBe(StartErrandRenderer);
    expect(getKandevRenderer("await_errands")).toBe(AwaitErrandsRenderer);
    expect(getKandevRenderer("cancel_errand")).toBe(CancelErrandRenderer);
  });

  it.each([
    "mcp__kandev__start_errand_kandev",
    "kandev/start_errand_kandev",
    "kandev: start_errand_kandev",
    "start_errand_kandev",
  ])("matches the provider-prefixed tool name %s", (toolName) => {
    const stem = kandevToolStemOf(kandevToolCall({ toolName }));
    expect(stem).toBe("start_errand");
    expect(getKandevRenderer(stem)).toBe(StartErrandRenderer);
  });

  it("does not claim a foreign MCP server's errand tool", () => {
    expect(kandevToolStemOf(kandevToolCall({ toolName: "mcp__other__start_errand_kandev" }))).toBe(
      null,
    );
  });
});
