import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { selectLiveErrandCount, selectTaskErrands } from "@/lib/state/slices/errands/selectors";
import type { Errand } from "@/lib/types/errands";
import { ERRAND_TASK_ID, makeErrand } from "@/lib/state/slices/errands/errands.test-helpers";
import { registerWsHandlers } from "@/lib/ws/router";
import { registerErrandsHandlers } from "./errands";

function withDeliveryState(errand: Errand, deliveryState: "received" | "uncertain"): Errand {
  return { ...errand, delivery_state: deliveryState };
}

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
    handle(
      errandUpdated(
        withDeliveryState(
          makeErrand({ status: "done", updated_at: "2026-10-26T10:00:09Z" }),
          "uncertain",
        ),
      ),
    );
    handle(errandUpdated(makeErrand({ status: "running", updated_at: "2026-10-26T10:00:01Z" })));

    expect(selectTaskErrands(store.getState(), ERRAND_TASK_ID)[0].status).toBe("done");
    expect(selectTaskErrands(store.getState(), ERRAND_TASK_ID)[0]).toHaveProperty(
      "delivery_state",
      "uncertain",
    );
    expect(selectLiveErrandCount(store.getState(), ERRAND_TASK_ID)).toBe(0);
  });

  it("applies equal-timestamp receipt and omission updates to only their errand", () => {
    const store = createAppStore();
    const handle = registerErrandsHandlers(store)["errand.updated"]!;
    const sibling = makeErrand({ errand_id: "errand-sibling" });
    handle(errandUpdated(withDeliveryState(makeErrand(), "uncertain")));
    handle(errandUpdated(sibling));

    handle(errandUpdated(withDeliveryState(makeErrand(), "received")));
    expect(
      selectTaskErrands(store.getState(), ERRAND_TASK_ID).find(
        (row) => row.errand_id === "errand-1",
      ),
    ).toHaveProperty("delivery_state", "received");
    expect(
      selectTaskErrands(store.getState(), ERRAND_TASK_ID).find(
        (row) => row.errand_id === sibling.errand_id,
      ),
    ).not.toHaveProperty("delivery_state");

    handle(errandUpdated(makeErrand()));
    expect(
      selectTaskErrands(store.getState(), ERRAND_TASK_ID).find(
        (row) => row.errand_id === "errand-1",
      ),
    ).not.toHaveProperty("delivery_state");
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
