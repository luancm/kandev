"use client";

import { createContext, useCallback } from "react";

export type OpenTaskSessionTarget = {
  taskId: string;
  sessionId: string;
  title: string;
  isErrand?: true;
};

/** Embedded surfaces own selection of their ordinary sessions. */
export const TaskSessionNavigationContext = createContext<
  ((target: OpenTaskSessionTarget) => void) | null
>(null);

/** Scope the embedded session selector to its current task. */
export function useScopedTaskSessionNavigation(
  taskId: string,
  onSelect?: (sessionId: string) => void,
) {
  return useCallback(
    (target: OpenTaskSessionTarget) => {
      if (target.taskId === taskId) onSelect?.(target.sessionId);
    },
    [taskId, onSelect],
  );
}
