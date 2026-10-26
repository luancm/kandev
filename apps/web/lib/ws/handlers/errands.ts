import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import type { WsHandlers } from "@/lib/ws/handlers/types";

/** Live errand rows, broadcast to the workspace on every durable errand change. */
export function registerErrandsHandlers(store: StoreApi<AppState>): WsHandlers {
  return {
    "errand.updated": (message) => {
      const errand = message.payload;
      if (!errand?.errand_id || !errand.task_id) return;
      store.getState().upsertErrand(errand);
    },
  };
}
