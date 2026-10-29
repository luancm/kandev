"use client";

import { createContext } from "react";

export type OpenTaskSessionTarget = {
  taskId: string;
  sessionId: string;
  title: string;
};

/** Embedded conversations keep Open and Back with their owning session selector. */
export const TaskSessionNavigationContext = createContext<
  ((target: OpenTaskSessionTarget) => void) | null
>(null);
