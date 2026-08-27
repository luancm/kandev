import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, fireEvent } from "@testing-library/react";
import { ContextMenu, ContextMenuContent, ContextMenuTrigger } from "@kandev/ui/context-menu";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import { pluginRegistry } from "@/lib/plugins/registry";
import {
  buildKanbanCardMenuEntries,
  KanbanCardContextMenuItems,
  KanbanCardDropdownMenuItems,
  type KanbanCardMenuEntry,
} from "./kanban-card-menu-items";

afterEach(cleanup);

const PluginBitbucketIcon = () => null;
const moveArgs = {
  currentWorkflowId: "workflow-1",
  currentStepId: "step-1",
  workflows: [{ id: "workflow-1", name: "Workflow 1" }],
  stepsByWorkflowId: {
    "workflow-1": [
      { id: "step-1", title: "Todo" },
      { id: "step-2", title: "Review" },
    ],
  },
};

// Regression: React synthetic events bubble through the fiber tree from a Radix portal; without stopPropagation the parent Card's onClick fires instead of the confirm dialog.
describe("KanbanCardDropdownMenuItems — click propagation", () => {
  function renderWithParent(entries: KanbanCardMenuEntry[], parentOnClick: () => void) {
    return render(
      <div data-testid="parent-card" onClick={parentOnClick}>
        <DropdownMenu defaultOpen>
          <DropdownMenuTrigger>open</DropdownMenuTrigger>
          <DropdownMenuContent>
            <KanbanCardDropdownMenuItems entries={entries} />
          </DropdownMenuContent>
        </DropdownMenu>
      </div>,
    );
  }

  it("clicking a menu item does not call the parent card's onClick", () => {
    const onDelete = vi.fn();
    const parentOnClick = vi.fn();
    const entries: KanbanCardMenuEntry[] = [
      {
        kind: "item",
        key: "delete",
        label: "Delete",
        onSelect: onDelete,
      },
    ];

    renderWithParent(entries, parentOnClick);

    const deleteItem = screen.getByRole("menuitem", { name: /delete/i });
    fireEvent.click(deleteItem);

    expect(onDelete).toHaveBeenCalledTimes(1);
    expect(parentOnClick).not.toHaveBeenCalled();
  });

  it("clicking an archive menu item does not call the parent card's onClick", () => {
    const onArchive = vi.fn();
    const parentOnClick = vi.fn();
    const entries: KanbanCardMenuEntry[] = [
      {
        kind: "item",
        key: "archive",
        label: "Archive",
        onSelect: onArchive,
      },
    ];

    renderWithParent(entries, parentOnClick);

    fireEvent.click(screen.getByRole("menuitem", { name: /archive/i }));

    expect(onArchive).toHaveBeenCalledTimes(1);
    expect(parentOnClick).not.toHaveBeenCalled();
  });

  it("pointer-down on a menu item does not reach the parent (dnd-kit guard)", () => {
    const parentOnPointerDown = vi.fn();
    const entries: KanbanCardMenuEntry[] = [
      { kind: "item", key: "delete", label: "Delete", onSelect: vi.fn() },
    ];

    render(
      <div data-testid="parent-card" onPointerDown={parentOnPointerDown}>
        <DropdownMenu defaultOpen>
          <DropdownMenuTrigger>open</DropdownMenuTrigger>
          <DropdownMenuContent>
            <KanbanCardDropdownMenuItems entries={entries} />
          </DropdownMenuContent>
        </DropdownMenu>
      </div>,
    );

    fireEvent.pointerDown(screen.getByRole("menuitem", { name: /delete/i }));

    expect(parentOnPointerDown).not.toHaveBeenCalled();
  });
});

