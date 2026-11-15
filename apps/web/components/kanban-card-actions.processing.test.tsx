import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { t } from "@/lib/i18n";
import { KanbanCardActions } from "./kanban-card-actions";
import type { KanbanCardActionProps } from "./kanban-card-content";

vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => true }));

afterEach(cleanup);

function actions(props: Partial<KanbanCardActionProps> = {}) {
  return (
    <StateProvider>
      <TooltipProvider>
        <KanbanCardActions
          task={{ id: "task-1", title: "Task", workflowStepId: "step-1" }}
          menuEntries={[
            { kind: "item", key: "delete", label: "Delete", onSelect: () => undefined },
          ]}
          {...props}
        />
      </TooltipProvider>
    </StateProvider>
  );
}

describe("Kanban card processing menu", () => {
  it.each(["isDeleting", "isArchiving"] as const)(
    "does not reopen a closed menu when %s starts from a confirmation dialog",
    async (processing) => {
      const view = render(actions());
      const trigger = screen.getByRole("button", { name: t("kanban:moreOptions") });
      fireEvent.pointerDown(trigger, { button: 0, pointerId: 1 });
      fireEvent.click(trigger);
      fireEvent.click(await screen.findByRole("menuitem", { name: "Delete" }));
      await waitFor(() => expect(screen.queryByRole("menu")).toBeNull());

      view.rerender(actions({ [processing]: true }));

      expect(screen.queryByRole("menu")).toBeNull();
      expect(document.body.style.pointerEvents).not.toBe("none");
    },
  );

  it.each(["isDeleting", "isArchiving"] as const)(
    "keeps an already open menu mounted while %s is in progress",
    async (processing) => {
      const view = render(actions());
      const trigger = screen.getByRole("button", { name: t("kanban:moreOptions") });
      fireEvent.pointerDown(trigger, { button: 0, pointerId: 1 });
      fireEvent.click(trigger);
      await screen.findByRole("menu");

      view.rerender(actions({ [processing]: true }));
      fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });

      expect(screen.getByRole("menu")).toBeDefined();
    },
  );
});

describe("Kanban card errands summary", () => {
  it("keeps the backend errand count separate from native subagent count", () => {
    render(
      actions({
        task: {
          id: "task-1",
          title: "Task",
          workflowStepId: "step-1",
          activeSubagentCount: 4,
          statusSummary: {
            revision: 9,
            updated_at: "2026-10-28T12:00:00Z",
            errands: { live: 2, waiting: true },
          },
        },
      }),
    );

    expect(screen.getByTestId("task-errands-summary").textContent).toContain("2 errands");
    expect(screen.getByTestId("task-errands-waiting").textContent).toBe("Waiting on errands");
    expect(screen.getByTestId("task-subagent-count").textContent).toBe("4");
  });
});
