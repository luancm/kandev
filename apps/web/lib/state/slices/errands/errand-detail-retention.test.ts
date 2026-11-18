import { expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { makeBrowserErrandRow, makeInspection } from "./errands.test-helpers";

function ownedInspection(errandId: string) {
  return makeInspection(makeBrowserErrandRow({ errand_id: errandId }));
}

it("keeps compact Errand detail until its final reader leaves", () => {
  const store = createAppStore();
  const inspection = ownedInspection("errand-1");
  store.getState().retainErrandInspection("host", inspection);
  store.getState().retainErrandInspection("header", inspection);

  store.getState().releaseErrandInspection("host");
  expect(store.getState().errands.detailBySessionId["errand-1"]).toEqual(inspection);
  store.getState().releaseErrandInspection("header");
  expect(store.getState().errands.detailBySessionId["errand-1"]).toBeUndefined();
  expect(store.getState().taskSessions.items["errand-1"]).toBeUndefined();
});

it("keeps one compact inspection when an owner opens many different Errands", () => {
  const store = createAppStore();
  for (let index = 0; index < 100; index++) {
    store.getState().retainErrandInspection("reader", ownedInspection(`errand-${index}`));
  }
  expect(Object.keys(store.getState().errands.detailBySessionId)).toEqual(["errand-99"]);
  expect(Object.keys(store.getState().taskSessions.items)).toHaveLength(0);
  expect(Object.keys(store.getState().messages.bySession)).toHaveLength(0);

  store.getState().releaseErrandInspection("reader");
  expect(Object.keys(store.getState().errands.detailBySessionId)).toHaveLength(0);
});
