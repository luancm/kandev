"use client";

import { memo, useCallback, useMemo, useRef, useState } from "react";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { cn } from "@kandev/ui/lib/utils";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@kandev/ui/drawer";
import { IconArrowRight, IconChevronDown } from "@tabler/icons-react";
import { useWorkflowMove } from "@/hooks/domains/kanban/use-workflow-move";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import type { WorkflowMoveEntryOptions } from "@/lib/api/domains/kanban-api";
import {
  useWorkflowMoveOptionsForm,
  WorkflowMoveOptionsFields,
  workflowMoveOptionsPayload,
  type WorkflowMoveOptionsDraft,
} from "./workflow-move-options";
import { AnchoredActionPopover } from "@/components/confirmation/anchored-action-popover";
import {
  useHoverIntentAffordance,
  HOVER_INTENT_OPEN_DELAY_MS,
} from "./use-hover-intent-affordance";
import {
  StepCircleIndicator,
  StepConnector,
  WorkflowStepTrigger,
} from "./workflow-step-primitives";
import { StepCapabilityIcons } from "@/components/step-capability-icons";
import { useAppStore } from "@/components/state-provider";
import { useContextFilesStore } from "@/lib/state/context-files-store";
import { useLayoutStore } from "@/lib/state/layout-store";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useToolbarCollapsed } from "@/hooks/use-toolbar-collapsed";
import type { KanbanStepEvents } from "@/lib/state/slices/kanban/types";
import { useTranslation } from "react-i18next";
import { useToast } from "@/components/toast-provider";

type Step = {
  id: string;
  name: string;
  color: string;
  position: number;
  events?: KanbanStepEvents;
  allow_manual_move?: boolean;
  prompt?: string;
  is_start_step?: boolean;
  agent_profile_id?: string;
};

const PLAN_CONTEXT_PATH = "plan:context";

/** Returns a callback that disables plan mode for the active session of a task. */
function useDisablePlanMode() {
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const planModeEnabled = useAppStore((s) =>
    activeSessionId ? (s.chatInput.planModeBySessionId[activeSessionId] ?? false) : false,
  );
  const setPlanMode = useAppStore((s) => s.setPlanMode);
  const setActiveDocument = useAppStore((s) => s.setActiveDocument);
  const closeDocument = useLayoutStore((s) => s.closeDocument);
  const removeContextFile = useContextFilesStore((s) => s.removeFile);
  const applyBuiltInPreset = useDockviewStore((s) => s.applyBuiltInPreset);

  return useCallback(() => {
    if (!activeSessionId || !planModeEnabled) return;
    applyBuiltInPreset("default");
    closeDocument(activeSessionId);
    setActiveDocument(activeSessionId, null);
    setPlanMode(activeSessionId, false);
    removeContextFile(activeSessionId, PLAN_CONTEXT_PATH);
  }, [
    activeSessionId,
    planModeEnabled,
    setPlanMode,
    setActiveDocument,
    closeDocument,
    removeContextFile,
    applyBuiltInPreset,
  ]);
}

type WorkflowStepperProps = {
  steps: Step[];
  currentStepId: string | null;
  taskId?: string | null;
  workflowId?: string | null;
  isArchived?: boolean;
};

