"use client";

import { useCallback } from "react";
import { IconGitPullRequest } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { useAppStore } from "@/components/state-provider";
import { cn } from "@/lib/utils";
import { replaceTaskUrl } from "@/lib/links";
import { aggregatePRStatusColor } from "../pr-task-icon";
import type { TaskPR } from "@/lib/types/github";

type PRRowTaskIndicatorProps = {
  tasks: TaskPR[] | undefined;
};

export function PRRowTaskIndicator({ tasks }: PRRowTaskIndicatorProps) {
  const setActiveTask = useAppStore((state) => state.setActiveTask);

  const handleClick = useCallback(
    (taskId: string) => {
      setActiveTask(taskId);
      replaceTaskUrl(taskId);
    },
    [setActiveTask],
  );

  if (!tasks || tasks.length === 0) {
    return (
      <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
        No task created yet
      </span>
    );
  }

  const color = aggregatePRStatusColor(tasks);
  const primary = tasks[0];
  const truncated =
    primary.pr_title.length > 40 ? primary.pr_title.slice(0, 40) + "…" : primary.pr_title;

  return (
    <button
      type="button"
      onClick={() => handleClick(primary.task_id)}
      className={cn(
        "inline-flex items-center gap-1.5 rounded px-1.5 py-0.5 text-xs",
        "hover:bg-muted/70 transition-colors cursor-pointer w-fit max-w-full",
      )}
    >
      <span className={cn("inline-flex items-center shrink-0", color)}>
        <IconGitPullRequest className="h-3 w-3" />
      </span>
      <span className="truncate text-foreground/80">{truncated}</span>
      {tasks.length > 1 && (
        <Badge variant="outline" className="h-4 px-1 py-0 text-[10px] shrink-0">
          {tasks.length}
        </Badge>
      )}
    </button>
  );
}
