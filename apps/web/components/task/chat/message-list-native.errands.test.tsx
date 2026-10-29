import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { NativeMessageList } from "./message-list-native";
const changes = vi.hoisted(() => ({ summaries: [], ensureTurnsLoaded: vi.fn() }));
vi.mock("@/hooks/domains/session/use-turn-changes", () => ({
  useSessionTurnChanges: () => changes,
}));
vi.mock("@/hooks/use-lazy-load-messages", () => ({
  OLDER_PAGE_LIMIT: 20,
  useLazyLoadMessages: () => ({ loadMore: async () => 0, hasMore: false, isLoadingMore: false }),
}));
afterEach(cleanup);
// @covers AC-TASKS-ERRANDS-010.10 AC-TASKS-ERRANDS-011.6
it("keeps the current-state tail inside the transcript scroll and resize owner", () => {
  const view = render(
    <StateProvider>
      <NativeMessageList
        items={[]}
        messages={[]}
        permissionsByToolCallId={new Map()}
        childrenByParentToolCallId={new Map()}
        taskId="task"
        sessionId="parent"
        messagesLoading={false}
        isWorking={false}
        sessionState="WAITING_FOR_INPUT"
        transcriptTail={<button data-testid="current-errands">Errands</button>}
      />
    </StateProvider>,
  );
  const tail = screen.getByTestId("current-errands");
  expect(tail.closest("[data-chat-content]")).not.toBeNull();
  expect(tail.closest(".chat-message-list")).not.toBeNull();
  expect(view.container.querySelectorAll(".chat-message-list")).toHaveLength(1);
  view.rerender(
    <StateProvider>
      <NativeMessageList
        items={[]}
        messages={[]}
        permissionsByToolCallId={new Map()}
        childrenByParentToolCallId={new Map()}
        taskId="task"
        sessionId="parent"
        messagesLoading={false}
        isWorking={false}
        sessionState="WAITING_FOR_INPUT"
      />
    </StateProvider>,
  );
  expect(screen.queryByTestId("current-errands")).toBeNull();
});
