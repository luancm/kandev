type NestableRow = {
  id: string;
  /** An errand shown nested under the session that started it. */
  nested: boolean;
  parentSessionId: string | null;
};

export type SessionRowGroup<T extends NestableRow> = {
  row: T;
  /** Errands started by `row`, in the order the rows were given. */
  children: T[];
};

/**
 * Groups nested rows under their parent row. A nested row whose parent is not
 * in the list stays top level so it is never hidden.
 */
export function nestSessionRows<T extends NestableRow>(rows: readonly T[]): SessionRowGroup<T>[] {
  const ids = new Set(rows.map((row) => row.id));
  const childrenByParent = new Map<string, T[]>();
  for (const row of rows) {
    if (!row.nested || !row.parentSessionId || !ids.has(row.parentSessionId)) continue;
    const siblings = childrenByParent.get(row.parentSessionId) ?? [];
    siblings.push(row);
    childrenByParent.set(row.parentSessionId, siblings);
  }
  const nestedIds = new Set([...childrenByParent.values()].flat().map((row) => row.id));
  return rows
    .filter((row) => !nestedIds.has(row.id))
    .map((row) => ({ row, children: childrenByParent.get(row.id) ?? [] }));
}