const WorkflowStepper = memo(function WorkflowStepper({
  steps,
  currentStepId,
  taskId,
  workflowId,
  isArchived,
}: WorkflowStepperProps) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const [movingToStepId, setMovingToStepId] = useState<string | null>(null);
  const disablePlanMode = useDisablePlanMode();
  const { move } = useWorkflowMove();

  const notifyMoveFailure = useCallback(
    (error: unknown) => {
      toast({
        title: t("task:failedToMoveTask"),
        description: error instanceof Error ? error.message : t("task:failedToMoveTask"),
        variant: "error",
      });
    },
    [t, toast],
  );

  const sortedSteps = useMemo(() => [...steps].sort((a, b) => a.position - b.position), [steps]);

  const currentIndex = useMemo(
    () => sortedSteps.findIndex((s) => s.id === currentStepId),
    [sortedSteps, currentStepId],
  );

  const handleMove = useCallback(
    async (stepId: string, entryOptions?: WorkflowMoveEntryOptions): Promise<boolean> => {
      if (!taskId || !workflowId) return false;
      setMovingToStepId(stepId);
      try {
        const result = await move(taskId, {
          workflow_id: workflowId,
          workflow_step_id: stepId,
          position: 0,
          entry_options: entryOptions,
        });
        if (result.disposition === "failed") {
          notifyMoveFailure(result.error);
          return false;
        }
        disablePlanMode();
        return true;
      } catch (error) {
        notifyMoveFailure(error);
        return false;
      } finally {
        setMovingToStepId(null);
      }
    },
    [taskId, workflowId, disablePlanMode, move, notifyMoveFailure],
  );

  // Collapse to a minimal view when the full stepper can't fit (w-full keeps the measurement track-driven).
  const containerRef = useRef<HTMLDivElement>(null);
  const isCollapsed = useToolbarCollapsed(containerRef);

  if (sortedSteps.length === 0) return null;

  return (
    <div
      ref={containerRef}
      data-testid="workflow-stepper"
      className="flex w-full min-w-0 items-center justify-center gap-0 overflow-hidden"
    >
      {isCollapsed ? (
        <MinimalWorkflowStepper
          sortedSteps={sortedSteps}
          currentIndex={currentIndex}
          isArchived={isArchived}
        />
      ) : (
        <>
          <div className="flex items-center gap-0">
            {sortedSteps.map((step, index) => (
              <WorkflowStepItem
                key={step.id}
                step={step}
                index={index}
                currentIndex={currentIndex}
                isArchived={isArchived}
                taskId={taskId}
                workflowId={workflowId}
                movingToStepId={movingToStepId}
                onMove={handleMove}
              />
            ))}
          </div>
          {isArchived && (
            <>
              <div className="h-px w-6 shrink-0 bg-border" />
              <span className="text-[11px] font-medium text-amber-500 bg-amber-500/15 px-2 py-0.5 rounded-md whitespace-nowrap">
                {t("task:filterDimensionArchived")}
              </span>
            </>
          )}
        </>
      )}
    </div>
  );
});

/** Minimal stepper: current step only (or archived badge), keeping the per-step test id + aria-current. */
function MinimalWorkflowStepper({
  sortedSteps,
  currentIndex,
  isArchived,
}: {
  sortedSteps: Step[];
  currentIndex: number;
  isArchived?: boolean;
}) {
  const { t } = useTranslation();
  if (isArchived) {
    return (
      <span
        data-testid="workflow-stepper-minimal"
        className="text-[11px] font-medium text-amber-500 bg-amber-500/15 px-2 py-0.5 rounded-md whitespace-nowrap"
      >
        {t("task:filterDimensionArchived")}
      </span>
    );
  }

  const current = currentIndex >= 0 ? sortedSteps[currentIndex] : sortedSteps[0];
  if (!current) return null;

  return (
    <div
      data-testid="workflow-stepper-minimal"
      className="flex min-w-0 items-center gap-1.5 rounded-md px-2 py-0.5"
    >
      <div
        data-testid={`workflow-step-${current.name}`}
        aria-current={currentIndex >= 0 ? "step" : undefined}
        className="flex min-w-0 items-center gap-1.5 text-xs"
      >
        <StepCircleIndicator isCurrent isCompleted={false} />
        <span className="truncate text-xs font-medium leading-none text-foreground">
          {current.name}
        </span>
      </div>
      {sortedSteps.length > 1 && (
        <span className="shrink-0 text-[11px] tabular-nums leading-none text-muted-foreground">
          {(currentIndex >= 0 ? currentIndex : 0) + 1}/{sortedSteps.length}
        </span>
      )}
    </div>
  );
}

/** Check if a step can be moved to */
function canMoveToStep(params: {
  isArchived: boolean | undefined;
  isCurrent: boolean;
  taskId: string | null | undefined;
  workflowId: string | null | undefined;
  isAdjacent: boolean;
  allowManualMove: boolean | undefined;
}): boolean {
  if (params.isArchived || params.isCurrent || !params.taskId || !params.workflowId) return false;
  return params.isAdjacent || !!params.allowManualMove;
}

