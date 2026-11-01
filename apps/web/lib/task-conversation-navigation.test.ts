import { describe, expect, it } from "vitest";
import type { TaskSession } from "@/lib/types/http";
import {
  conversationTabSessionId,
  errandParentSessionId,
  isErrandConversation,
  panelConversationSessionId,
} from "./task-conversation-navigation";

const sessions = [
  { id: "main" },
  { id: "review" },
  { id: "child", role: "errand", parent_session_id: "main" },
  { id: "other-child", role: "errand", parent_session_id: "review" },
] as TaskSession[];

describe("temporary errand conversations", () => {
  it("uses persisted roles and known errand rows while treating legacy role-less sessions as normal", () => {
    const child = { id: "child", role: "errand", parent_session_id: "main" } as TaskSession;
    const partialChild = { id: "child" } as TaskSession;
    const legacy = { id: "legacy" } as TaskSession;

    expect(isErrandConversation(child)).toBe(true);
    expect(isErrandConversation(partialChild, { child: { parent_session_id: "main" } })).toBe(true);
    expect(isErrandConversation(legacy)).toBe(false);
  });

  it("retains only the selected errand's known normal parent", () => {
    expect(errandParentSessionId("child", sessions)).toBe("main");
    expect(errandParentSessionId("other-child", sessions)).toBe("review");
    expect(errandParentSessionId("main", sessions)).toBeNull();
    expect(errandParentSessionId("loading", sessions)).toBeNull();
    expect(errandParentSessionId(null, sessions)).toBeNull();
  });
  it("does not retain an unrelated host for a missing or invalid parent", () => {
    const orphan = { id: "orphan", role: "errand", parent_session_id: "gone" } as TaskSession;
    const nested = { id: "nested", role: "errand", parent_session_id: "child" } as TaskSession;
    expect(errandParentSessionId("orphan", [...sessions, orphan])).toBeNull();
    expect(errandParentSessionId("nested", [...sessions, nested])).toBeNull();
  });
  it("retains a partially hydrated child's parent from the known errand row", () => {
    expect(
      errandParentSessionId("child", [{ id: "main" }, { id: "child" }] as TaskSession[], {
        child: { parent_session_id: "main" },
      }),
    ).toBe("main");
  });
  it("keeps both normal conversations as independent tab hosts", () => {
    expect(conversationTabSessionId("main", sessions)).toBe("main");
    expect(conversationTabSessionId("review", sessions)).toBe("review");
    expect(conversationTabSessionId("child", sessions)).toBe("main");
    expect(conversationTabSessionId("other-child", sessions)).toBe("review");
  });
  it("shows a child only inside its parent's surface and returns to the normal conversation on selection", () => {
    expect(panelConversationSessionId("main", "child", sessions)).toBe("child");
    expect(panelConversationSessionId("review", "child", sessions)).toBe("review");
    expect(panelConversationSessionId("main", "main", sessions)).toBe("main");
    expect(panelConversationSessionId("main", "review", sessions)).toBe("main");
  });
  it("honors known errand identity before the partial session role arrives", () => {
    const partial = [{ id: "main" }, { id: "child" }] as TaskSession[];
    expect(
      conversationTabSessionId("child", partial, { child: { parent_session_id: "main" } }),
    ).toBe("main");
  });
  it("retains orphan transcript access without manufacturing a regular errand tab", () => {
    const orphan = [{ id: "child", role: "errand", parent_session_id: "gone" }] as TaskSession[];
    expect(conversationTabSessionId("child", orphan)).toBeNull();
    expect(panelConversationSessionId(null, "child", orphan)).toBe("child");
    expect(conversationTabSessionId("child", [...orphan, ...sessions.slice(0, 2)])).toBe("main");
  });
  it("does not pick another conversation before the selected row has loaded", () => {
    expect(conversationTabSessionId("loading", [])).toBe("loading");
    expect(conversationTabSessionId(null, sessions)).toBeNull();
  });
});