describe("buildKanbanCardMenuEntries — external issue links", () => {
  function itemLabels(entry: KanbanCardMenuEntry | undefined) {
    if (entry?.kind !== "submenu") return [];
    return entry.children.filter((child) => child.kind === "item").map((child) => child.label);
  }

  it("adds configured external issue providers to the Link submenu", () => {
    const entries = buildKanbanCardMenuEntries({
      workflows: [],
      stepsByWorkflowId: {},
      onLinkPullRequest: vi.fn(),
      onLinkIssue: vi.fn(),
      onLinkMergeRequest: vi.fn(),
      onLinkJiraTicket: vi.fn(),
      onLinkLinearIssue: vi.fn(),
      onLinkSentryIssue: vi.fn(),
    });

    const linkMenu = entries.find((entry) => entry.kind === "submenu" && entry.key === "link");
    expect(linkMenu?.kind).toBe("submenu");

    expect(itemLabels(linkMenu)).toEqual([
      "GitHub Pull Request",
      "GitHub Issue",
      "GitLab Merge Request",
      "Jira Ticket",
      "Linear Issue",
      "Sentry Issue",
    ]);
  });

  it("omits external issue providers that are not configured", () => {
    const entries = buildKanbanCardMenuEntries({
      workflows: [],
      stepsByWorkflowId: {},
      onLinkPullRequest: vi.fn(),
      onLinkIssue: vi.fn(),
      onLinkJiraTicket: vi.fn(),
    });

    const linkMenu = entries.find((entry) => entry.kind === "submenu" && entry.key === "link");
    expect(linkMenu?.kind).toBe("submenu");

    expect(itemLabels(linkMenu)).toEqual(["GitHub Pull Request", "GitHub Issue", "Jira Ticket"]);
  });

  it("adds registered Link actions to the native Link submenu", () => {
    const entries = buildKanbanCardMenuEntries({
      workflows: [],
      stepsByWorkflowId: {},
      onLinkPullRequest: vi.fn(),
      pluginLinkActions: [
        {
          id: "bitbucket-pull-request",
          label: "Bitbucket Pull Request",
          icon: PluginBitbucketIcon,
          onSelect: vi.fn(),
        },
      ],
    } as never);

    const linkMenu = entries.find((entry) => entry.kind === "submenu" && entry.key === "link");
    expect(itemLabels(linkMenu)).toContain("Bitbucket Pull Request");
    const bitbucket =
      linkMenu?.kind === "submenu"
        ? linkMenu.children.find(
            (entry) => entry.kind === "item" && entry.key === "link-plugin-bitbucket-pull-request",
          )
        : undefined;
    expect(bitbucket?.kind).toBe("item");
    if (bitbucket?.kind === "item") {
      expect((bitbucket.icon as { type?: unknown })?.type).toBe(PluginBitbucketIcon);
    }
  });
});

describe("buildKanbanCardMenuEntries — move options", () => {
  it("nests move options under each direct step entry", () => {
    const onMoveToStep = vi.fn();
    const onMoveToStepWithOptions = vi.fn();
    const entries = buildKanbanCardMenuEntries({
      ...moveArgs,
      onMoveToStep,
      onMoveToStepWithOptions,
    });

    const moveToIndex = entries.findIndex((entry) => entry.key === "move-to");
    expect(moveToIndex).toBeGreaterThanOrEqual(0);
    expect(entries.some((entry) => entry.key === "move-with-options")).toBe(false);

    const directMenu = entries[moveToIndex];
    expect(directMenu.kind).toBe("submenu");
    if (directMenu.kind !== "submenu") return;

    const directStep = directMenu.children.find((entry) => entry.key === "step-step-2");
    expect(directStep?.kind).toBe("submenu");
    if (directStep?.kind !== "submenu") return;

    expect(directStep.children).toHaveLength(1);
    const optionsEntry = directStep.children[0];
    expect(optionsEntry.kind).toBe("item");
    if (optionsEntry.kind !== "item") return;
    expect(optionsEntry.label).toBe("Move with options");

    directStep.onSelect?.();
    optionsEntry.onSelect?.();

    expect(onMoveToStep).toHaveBeenCalledWith("step-2");
    expect(onMoveToStepWithOptions).toHaveBeenCalledWith("step-2", undefined);
  });

  it("keeps plain direct step entries without a single-task options handler", () => {
    const entries = buildKanbanCardMenuEntries({
      ...moveArgs,
      onMoveToStep: vi.fn(),
    });

    const moveTo = entries.find((entry) => entry.key === "move-to");
    expect(moveTo?.kind).toBe("submenu");
    if (moveTo?.kind !== "submenu") return;
    expect(moveTo.children.every((entry) => entry.kind === "item")).toBe(true);
  });

  it("keeps the current step disabled when options are available", () => {
    const entries = buildKanbanCardMenuEntries({
      ...moveArgs,
      onMoveToStep: vi.fn(),
      onMoveToStepWithOptions: vi.fn(),
    });

    const moveTo = entries.find((entry) => entry.key === "move-to");
    expect(moveTo?.kind).toBe("submenu");
    if (moveTo?.kind !== "submenu") return;

    const currentStep = moveTo.children.find((entry) => entry.key === "step-step-1");
    expect(currentStep?.kind).toBe("submenu");
    if (currentStep?.kind !== "submenu") return;
    expect(currentStep.disabled).toBe(true);
    expect(currentStep.children[0].kind === "item" && currentStep.children[0].disabled).toBe(true);
  });
});