function WorkflowStepSurface({
  step,
  index,
  isCurrent,
  isCompleted,
  usesTouchDrawer,
  touchDrawerOpen,
  onTouchDrawerOpenChange,
  affordance,
  children,
}: {
  step: Step;
  index: number;
  isCurrent: boolean;
  isCompleted: boolean;
  usesTouchDrawer: boolean;
  touchDrawerOpen: boolean;
  onTouchDrawerOpenChange: (open: boolean) => void;
  affordance: ReturnType<typeof useHoverIntentAffordance>;
  children: React.ReactNode;
}) {
  return (
    <div className="flex items-center">
      {index > 0 && <StepConnector isActive={isCompleted || isCurrent} />}
      {usesTouchDrawer ? (
        <Drawer open={touchDrawerOpen} onOpenChange={onTouchDrawerOpenChange}>
          <DrawerTrigger asChild>
            <WorkflowStepTrigger
              stepName={step.name}
              isCurrent={isCurrent}
              isCompleted={isCompleted}
              isTouchSurface
            />
          </DrawerTrigger>
          <DrawerContent className="!top-0 !bottom-auto !mt-0 !h-dvh !max-h-dvh min-h-0 overflow-hidden pb-[env(safe-area-inset-bottom,0px)]">
            <DrawerHeader className="shrink-0 text-left">
              <DrawerTitle>{step.name}</DrawerTitle>
              <DrawerDescription className="sr-only">{step.name}</DrawerDescription>
            </DrawerHeader>
            <div
              className="min-h-0 flex-1 overflow-y-auto px-4 pb-5"
              data-vaul-no-drag
              data-testid="workflow-step-drawer-body"
            >
              {children}
            </div>
          </DrawerContent>
        </Drawer>
      ) : (
        <StepPopover
          affordance={affordance}
          step={step}
          isCurrent={isCurrent}
          isCompleted={isCompleted}
        >
          {children}
        </StepPopover>
      )}
    </div>
  );
}

/** Individual step in the workflow stepper */
function WorkflowStepItem({
  step,
  index,
  currentIndex,
  isArchived,
  taskId,
  workflowId,
  movingToStepId,
  onMove,
}: {
  step: Step;
  index: number;
  currentIndex: number;
  isArchived?: boolean;
  taskId?: string | null;
  workflowId?: string | null;
  movingToStepId: string | null;
  onMove: (stepId: string, entryOptions?: WorkflowMoveEntryOptions) => Promise<boolean>;
}) {
  const isCompleted = !isArchived && currentIndex >= 0 && index < currentIndex;
  const isCurrent = !isArchived && index === currentIndex;
  const isAdjacent =
    currentIndex >= 0 && (index === currentIndex - 1 || index === currentIndex + 1);
  const canMove = canMoveToStep({
    isArchived,
    isCurrent,
    taskId,
    workflowId,
    isAdjacent,
    allowManualMove: step.allow_manual_move,
  });

  const usesTouchDrawer = useTouchDrawer();
  const affordance = useHoverIntentAffordance({ openDelayMs: HOVER_INTENT_OPEN_DELAY_MS });
  const [touchDrawerOpen, setTouchDrawerOpen] = useState(false);

  const closeSurface = useCallback(() => {
    affordance.requestClose();
    setTouchDrawerOpen(false);
  }, [affordance]);

  const stepActions = (
    <StepHoverContent
      step={step}
      isCurrent={isCurrent}
      canMove={canMove}
      isMoving={movingToStepId === step.id}
      onMove={onMove}
      onSurfaceClose={closeSurface}
    />
  );

  return (
    <WorkflowStepSurface
      step={step}
      index={index}
      isCurrent={isCurrent}
      isCompleted={isCompleted}
      usesTouchDrawer={usesTouchDrawer}
      touchDrawerOpen={touchDrawerOpen}
      onTouchDrawerOpenChange={setTouchDrawerOpen}
      affordance={affordance}
    >
      {stepActions}
    </WorkflowStepSurface>
  );
}

/** Fine-pointer surface: hover/focus opens the compact step popover. */
function StepPopover({
  affordance,
  step,
  isCurrent,
  isCompleted,
  children,
}: {
  affordance: ReturnType<typeof useHoverIntentAffordance>;
  step: Step;
  isCurrent: boolean;
  isCompleted: boolean;
  children: React.ReactNode;
}) {
  const anchorRef = useRef<HTMLButtonElement | null>(null);
  return (
    <>
      <WorkflowStepTrigger
        ref={anchorRef}
        stepName={step.name}
        isCurrent={isCurrent}
        isCompleted={isCompleted}
        onPointerEnter={affordance.triggerProps.onPointerEnter}
        onPointerLeave={affordance.triggerProps.onPointerLeave}
        onFocus={affordance.triggerProps.onFocus}
      />
      <AnchoredActionPopover
        open={affordance.open}
        anchorRef={anchorRef}
        contentRef={affordance.contentRef}
        focusReturnRef={anchorRef}
        compact
        title={step.name}
        body={children}
        widthClassName="w-auto min-w-28 max-w-[min(24rem,calc(100vw-1rem))]"
        testId="workflow-step-popover"
        interactionProps={{
          onPointerEnter: affordance.contentProps.onPointerEnter,
          onPointerLeave: affordance.contentProps.onPointerLeave,
          onPointerDownCapture: affordance.contentProps.onPointerDownCapture,
          onFocusCapture: affordance.contentProps.onFocusCapture,
        }}
        onOpenChange={(open) => {
          if (!open) affordance.requestClose(true);
        }}
      />
    </>
  );
}

