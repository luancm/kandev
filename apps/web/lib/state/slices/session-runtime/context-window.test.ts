import { describe, expect, it } from "vitest";
import {
  normalizeContextCompactionCount,
  parseContextWindowEntry,
  readContextWindowMetadata,
  readContextWindowMetadataFromAliases,
  updateContextWindowMetadata,
} from "./context-window";

describe("parseContextWindowEntry", () => {
  it.each(["acp", "api"] as const)("retains the %s source", (source) => {
    expect(
      parseContextWindowEntry({
        size: 258_400,
        used: 95_100,
        remaining: 163_300,
        efficiency: 36.8,
        source,
      }),
    ).toEqual({
      size: 258_400,
      used: 95_100,
      remaining: 163_300,
      efficiency: 36.8,
      compactionCount: 0,
      source,
      timestamp: undefined,
    });
  });

  it("drops an unknown source from older or malformed metadata", () => {
    expect(parseContextWindowEntry({ size: 128_000, source: "cache" })).toEqual(
      expect.objectContaining({ source: undefined }),
    );
  });

  it("parses a caller-supplied persisted inferred compaction count", () => {
    expect(
      parseContextWindowEntry(
        {
          size: 128_000,
          used: 64_000,
          remaining: 64_000,
          efficiency: 50,
        },
        undefined,
        3,
      ),
    ).toEqual(expect.objectContaining({ compactionCount: 3 }));
  });

  it.each([
    { label: "missing", value: undefined },
    { label: "invalid", value: "3" },
    { label: "negative", value: -2 },
  ])("normalizes a $label compaction count to zero", ({ value }) => {
    expect(
      parseContextWindowEntry(
        { size: 128_000, used: 64_000, compaction_count: value },
        undefined,
        value,
      ),
    ).toEqual(expect.objectContaining({ compactionCount: 0 }));
  });

  it("prefers a caller-supplied timestamp over embedded metadata", () => {
    expect(
      parseContextWindowEntry(
        {
          size: 128_000,
          used: 64_000,
          remaining: 64_000,
          efficiency: 50,
          timestamp: "stored",
        },
        "live-2026-07-17T11:00:00.000Z",
      ),
    ).toEqual(expect.objectContaining({ timestamp: "live-2026-07-17T11:00:00.000Z" }));
  });

  it("rejects non-object metadata", () => {
    expect(parseContextWindowEntry(null)).toBeNull();
  });
});

describe("context-window metadata ownership", () => {
  it("distinguishes omitted context from an explicit null using own properties", () => {
    expect(readContextWindowMetadata({ unrelated: true })).toEqual({ present: false });
    expect(readContextWindowMetadata({ context_window: null })).toEqual({
      present: true,
      contextWindow: null,
      compactionCount: undefined,
    });
    expect(readContextWindowMetadata(Object.create({ context_window: null }))).toEqual({
      present: false,
    });
  });

  it("prefers a present metadata context and falls back to session_metadata", () => {
    expect(
      readContextWindowMetadataFromAliases(
        { context_window: null },
        { context_window: { size: 128_000 } },
      ),
    ).toEqual({ present: true, contextWindow: null, compactionCount: undefined });
    expect(
      readContextWindowMetadataFromAliases(
        { unrelated: true },
        { context_window: { size: 128_000 }, context_compaction_count: 3 },
      ),
    ).toEqual({
      present: true,
      contextWindow: { size: 128_000 },
      compactionCount: 3,
    });
  });

  it("keeps metadata explicit on clear and writes the parsed compaction count on set", () => {
    expect(
      updateContextWindowMetadata(
        { plan_mode: true, context_window: { size: 128_000 }, context_compaction_count: 2 },
        null,
      ),
    ).toEqual({ plan_mode: true, context_window: null, context_compaction_count: 2 });
    expect(
      updateContextWindowMetadata(
        { plan_mode: true },
        {
          size: 128_000,
          used: 64_000,
          remaining: 64_000,
          efficiency: 50,
          compactionCount: 3.8,
          source: "acp",
        },
      ),
    ).toEqual({
      plan_mode: true,
      context_window: {
        size: 128_000,
        used: 64_000,
        remaining: 64_000,
        efficiency: 50,
        source: "acp",
      },
      context_compaction_count: 3,
    });
    expect(normalizeContextCompactionCount(Number.POSITIVE_INFINITY)).toBe(0);
  });
});
