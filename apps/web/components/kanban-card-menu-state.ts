import { useRef, useState } from "react";
import {
  buildKanbanCardMenuEntries,
  useKanbanCardMoveTargets,
} from "@/components/kanban-card-menu-items";
import { useTaskMoveOptions } from "@/components/task/task-move-context-menu";
import { useTaskPluginLinkActions } from "@/components/task/task-session-sidebar-link-actions";
import { useTaskWorkflowMove } from "@/hooks/use-task-workflow-move";
import { useTaskMultiSelectStore } from "@/hooks/use-task-multi-select";
import { useDetachTask } from "@/hooks/use-detach-task";
import type { KanbanExternalLinkAvailability } from "./kanban-external-link-availability";
import type { KanbanCardProps, KanbanPresentation, Task } from "./kanban-card";
import type { ExternalLinkProvider } from "@/components/task/task-external-link-dialog";
import type { PluginTaskMenuContext } from "@/lib/plugins/types";
import { usePluginRegistry } from "@/lib/plugins/registry";

function useKanbanCardMoveMenuActions({
  task,
  steps,
  isSelected,
  selectedIds,
  onMove,
}: Pick<KanbanCardProps, "task" | "steps" | "isSelected" | "selectedIds" | "onMove">) {
  const moveTargets = useKanbanCardMoveTargets(task.id, steps);
  const moveTasks = useTaskWorkflowMove();
  const moveOptions = useTaskMoveOptions({
    taskId: task.id,
    workflowId: moveTargets.currentWorkflowId,
    steps: moveTargets.currentWorkflowId
      ? moveTargets.stepsByWorkflowId[moveTargets.currentWorkflowId]
      : undefined,
  });
  const moveOptionsMenuBoundaryRef = useRef<HTMLDivElement | null>(null);
  const { sortByDisplayOrder, getWorkflowIdForTask } = useTaskMultiSelectStore();

  const runMoveTasks = (
    taskIds: string[],
    workflowId: string,
    stepId: string,
    destination: "step" | "workflow",
  ) => {
    void moveTasks(taskIds, workflowId, stepId, destination).catch(() => {
      // useTaskWorkflowMove already shows the failure toast.
    });
  };
  const moveToStepFromDropdown = (stepId: string) => {
    if (onMove) {
      onMove(task, stepId);
      return;
    }
    if (moveTargets.currentWorkflowId) {
      runMoveTasks([task.id], moveTargets.currentWorkflowId, stepId, "step");
    }
  };
  const selectedTaskIds = isSelected && selectedIds?.size ? [...selectedIds] : [task.id];
  const orderedSelectedIds = () => sortByDisplayOrder(selectedTaskIds);
  const isMixedWorkflowSelection =
    selectedTaskIds.length > 1 &&
    new Set(selectedTaskIds.map((id) => getWorkflowIdForTask(id))).size > 1;
  const moveSelectedToStep = (stepId: string) => {
    if (selectedTaskIds.length === 1 && selectedTaskIds[0] === task.id && onMove) {
      onMove(task, stepId);
      return;
    }
    if (!moveTargets.currentWorkflowId) return;
    runMoveTasks(orderedSelectedIds(), moveTargets.currentWorkflowId, stepId, "step");
  };

  return {
    moveTargets,
    moveOptionsStep: moveOptions.moveOptionsStep,
    moveOptionsAnchorRef: moveOptions.moveOptionsAnchorRef,
    moveOptionsMenuBoundaryRef,
    isMovingWithOptions: moveOptions.isMoving,
    closeMoveOptions: moveOptions.closeMoveOptions,
    submitMoveOptions: moveOptions.submitMoveOptions,
    openMoveOptions: moveOptions.openMoveOptions,
    moveToStepFromDropdown,
    moveSelectedToStep: isMixedWorkflowSelection ? undefined : moveSelectedToStep,
    sendTaskToWorkflow: (workflowId: string, stepId: string) => {
      runMoveTasks([task.id], workflowId, stepId, "workflow");
    },
    sendSelectionToWorkflow: (workflowId: string, stepId: string) => {
      runMoveTasks(orderedSelectedIds(), workflowId, stepId, "workflow");
    },
  };
}

function externalLinkHandlers(
  availability: KanbanExternalLinkAvailability,
  setExternalLinkProvider: (provider: ExternalLinkProvider) => void,
) {
  return {
    onLinkJiraTicket: availability.jira ? () => setExternalLinkProvider("jira") : undefined,
    onLinkLinearIssue: availability.linear ? () => setExternalLinkProvider("linear") : undefined,
    onLinkSentryIssue: availability.sentry ? () => setExternalLinkProvider("sentry") : undefined,
  };
}

/** Link-dialog openers shared by both the dropdown and context menu builds. */
function buildLinkDialogHandlers(
  externalLinkAvailability: KanbanExternalLinkAvailability,
  dialogs: ReturnType<typeof useKanbanCardDialogState>,
) {
  return {
    onLinkPullRequest: () => dialogs.setShowPRDialog(true),
    onLinkIssue: () => dialogs.setShowIssueDialog(true),
    onLinkMergeRequest: externalLinkAvailability.gitlab
      ? () => dialogs.setShowMRDialog(true)
      : undefined,
    ...externalLinkHandlers(externalLinkAvailability, dialogs.setExternalLinkProvider),
  };
}

export function buildPluginMenuContext(
  task: Task,
  workspaceId: string | null,
  presentation: KanbanPresentation,
): PluginTaskMenuContext {
  return {
    workspaceId: workspaceId ?? "",
    taskId: task.id,
    taskTitle: task.title,
    workflowStepId: task.workflowStepId ?? null,
    presentation,
  };
}

