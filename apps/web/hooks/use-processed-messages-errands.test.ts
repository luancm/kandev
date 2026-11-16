import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { buildGroupedRenderItems } from "./use-processed-messages";

function toolCall(id: string, toolName: string): Message {
  return {
    id,
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: toolName,
    type: "tool_call",
    created_at: "",
    turn_id: "turn-1",
    metadata: {
      title: toolName,
      status: "complete",
      normalized: { kind: "generic", generic: { name: "other", input: {}, output: null } },
    },
  };
}

function shell(id: string): Message {
  return {
    ...toolCall(id, "Bash"),
    type: "tool_execute",
    metadata: {
      status: "complete",
      normalized: { shell_exec: { command: "ls", output: { exit_code: 0 } } },
    },
  };
}

describe("buildGroupedRenderItems errand hoisting", () => {
  it("keeps each errand start and await call visible as its own row", () => {
    const items = buildGroupedRenderItems(
      [
        shell("a"),
        shell("b"),
        toolCall("start-1", "mcp__kandev__start_errand_kandev"),
        toolCall("start-2", "kandev/start_errand_kandev"),
        toolCall("await", "mcp__kandev__await_errands_kandev"),
        shell("c"),
        shell("d"),
      ],
      "s1",
      { canAnchorPrepareProgress: false },
    );

    expect(items.map((item) => item.type)).toEqual([
      "turn_group",
      "message",
      "message",
      "message",
      "turn_group",
    ]);
    expect(items.slice(1, 4).map((item) => item.type === "message" && item.message.id)).toEqual([
      "start-1",
      "start-2",
      "await",
    ]);
  });

  it("still groups an agent's cancel call with surrounding activity", () => {
    const items = buildGroupedRenderItems(
      [shell("a"), toolCall("cancel", "mcp__kandev__cancel_errand_kandev")],
      "s1",
      { canAnchorPrepareProgress: false },
    );

    expect(items.map((item) => item.type)).toEqual(["turn_group"]);
  });
});