/** Step popover/drawer body: the one-shot options, a direct move, status info. */
function StepHoverContent({
  step,
  isCurrent,
  canMove,
  isMoving,
  onMove,
  onSurfaceClose,
}: {
  step: Step;
  isCurrent: boolean;
  canMove: boolean;
  isMoving: boolean;
  onMove: (stepId: string, entryOptions?: WorkflowMoveEntryOptions) => Promise<boolean>;
  onSurfaceClose: () => void;
}) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const form = useWorkflowMoveOptionsForm();
  const [optionsOpen, setOptionsOpen] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const busy = isMoving || submitting;
  // Untouched options normalize to undefined, so the same button stays the
  // direct move; anything the user filled becomes this entry's options.
  const entryOptions = workflowMoveOptionsPayload(form.draft);

  const submitMove = async () => {
    if (busy) return;
    setSubmitting(true);
    try {
      const moved = await onMove(step.id, entryOptions);
      if (moved) {
        form.resetDraft();
        onSurfaceClose();
      }
    } finally {
      setSubmitting(false);
    }
  };

  // Desktop hover card keeps the compact h-6 action; the coarse-pointer Drawer body keeps 44px targets.
  const actionClass = usesTouchDrawer
    ? "min-h-11 cursor-pointer text-xs px-2.5 rounded-sm"
    : "cursor-pointer text-xs h-6 px-2.5 rounded-sm";

  return (
    <div
      className={cn(
        "flex w-auto min-w-28 flex-col items-center gap-1.5",
        usesTouchDrawer ? "p-1.5" : "p-0",
      )}
    >
      {canMove && (
        <>
          <Button
            size="sm"
            variant="default"
            className={actionClass}
            disabled={busy}
            onClick={submitMove}
            data-testid="workflow-step-move-here"
          >
            <IconArrowRight className="h-3 w-3" />
            {busy ? t("task:moving") : t("task:moveHere")}
          </Button>
          <WorkflowMoveOptionsDisclosure
            stepName={step.name}
            open={optionsOpen}
            onOpenChange={setOptionsOpen}
            usesTouchDrawer={usesTouchDrawer}
            draft={form.draft}
            onDraftChange={form.patchDraft}
            profileOptions={form.profileOptions}
          />
        </>
      )}
      {isCurrent && (
        <div className="text-[11px] text-muted-foreground">{t("task:currentStep")}</div>
      )}
      <StepCapabilityIcons events={step.events} agentProfileId={step.agent_profile_id} />
    </div>
  );
}

function WorkflowMoveOptionsDisclosure({
  stepName,
  open,
  onOpenChange,
  usesTouchDrawer,
  draft,
  onDraftChange,
  profileOptions,
}: {
  stepName: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  usesTouchDrawer: boolean;
  draft: WorkflowMoveOptionsDraft;
  onDraftChange: (patch: Partial<WorkflowMoveOptionsDraft>) => void;
  profileOptions: ReturnType<typeof useWorkflowMoveOptionsForm>["profileOptions"];
}) {
  const { t } = useTranslation();
  return (
    <Collapsible
      open={open}
      onOpenChange={onOpenChange}
      className="w-full"
      data-testid="workflow-step-move-options"
    >
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className={cn(
            "flex w-full cursor-pointer items-center justify-center gap-1 text-xs text-muted-foreground hover:text-foreground",
            usesTouchDrawer ? "min-h-11" : "h-6",
          )}
          aria-label={t("task:workflowMoveOptionsForStep", { step: stepName })}
          data-testid="workflow-step-move-options-trigger"
        >
          <span>{t("task:workflowMoveOptions")}</span>
          <IconChevronDown
            className={cn("h-3.5 w-3.5 transition-transform", open && "rotate-180")}
            aria-hidden="true"
          />
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent className="pt-1.5" data-testid="workflow-step-move-options-content">
        <div className="w-full min-w-0 text-left">
          <WorkflowMoveOptionsFields
            draft={draft}
            onDraftChange={onDraftChange}
            profileOptions={profileOptions}
            isTouchSurface={usesTouchDrawer}
            instructionsRows={3}
          />
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

export { WorkflowStepper };
export type { Step as WorkflowStepperStep };
