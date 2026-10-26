import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { selectLiveErrandCount, selectTaskErrands } from "@/lib/state/slices/errands/selectors";
import type { Errand } from "@/lib/types/errands";
import { ERRAND_TASK_ID, makeErrand } from "@/lib/state/slices/errands/errands.test-helpers";
import { registerWsHandlers } from "@/lib/ws/router";
import { registerErrandsHandlers } from "./errands";

function errandUpdated(payload: Partial<Errand> | null) {
  return { type: "notification", action: "errand.updated", payload } as never;
}

describe("errand.updated", () => {
  it("upserts the errand into its task", () => {
    const store = createAppStore();
    registerErrandsHandlers(store)["errand.updated"]!(errandUpdated(makeErrand()));

    expect(selectTaskErrands(store.getState(), ERRAND_TASK_ID)).toEqual([makeErrand()]);
  });

  it("applies updates in timestamp order regardless of arrival order", () => {
    const store = createAppStore();
    const handle = registerErrandsHandlers(store)["errand.updated"]!;
    handle(errandUpdated(makeErrand({ status: "done", updated_at: "2026-10-26T10:00:09Z" })));
    handle(errandUpdated(makeErrand({ status: "running", updated_at: "2026-10-26T10:00:01Z" })));

    expect(selectTaskErrands(store.getState(), ERRAND_TASK_ID)[0].status).toBe("done");
    expect(selectLiveErrandCount(store.getState(), ERRAND_TASK_ID)).toBe(0);
  });

  it("ignores payloads without an errand or task identity", () => {
    const store = createAppStore();
    const handle = registerErrandsHandlers(store)["errand.updated"]!;
    handle(errandUpdated(null));
    handle(errandUpdated(makeErrand({ errand_id: "" })));
    handle(errandUpdated(makeErrand({ task_id: "" })));

    expect(store.getState().errands.byTaskId).toEqual({});
  });

  it("is registered in the WebSocket handler registry", () => {
    const store = createAppStore();
    const { handlers, dispose } = registerWsHandlers(store);
    handlers["errand.updated"]!(errandUpdated(makeErrand()));
    dispose();

    expect(selectTaskErrands(store.getState(), ERRAND_TASK_ID)).toHaveLength(1);
  });
});
