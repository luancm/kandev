"use client";

import { createContext, useCallback } from "react";

export type OpenTaskSessionTarget = {
  taskId: string;
  sessionId: string;
  title: string;
};

/** Embedded conversations keep Open and Back with their owning session selector. */
export const TaskSessionNavigationContext = createContext<
  ((target: OpenTaskSessionTarget) => void) | null
>(null);

/** Embedded selectors keep both Open and Back inside their current task. */
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
