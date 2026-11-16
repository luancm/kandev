import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";

describe("connection generation", () => {
  it("advances only on actual connected boundaries without any mounted hooks", () => {
    const store = createAppStore();
    const statuses = [
      "disconnected",
      "connected",
      "connected",
      "disconnected",
      "connecting",
      "connected",
    ] as const;
    const generations = statuses.map((status) => {
      store.getState().setConnectionStatus(status);
      return store.getState().connection.generation;
    });
    expect(generations).toEqual([0, 1, 1, 2, 2, 3]);
  });
});