/** Every confirm/link-dialog open flag the card menus and their dialogs share. */
function useKanbanCardDialogState() {
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showArchiveConfirm, setShowArchiveConfirm] = useState(false);
  const [showDetachConfirm, setShowDetachConfirm] = useState(false);
  const [showPRDialog, setShowPRDialog] = useState(false);
  const [showIssueDialog, setShowIssueDialog] = useState(false);
  const [showMRDialog, setShowMRDialog] = useState(false);
  const [externalLinkProvider, setExternalLinkProvider] = useState<ExternalLinkProvider | null>(
    null,
  );
  return {
    showDeleteConfirm,
    setShowDeleteConfirm,
    showArchiveConfirm,
    setShowArchiveConfirm,
    showDetachConfirm,
    setShowDetachConfirm,
    showPRDialog,
    setShowPRDialog,
    showIssueDialog,
    setShowIssueDialog,
    showMRDialog,
    setShowMRDialog,
    externalLinkProvider,
    setExternalLinkProvider,
  };
}

function requestConfirmation(setOpen: (open: boolean) => void) {
  // Let Radix finish the menu's pointer sequence before the local surface opens.
  window.setTimeout(() => setOpen(true), 300);
}

export function useKanbanCardMenus({
  task,
  workspaceId,
  presentation = "desktop",
  steps,
  isDeleting,
  isArchiving,
  isSelected,
  selectedIds,
  onEdit,
  onDelete,
  onArchive,
  onMove,
  externalLinkAvailability,
}: Pick<
  KanbanCardProps,
  | "task"
  | "workspaceId"
  | "presentation"
  | "externalLinkAvailability"
  | "steps"
  | "isDeleting"
  | "isArchiving"
  | "isSelected"
  | "selectedIds"
  | "onEdit"
  | "onDelete"
  | "onArchive"
  | "onMove"
>) {
  const pluginLinkActions = useTaskPluginLinkActions(task.id, task.repositories ?? []);
  // Plugins load asynchronously and can be disabled/uninstalled at runtime;
  // re-render on any registry change so a menu action a plugin registers
  // after this card already mounted still appears, and one whose plugin was
  // just disabled doesn't linger as a stale entry.
  usePluginRegistry();
  const moveMenu = useKanbanCardMoveMenuActions({ task, steps, isSelected, selectedIds, onMove });
  const dialogs = useKanbanCardDialogState();
  const { detachTask, detachingTaskId } = useDetachTask();
  const detachAnchorRef = useRef<HTMLDivElement>(null);
  const detachFocusReturnRef = useRef<HTMLButtonElement>(null);
  const isDetaching = detachingTaskId === task.id;
  const disabled = Boolean(isDeleting || isArchiving || isDetaching);
  const actingOnMultiSelection = Boolean(isSelected && selectedIds && selectedIds.size > 1);

  const handleDetachConfirm = async () => {
    try {
      await detachTask(task.id);
      dialogs.setShowDetachConfirm(false);
    } catch (error) {
      console.error("Failed to detach task:", error);
    }
  };

  const requestDetachConfirmation = () => requestConfirmation(dialogs.setShowDetachConfirm);
  const requestArchiveConfirmation = () => requestConfirmation(dialogs.setShowArchiveConfirm);

  const menuBase = {
    currentWorkflowId: moveMenu.moveTargets.currentWorkflowId,
    currentStepId: task.workflowStepId,
    workflows: moveMenu.moveTargets.workflowItems,
    stepsByWorkflowId: moveMenu.moveTargets.stepsByWorkflowId,
    disabled,
    isDeleting,
    isArchiving,
    isDetaching,
    parentTaskId: task.parentTaskId,
    onEdit: onEdit ? () => onEdit(task) : undefined,
    onArchive: onArchive ? requestArchiveConfirmation : undefined,
    onDelete: onDelete ? () => dialogs.setShowDeleteConfirm(true) : undefined,
    onDetach: task.parentTaskId && !actingOnMultiSelection ? requestDetachConfirmation : undefined,
    ...buildLinkDialogHandlers(externalLinkAvailability, dialogs),
    pluginLinkActions,
  };

  const pluginMenuContext = buildPluginMenuContext(task, workspaceId, presentation);
  const optionsHandler = actingOnMultiSelection ? undefined : moveMenu.openMoveOptions;

  return {
    ...dialogs,
    moveOptionsStep: moveMenu.moveOptionsStep,
    moveOptionsAnchorRef: moveMenu.moveOptionsAnchorRef,
    moveOptionsMenuBoundaryRef: moveMenu.moveOptionsMenuBoundaryRef,
    isMovingWithOptions: moveMenu.isMovingWithOptions,
    closeMoveOptions: moveMenu.closeMoveOptions,
    submitMoveOptions: moveMenu.submitMoveOptions,
    dropdownMenuEntries: buildKanbanCardMenuEntries({
      ...menuBase,
      onMoveToStep: moveMenu.moveToStepFromDropdown,
      onMoveToStepWithOptions: optionsHandler,
      onSendToWorkflow: moveMenu.sendTaskToWorkflow,
      pluginMenuContext,
    }),
    contextMenuEntries: buildKanbanCardMenuEntries({
      ...menuBase,
      onMoveToStep: moveMenu.moveSelectedToStep,
      onMoveToStepWithOptions: optionsHandler,
      onSendToWorkflow: moveMenu.sendSelectionToWorkflow,
      pluginMenuContext,
    }),
    isDetaching,
    detachAnchorRef,
    detachFocusReturnRef,
    archiveAnchorRef: detachFocusReturnRef,
    archiveFocusReturnRef: detachFocusReturnRef,
    handleDetachConfirm,
  };
}

export type KanbanCardMenuState = ReturnType<typeof useKanbanCardMenus>;
