"use client";

import { createContext, useContext, memo, useCallback, useState } from "react";
import { IconArchiveOff } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { unarchiveTask } from "@/lib/api";
import { useToast } from "@/components/toast-provider";

type TaskArchivedState = {
  isArchived: boolean;
  archivedTaskId?: string;
  archivedTaskTitle?: string;
  archivedTaskRepositoryPath?: string;
  archivedTaskUpdatedAt?: string;
};

const TaskArchivedContext = createContext<TaskArchivedState>({ isArchived: false });

export const TaskArchivedProvider = TaskArchivedContext.Provider;

export function useIsTaskArchived() {
  return useContext(TaskArchivedContext).isArchived;
}

export function useArchivedTaskState() {
  return useContext(TaskArchivedContext);
}

export const ArchivedPanelPlaceholder = memo(function ArchivedPanelPlaceholder({
  message = "Workspace not available — this task is archived",
}: {
  message?: string;
}) {
  const { archivedTaskId } = useArchivedTaskState();
  const { toast } = useToast();
  const [loading, setLoading] = useState(false);

  const handleUnarchive = useCallback(async () => {
    if (!archivedTaskId) return;
    setLoading(true);
    try {
      await unarchiveTask(archivedTaskId);
      toast({ title: "Task unarchived", description: "The task has been restored to the board." });
      window.location.reload();
    } catch (err) {
      toast({
        title: "Failed to unarchive task",
        description: err instanceof Error ? err.message : "Unknown error",
        variant: "error",
      });
    } finally {
      setLoading(false);
    }
  }, [archivedTaskId, toast]);

  return (
    <PanelRoot>
      <PanelBody>
        <div className="flex flex-col items-center justify-center gap-2 h-full text-muted-foreground text-xs">
          {message}
          {archivedTaskId && (
            <Button
              variant="outline"
              size="sm"
              className="cursor-pointer gap-1.5"
              disabled={loading}
              onClick={handleUnarchive}
            >
              <IconArchiveOff className="h-3.5 w-3.5" />
              Unarchive
            </Button>
          )}
        </div>
      </PanelBody>
    </PanelRoot>
  );
});
