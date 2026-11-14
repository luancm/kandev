import type { ErrandsSliceState } from "./types";

export function forgetErrandInspection(
  errands: ErrandsSliceState["errands"],
  sessionId: string,
): void {
  delete errands.detailBySessionId?.[sessionId];
  delete errands.detailObservationRevisionBySessionId?.[sessionId];
  delete errands.detailOwnersBySessionId?.[sessionId];
}
