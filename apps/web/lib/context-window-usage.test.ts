import { describe, expect, it } from "vitest";
import { deriveContextWindowUsage } from "./context-window-usage";

describe("deriveContextWindowUsage", () => {
  it("keeps measured precision and exposes the existing rounded display value", () => {
    const usage = deriveContextWindowUsage({ size: 200_000, used: 56_047 });

    expect(usage).toMatchObject({ roundedPercent: "28", unmeasured: false });
    expect(usage?.usagePercent).toBeCloseTo(28.0235);
  });

  it("distinguishes unmeasured from full context and omits unavailable or impossible reports", () => {
    expect(deriveContextWindowUsage({ size: 200_000, used: 0 })).toEqual({
      usagePercent: 0,
      roundedPercent: "0",
      unmeasured: true,
    });
    expect(deriveContextWindowUsage({ size: 200_000, used: 200_000 })).toEqual({
      usagePercent: 100,
      roundedPercent: "100",
      unmeasured: false,
    });
    expect(deriveContextWindowUsage({ size: 0, used: 0 })).toBeUndefined();
    expect(deriveContextWindowUsage({ size: 200_000, used: 233_900 })).toBeUndefined();
    expect(deriveContextWindowUsage(undefined)).toBeUndefined();
  });
});