function renderNestedMoveOptionsMenu(
  type: "context" | "dropdown",
  onMoveToStep: (stepId: string) => void,
  onMoveToStepWithOptions: (stepId: string) => void,
  parentOnClick: () => void,
) {
  const entries = buildKanbanCardMenuEntries({
    ...moveArgs,
    onMoveToStep,
    onMoveToStepWithOptions,
  });

  if (type === "context") {
    render(
      <div onClick={parentOnClick}>
        <ContextMenu>
          <ContextMenuTrigger data-testid="context-trigger">Task</ContextMenuTrigger>
          <ContextMenuContent>
            <KanbanCardContextMenuItems entries={entries} />
          </ContextMenuContent>
        </ContextMenu>
      </div>,
    );
    fireEvent.contextMenu(screen.getByTestId("context-trigger"));
    return;
  }

  render(
    <div onClick={parentOnClick}>
      <DropdownMenu defaultOpen>
        <DropdownMenuTrigger>open</DropdownMenuTrigger>
        <DropdownMenuContent>
          <KanbanCardDropdownMenuItems entries={entries} />
        </DropdownMenuContent>
      </DropdownMenu>
    </div>,
  );
}

async function openNestedMoveOptionsStep() {
  fireEvent.pointerMove(screen.getByTestId("task-context-move-to"), {
    pointerType: "mouse",
  });
  const step = await screen.findByTestId("task-context-step-step-2");
  fireEvent.pointerMove(step, { pointerType: "mouse" });
  return step;
}

describe("KanbanCardMenuItems — nested move options renderers", () => {
  const renderMenu = renderNestedMoveOptionsMenu;
  const openStep = openNestedMoveOptionsStep;

  it.each(["context", "dropdown"] as const)(
    "keeps direct and optioned actions on the same step row for %s menus",
    async (type) => {
      const onMoveToStep = vi.fn();
      const onMoveToStepWithOptions = vi.fn();
      const parentOnClick = vi.fn();
      renderMenu(type, onMoveToStep, onMoveToStepWithOptions, parentOnClick);

      const step = await openStep();
      fireEvent.click(step);

      expect(onMoveToStep).toHaveBeenCalledOnce();
      expect(onMoveToStep).toHaveBeenCalledWith("step-2");
      expect(onMoveToStepWithOptions).not.toHaveBeenCalled();
      expect(parentOnClick).not.toHaveBeenCalled();
    },
  );

  it.each(["context", "dropdown"] as const)(
    "keeps a touch tap on the step label direct for %s menus",
    async (type) => {
      const onMoveToStep = vi.fn();
      const onMoveToStepWithOptions = vi.fn();
      renderMenu(type, onMoveToStep, onMoveToStepWithOptions, vi.fn());

      const step = await openStep();
      fireEvent.pointerDown(step, { pointerType: "touch", clientX: 0 });
      fireEvent.click(step, { clientX: 0 });

      expect(onMoveToStep).toHaveBeenCalledOnce();
      expect(onMoveToStepWithOptions).not.toHaveBeenCalled();
    },
  );

  it.each(["context", "dropdown"] as const)(
    "opens nested options from the step chevron on touch for %s menus",
    async (type) => {
      const onMoveToStep = vi.fn();
      const onMoveToStepWithOptions = vi.fn();
      renderMenu(type, onMoveToStep, onMoveToStepWithOptions, vi.fn());

      const step = await openStep();
      vi.spyOn(step, "getBoundingClientRect").mockReturnValue({
        x: 0,
        y: 0,
        width: 100,
        height: 44,
        top: 0,
        right: 100,
        bottom: 44,
        left: 0,
        toJSON: () => ({}),
      });
      fireEvent.pointerDown(step, { pointerType: "touch", clientX: 96 });
      fireEvent.click(step, { clientX: 96 });

      expect(await screen.findByTestId("task-context-options-step-step-2")).toBeTruthy();
      expect(onMoveToStep).not.toHaveBeenCalled();
    },
  );

  it.each(["context", "dropdown"] as const)(
    "opens and selects the nested options leaf for %s menus",
    async (type) => {
      const onMoveToStep = vi.fn();
      const onMoveToStepWithOptions = vi.fn();
      renderMenu(type, onMoveToStep, onMoveToStepWithOptions, vi.fn());

      const step = await openStep();
      const options = await screen.findByTestId("task-context-options-step-step-2");
      expect(step.getAttribute("aria-haspopup")).toBe("menu");
      fireEvent.click(options);

      expect(onMoveToStepWithOptions).toHaveBeenCalledOnce();
      expect(onMoveToStepWithOptions.mock.calls[0]?.[0]).toBe("step-2");
      expect(onMoveToStepWithOptions.mock.calls[0]?.[1]).toBeInstanceOf(HTMLElement);
      expect(onMoveToStep).not.toHaveBeenCalled();
    },
  );
});

