export type ContextWindowUsageInput = {
  size: number;
  used: number;
};

export type ContextWindowUsage = {
  usagePercent: number;
  roundedPercent: string;
  unmeasured: boolean;
};

export function isContextWindowReliable(size: number, used: number): boolean {
  return size > 0 && used <= size;
}

export function deriveContextWindowUsage(
  window: ContextWindowUsageInput | null | undefined,
): ContextWindowUsage | undefined {
  if (!window || !isContextWindowReliable(window.size, window.used)) return undefined;

  const usagePercent = (window.used / window.size) * 100;
  return {
    usagePercent,
    roundedPercent: usagePercent.toFixed(0),
    unmeasured: window.used === 0,
  };
}
