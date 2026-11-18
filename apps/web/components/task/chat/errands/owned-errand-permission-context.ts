import { createContext } from "react";

export const OwnedErrandPermissionContext = createContext<{
  taskId: string;
  errandId: string;
} | null>(null);
