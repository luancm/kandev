"use client";

import { useState } from "react";
import { IconChevronLeft, IconFolder, IconGitBranch } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { useNow } from "@/hooks/use-now";
import { isLiveErrand, type Errand } from "@/lib/types/errands";
import { cn } from "@/lib/utils";
import { errandElapsedLabel } from "./errand-display";
import { ERRAND_TOUCH_TARGET } from "./errand-open-button";
import { ErrandProfileLabel } from "./errand-profile-label";
import { ErrandStatusLabel } from "./errand-status";
import { useOpenTaskSession } from "./use-open-task-session";
import type { ErrandViewModel } from "./use-errand-view-model";

const ELAPSED_TICK_MS = 30_000;

function Dot() {
  return (
    <span aria-hidden="true" className="shrink-0 text-muted-foreground/40">
      ·
    </span>
  );
}

function LiveElapsed({ errand }: { errand: Errand }) {
  const now = useNow(ELAPSED_TICK_MS);
  return <>{errandElapsedLabel(errand, now)}</>;
}

/** Status with its elapsed time; only a live errand ticks. */
function StatusWithElapsed({ errand }: { errand: Errand }) {
  return (
    <span className="inline-flex shrink-0 items-center gap-1.5">
      <ErrandStatusLabel status={errand.status} testId="errand-header-status" />
      <span className="tabular-nums text-muted-foreground" data-testid="errand-header-elapsed">
        {isLiveErrand(errand) ? (
          <LiveElapsed errand={errand} />
        ) : (
          errandElapsedLabel(errand, Date.now())
        )}
      </span>
    </span>
  );
}

/** Shared workspace path and branch; the path truncates from its start. */
export function ErrandWorkspaceLocation({
  path,
  branch,
  className,
}: {
  path: string | null;
  branch: string | null;
  className?: string;
}) {
  const { t } = useTranslation();
  if (!path) return null;
  const full = branch ? t("task:errandWorkspaceWithBranch", { path, branch }) : path;
  return (
    <span
      data-testid="errand-workspace"
      title={full}
      className={cn("inline-flex min-w-0 items-center gap-1 text-muted-foreground", className)}
    >
      <IconFolder aria-hidden className="h-3 w-3 shrink-0" />
      <span dir="rtl" className="min-w-0 truncate text-left font-mono text-[10px]">
        <bdi dir="ltr">{path}</bdi>
      </span>
      {branch && (
        <span className="inline-flex min-w-0 max-w-[40%] shrink-0 items-center gap-0.5">
          <IconGitBranch aria-hidden className="h-3 w-3 shrink-0" />
          <span className="truncate font-mono text-[10px]">{branch}</span>
        </span>
      )}
    </span>
  );
}

/** Profile, status, elapsed time, and optionally the shared workspace. */
export function ErrandMetaRow({
  model,
  showWorkspace,
}: {
  model: ErrandViewModel;
  showWorkspace: boolean;
}) {
  const { errand } = model;
  return (
    <div
      data-testid="errand-header-meta"
      className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5 text-[11px] text-muted-foreground"
    >
      {errand && <ErrandProfileLabel profileId={errand.agent_profile_id} className="max-w-[40%]" />}
      {errand && <Dot />}
      {errand && <StatusWithElapsed errand={errand} />}
      {showWorkspace && model.workspacePath && (
        <>
          <Dot />
          <ErrandWorkspaceLocation
            path={model.workspacePath}
            branch={model.branch}
            className="max-w-full flex-1"
          />
        </>
      )}
    </div>
  );
}

/** Back link to the parent session; opens it the same way Open opens an errand. */
export function BackToParentButton({
  model,
  variant,
}: {
  model: ErrandViewModel;
  variant: "desktop" | "phone";
}) {
  const { t } = useTranslation();
  const openSession = useOpenTaskSession();
  const { parentSessionId, parentTitle, taskId } = model;
  return (
    <Button
      type="button"
      variant="ghost"
      size="sm"
      disabled={!parentSessionId}
      aria-label={t("task:errandBackToParentAria", { parent: parentTitle })}
      data-testid="errand-back-to-parent"
      onClick={() => {
        if (parentSessionId) {
          openSession({ taskId, sessionId: parentSessionId, title: parentTitle });
        }
      }}
      className={cn(
        "min-w-0 shrink cursor-pointer gap-0.5 text-muted-foreground hover:text-foreground",
        variant === "phone" ? "h-11 max-w-[40%] px-2 text-xs" : "h-6 max-w-[35%] px-1.5 text-xs",
        variant === "desktop" && ERRAND_TOUCH_TARGET,
      )}
    >
      <IconChevronLeft aria-hidden className="h-3.5 w-3.5 shrink-0" />
      <span className="truncate">{parentTitle}</span>
    </Button>
  );
}

/** Objective preview that expands in place; the phone form also lists the workspace. */
export function ErrandObjectiveDisclosure({
  model,
  includeWorkspace,
}: {
  model: ErrandViewModel;
  includeWorkspace: boolean;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const { objective } = model;
  const hasWorkspace = includeWorkspace && Boolean(model.workspacePath);
  if (!objective) {
    return hasWorkspace ? (
      <ErrandWorkspaceLocation
        path={model.workspacePath}
        branch={model.branch}
        className="max-w-full"
      />
    ) : null;
  }
  return (
    <Collapsible open={open} onOpenChange={setOpen} data-testid="errand-objective">
      <div className="flex min-w-0 items-center gap-1.5 text-[11px]">
        <span className="shrink-0 text-muted-foreground">{t("task:errandObjectiveLabel")}</span>
        <span className="min-w-0 flex-1 truncate text-foreground/80" title={objective}>
          {open ? null : objective}
        </span>
        <CollapsibleTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            data-testid="errand-objective-toggle"
            className={cn("h-5 shrink-0 cursor-pointer px-1.5 text-[11px]", ERRAND_TOUCH_TARGET)}
          >
            {t(open ? "task:errandObjectiveLess" : "task:errandObjectiveMore")}
          </Button>
        </CollapsibleTrigger>
      </div>
      <CollapsibleContent className="space-y-1.5 pt-1">
        <p className="max-h-40 overflow-y-auto whitespace-pre-wrap break-words text-[11px] text-foreground/80 [overflow-wrap:anywhere]">
          {objective}
        </p>
        {hasWorkspace && (
          <ErrandWorkspaceLocation
            path={model.workspacePath}
            branch={model.branch}
            className="max-w-full"
          />
        )}
      </CollapsibleContent>
    </Collapsible>
  );
}
