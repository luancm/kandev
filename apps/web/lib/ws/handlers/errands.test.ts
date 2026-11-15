import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import {
  selectErrandBySessionId,
  selectErrandBrowserSummary,
} from "@/lib/state/slices/errands/selectors";
import { stateReadScopeIdentity } from "@/lib/state/shared-resource-reads";
import type { Errand } from "@/lib/types/errands";
import {
  ERRAND_TASK_ID,
  makeErrand,
  makeBrowserErrandRow,
  makeBrowserSummary,
} from "@/lib/state/slices/errands/errands.test-helpers";
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

    expect(selectErrandBySessionId(store.getState(), ERRAND_TASK_ID, "errand-1")).toEqual(
      makeErrand(),
    );
  });

  it("applies updates in timestamp order regardless of arrival order", () => {
    const store = createAppStore();
    const handle = registerErrandsHandlers(store)["errand.updated"]!;
    store.getState().setConnectionStatus("connected");
    const observation = {
      connectionGeneration: store.getState().connection.generation ?? 0,
      readScopeIdentity: stateReadScopeIdentity(store.getState()),
      taskResetEpoch: 0,
      observationRevision: 0,
    };
    const pending = makeBrowserErrandRow({
      status: "done",
      result_state: "pending",
      outstanding: true,
      delivery_state: "uncertain",
      updated_at: "2026-10-26T10:00:09Z",
    });
    store.getState().beginErrandBrowserSummaryRead(ERRAND_TASK_ID, observation, "summary");
    store.getState().setErrandBrowserSummary(
      ERRAND_TASK_ID,
      makeBrowserSummary([pending], {
        live: 0,
        outstanding: 1,
        pending_results: 1,
      }),
      observation,
      "summary",
    );
    handle(errandUpdated(makeErrand({ status: "running", updated_at: "2026-10-26T10:00:01Z" })));

    const summary = selectErrandBrowserSummary(store.getState(), ERRAND_TASK_ID);
    expect(summary?.errands).toEqual([pending]);
    expect(summary?.counts).toMatchObject({ live: 0, outstanding: 1, pending_results: 1 });
    expect(selectErrandBySessionId(store.getState(), ERRAND_TASK_ID, "errand-1")).toBeUndefined();
  });

  it("applies equal-timestamp receipt and omission updates to only their errand", () => {
    const store = createAppStore();
    const handle = registerErrandsHandlers(store)["errand.updated"]!;
    const sibling = makeErrand({ errand_id: "errand-sibling" });
    handle(errandUpdated(withDeliveryState(makeErrand(), "uncertain")));
    handle(errandUpdated(sibling));

    handle(errandUpdated(withDeliveryState(makeErrand(), "received")));
    expect(selectErrandBySessionId(store.getState(), ERRAND_TASK_ID, "errand-1")).toHaveProperty(
      "delivery_state",
      "received",
    );
    expect(
      selectErrandBySessionId(store.getState(), ERRAND_TASK_ID, sibling.errand_id),
    ).not.toHaveProperty("delivery_state");

    handle(errandUpdated(makeErrand()));
    expect(
      selectErrandBySessionId(store.getState(), ERRAND_TASK_ID, "errand-1"),
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

    expect(selectErrandBySessionId(store.getState(), ERRAND_TASK_ID, "errand-1")).toEqual(
      makeErrand(),
    );
  });
});

describe("errand.updated attention reason", () => {
  it("projects recovery transitions and clears a missing reason", () => {
    const store = createAppStore();
    const handle = registerErrandsHandlers(store)["errand.updated"]!;
    const recovery = {
      ...makeErrand({
        status: "needs_attention",
        result_state: "delivered",
        result_preview: "Prior result",
        result_truncated: true,
      }),
      attention_reason: "recovery_unresolved",
      delivery_state: "uncertain" as const,
    } as Errand;
    handle(errandUpdated(recovery));
    expect(
      selectErrandBySessionId(store.getState(), ERRAND_TASK_ID, "errand-1")?.attention_reason,
    ).toBe("recovery_unresolved");

    handle(
      errandUpdated({
        ...recovery,
        attention_reason: "permission",
        updated_at: "2026-10-26T10:00:02Z",
      } as Errand),
    );
    expect(
      selectErrandBySessionId(store.getState(), ERRAND_TASK_ID, "errand-1")?.attention_reason,
    ).toBe("permission");

    const running = {
      ...recovery,
      status: "running" as const,
      updated_at: "2026-10-26T10:00:03Z",
    };
    delete running.attention_reason;
    handle(errandUpdated(running));

    const projected = selectErrandBySessionId(store.getState(), ERRAND_TASK_ID, "errand-1");
    expect(projected).not.toHaveProperty("attention_reason");
    expect(projected).toMatchObject({
      status: "running",
      result_state: "delivered",
      result_preview: "Prior result",
      result_truncated: true,
      delivery_state: "uncertain",
    });
  });
});
