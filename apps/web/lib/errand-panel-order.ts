import { isLiveErrand, type Errand } from "./types/errands";

type Group = { parentId: string; errands: Errand[] };

function timestamp(errand: Errand): number {
  const value = Date.parse(errand.created_at);
  return Number.isNaN(value) ? -Infinity : value;
}

function groupErrands(errands: Errand[]): Group[] {
  const groups = new Map<string, Errand[]>();
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

export function orderErrandPanel(errands: Errand[]): { active: Group[]; finished: Group[] } {
  return {
    active: groupErrands(errands.filter(isLiveErrand)),
    finished: groupErrands(errands.filter((item) => !isLiveErrand(item))),
  };
}
