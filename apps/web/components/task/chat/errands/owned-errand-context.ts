import { createContext } from "react";

export const OwnedErrandContext = createContext<{
  taskId: string;
  errandId: string;
  readScope: string;
} | null>(null);
