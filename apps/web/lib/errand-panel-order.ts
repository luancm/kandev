import { isLiveErrand, type ErrandStatus } from "./types/errands";

type PanelErrand = {
  errand_id: string;
  parent_session_id: string;
  created_at: string;
  status: ErrandStatus;
  outstanding?: boolean;
};
type Group<T extends PanelErrand> = { parentId: string; errands: T[] };

function timestamp(errand: PanelErrand): number {
  const value = Date.parse(errand.created_at);
  return Number.isNaN(value) ? -Infinity : value;
}

function groupErrands<T extends PanelErrand>(errands: T[]): Group<T>[] {
  const groups = new Map<string, T[]>();
  for (const errand of errands) {
    const group = groups.get(errand.parent_session_id) ?? [];
    group.push(errand);
    groups.set(errand.parent_session_id, group);
  }
  return Array.from(groups, ([parentId, rows]) => ({
    parentId,
    errands: rows.sort(
      (a, b) => timestamp(b) - timestamp(a) || a.errand_id.localeCompare(b.errand_id),
    ),
  })).sort(
    (a, b) =>
      timestamp(b.errands[0]) - timestamp(a.errands[0]) || a.parentId.localeCompare(b.parentId),
  );
}

export function orderErrandPanel<T extends PanelErrand>(
  errands: T[],
): { active: Group<T>[]; finished: Group<T>[] } {
  const isOutstanding = (item: T) =>
    typeof item.outstanding === "boolean" ? item.outstanding : isLiveErrand(item);
  return {
    active: groupErrands(errands.filter(isOutstanding)),
    finished: groupErrands(errands.filter((item) => !isOutstanding(item))),
  };
}