describe("buildKanbanCardMenuEntries — !onEdit does not disable plugin edit actions", () => {
  const PLUGIN_ID = "kandev-plugin-notes";

  afterEach(() => {
    pluginRegistry.unregisterPlugin(PLUGIN_ID);
  });

  it("disables Edit task but leaves a visible plugin action enabled when onEdit is absent", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
      id: "enhance",
      label: "Enhance notes",
      group: "edit",
      run: vi.fn(),
    });

    const entries = buildKanbanCardMenuEntries({
      workflows: [],
      stepsByWorkflowId: {},
      // onEdit intentionally omitted — a card with no edit handler wired up.
    });

    const editMenu = entries.find((entry) => entry.key === "edit");
    expect(editMenu?.kind).toBe("submenu");
    if (editMenu?.kind !== "submenu") return;

    const editTask = editMenu.children.find(
      (child) => child.kind === "item" && child.key === "edit-task",
    );
    const pluginAction = editMenu.children.find(
      (child) => child.kind === "item" && child.key === `plugin-edit-${PLUGIN_ID}-enhance`,
    );

    expect(editTask?.kind === "item" && editTask.disabled).toBe(true);
    expect(pluginAction?.kind === "item" && pluginAction.disabled).toBeFalsy();
  });
});

describe("buildKanbanCardMenuEntries — 'primary' group plugin actions", () => {
  const PLUGIN_ID = "kandev-plugin-tags";

  afterEach(() => {
    pluginRegistry.unregisterPlugin(PLUGIN_ID);
  });

  function entryKeys(entries: KanbanCardMenuEntry[]) {
    return entries.map((entry) => entry.key);
  }

  it("renders a 'primary' group action as a flat item between Send to workflow and Link", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
      id: "quick-tag",
      label: "Quick tag",
      icon: PluginBitbucketIcon,
      group: "primary",
      run: vi.fn(),
    });

    const entries = buildKanbanCardMenuEntries({
      currentWorkflowId: "wf-1",
      workflows: [
        { id: "wf-1", name: "Workflow 1" },
        { id: "wf-2", name: "Workflow 2" },
      ],
      stepsByWorkflowId: {
        "wf-1": [
          { id: "s1", title: "Step 1" },
          { id: "s2", title: "Step 2" },
        ],
        "wf-2": [{ id: "s3", title: "Step 3" }],
      },
      onSendToWorkflow: vi.fn(),
      onLinkPullRequest: vi.fn(),
    });

    const keys = entryKeys(entries);
    const sendToIndex = keys.indexOf("send-to-workflow");
    const primaryIndex = keys.indexOf(`plugin-primary-${PLUGIN_ID}-quick-tag`);
    const linkIndex = keys.indexOf("link");

    expect(sendToIndex).toBeGreaterThanOrEqual(0);
    expect(primaryIndex).toBeGreaterThanOrEqual(0);
    expect(linkIndex).toBeGreaterThanOrEqual(0);
    expect(sendToIndex).toBeLessThan(primaryIndex);
    expect(primaryIndex).toBeLessThan(linkIndex);

    const primaryEntry = entries[primaryIndex];
    expect(primaryEntry.kind).toBe("item");
    if (primaryEntry.kind === "item") {
      expect(primaryEntry.label).toBe("Quick tag");
      expect((primaryEntry.icon as { type?: unknown })?.type).toBe(PluginBitbucketIcon);
    }
  });

  it("does not add a 'primary' entry when visible(context) returns false", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
      id: "quick-tag",
      label: "Quick tag",
      group: "primary",
      visible: () => false,
      run: vi.fn(),
    });

    const entries = buildKanbanCardMenuEntries({ workflows: [], stepsByWorkflowId: {} });

    expect(entryKeys(entries)).not.toContain(`plugin-primary-${PLUGIN_ID}-quick-tag`);
  });

  it("leaves the 'edit' group submenu unaffected by 'primary' group registrations", () => {
    pluginRegistry.forPlugin(PLUGIN_ID).registerTaskMenuAction({
      id: "quick-tag",
      label: "Quick tag",
      group: "primary",
      run: vi.fn(),
    });

    const entries = buildKanbanCardMenuEntries({ workflows: [], stepsByWorkflowId: {} });
    const editMenu = entries.find((entry) => entry.key === "edit");

    expect(editMenu?.kind).toBe("item");
  });
});

describe("buildKanbanCardMenuEntries — detach", () => {
  const baseArgs = {
    workflows: [],
    stepsByWorkflowId: {},
  };

  it("offers detach for a child task and invokes the action", () => {
    const onDetach = vi.fn();
    const entries = buildKanbanCardMenuEntries({
      ...baseArgs,
      parentTaskId: "parent-1",
      onDetach,
    });
    const detach = entries.find((entry) => entry.kind === "item" && entry.key === "detach");

    expect(detach?.kind).toBe("item");
    if (detach?.kind === "item") detach.onSelect?.();
    expect(onDetach).toHaveBeenCalledOnce();
  });

  it("omits detach for a root task", () => {
    const entries = buildKanbanCardMenuEntries({
      ...baseArgs,
      onDetach: vi.fn(),
    });

    expect(entries.some((entry) => entry.key === "detach")).toBe(false);
  });
});
