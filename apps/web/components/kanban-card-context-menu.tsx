"use client";

import { ContextMenu, ContextMenuContent, ContextMenuTrigger } from "@kandev/ui/context-menu";
import type { RefObject } from "react";
import {
  KanbanCardContextMenuItems,
  type KanbanCardMenuEntry,
} from "@/components/kanban-card-menu-items";
import { isWorkflowMoveOptionsTarget } from "@/components/task/workflow-move-surface";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

export function KanbanCardContextMenu({
  entries,
  children,
  menuBoundaryRef,
}: {
  entries: KanbanCardMenuEntry[];
  children: React.ReactNode;
  menuBoundaryRef?: RefObject<HTMLDivElement | null>;
}) {
  const { isDesktop } = useResponsiveBreakpoint();

  if (!isDesktop) return children;

  return (
    <ContextMenu>
      <ContextMenuTrigger className="block">{children}</ContextMenuTrigger>
      <ContextMenuContent
        ref={menuBoundaryRef}
        className="w-56"
        onInteractOutside={(event) => {
          if (isWorkflowMoveOptionsTarget(event.target)) event.preventDefault();
        }}
      >
        <KanbanCardContextMenuItems entries={entries} />
      </ContextMenuContent>
    </ContextMenu>
  );
}
