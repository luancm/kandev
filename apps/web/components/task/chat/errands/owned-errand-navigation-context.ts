"use client";

import { createContext } from "react";
import type { ErrandInspection } from "@/lib/types/errands";

export const OwnedErrandNavigationContext = createContext<
  ((inspection: ErrandInspection) => boolean) | null
>(null);

export const LinkedOwnedErrandContext = createContext<ErrandInspection | null>(null);
