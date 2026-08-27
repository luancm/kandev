"use client";

import { forwardRef, type ComponentPropsWithoutRef } from "react";
import { cn } from "@kandev/ui/lib/utils";

type WorkflowStepTriggerProps = ComponentPropsWithoutRef<"button"> & {
  stepName: string;
  isCurrent: boolean;
  isCompleted: boolean;
  isTouchSurface?: boolean;
};

/** Step trigger. Forwards trigger props (and the ref) injected by PopoverTrigger/DrawerTrigger with asChild. */
const WorkflowStepTrigger = forwardRef<HTMLButtonElement, WorkflowStepTriggerProps>(
  function WorkflowStepTrigger(
    { stepName, isCurrent, isCompleted, isTouchSurface = false, ...triggerProps },
    ref,
  ) {
    return (
      <button
        type="button"
        ref={ref}
        {...triggerProps}
        data-testid={`workflow-step-${stepName}`}
        aria-current={isCurrent ? "step" : undefined}
        className={cn(
          "flex items-center gap-1.5 rounded-md px-2 py-0.5 text-xs whitespace-nowrap transition-colors cursor-pointer",
          isTouchSurface && "min-h-11 min-w-11",
          isCurrent ? "bg-muted/40" : "hover:bg-muted/30",
        )}
      >
        <StepCircleIndicator isCurrent={isCurrent} isCompleted={isCompleted} />
        <span className={cn("text-xs leading-none", getStepLabelClass(isCurrent, isCompleted))}>
          {stepName}
        </span>
      </button>
    );
  },
);

/** Connector line between steps */
function StepConnector({ isActive }: { isActive: boolean }) {
  return (
    <div className={cn("h-px w-6 shrink-0", isActive ? "bg-muted-foreground/40" : "bg-border")} />
  );
}

/** Circle indicator for step state */
function StepCircleIndicator({
  isCurrent,
  isCompleted,
}: {
  isCurrent: boolean;
  isCompleted: boolean;
}) {
  if (isCurrent) {
    return (
      <span className="relative flex items-center justify-center shrink-0">
        <span className="absolute h-3.5 w-3.5 rounded-full border-2 border-primary/40" />
        <span className="h-2 w-2 rounded-full bg-primary" />
      </span>
    );
  }
  if (isCompleted) {
    return (
      <span className="relative flex items-center justify-center shrink-0">
        <span className="h-2 w-2 rounded-full bg-muted-foreground/60" />
      </span>
    );
  }
  return (
    <span className="relative flex items-center justify-center shrink-0">
      <span className="h-2 w-2 rounded-full border border-muted-foreground/40" />
    </span>
  );
}

/** Get CSS class for step label based on state */
function getStepLabelClass(isCurrent: boolean, isCompleted: boolean): string {
  if (isCurrent) return "text-foreground font-medium";
  if (isCompleted) return "text-muted-foreground";
  return "text-muted-foreground/60";
}

export { StepCircleIndicator, StepConnector, WorkflowStepTrigger, getStepLabelClass };
