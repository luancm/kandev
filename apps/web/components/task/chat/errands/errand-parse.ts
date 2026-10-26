// Parsers for errand tool calls and delivery metadata. Every input is an
// untrusted transcript payload, so each field is narrowed before use and a
// malformed value degrades to "unknown" instead of throwing.

import { parseErrandStatus, type ErrandStatus } from "@/lib/types/errands";
import type { KandevStatus } from "../messages/kandev/shared";
import { pickArray, pickObject, pickString } from "../messages/kandev/parse";

/** Matches the backend's `result_preview` and delivery preview bound. */
export const ERRAND_PREVIEW_MAX_CHARS = 280;
const MAX_LISTED_ERRANDS = 20;
const MAX_DELIVERY_ITEMS = 50;

function nonEmptyString(value: unknown): string | null {
  return typeof value === "string" && value.trim() !== "" ? value : null;
}

function stringField(obj: unknown, key: string): string | null {
  return nonEmptyString(pickString(obj, key));
}

/** Readable text of an MCP error result: a plain string, `error`, or `message`. */
function errorTextOf(result: unknown): string | null {
  if (typeof result === "string") return result.trim();
  return stringField(result, "error") ?? stringField(result, "message");
}

function resolveErrorText(result: unknown, status: KandevStatus, hasPayload: boolean) {
  if (status === "error") return errorTextOf(result) ?? "";
  if (hasPayload || result === null || result === undefined) return null;
  return errorTextOf(result);
}

export type StartErrandView = {
  title: string | null;
  prompt: string | null;
  /** Equals the errand's session ID; null until the start call returns. */
  errandId: string | null;
  status: ErrandStatus | null;
  profileId: string | null;
  profileName: string | null;
  /** Non-null when the call failed; may be empty when the failure has no text. */
  errorText: string | null;
};

export function parseStartErrand(
  args: Record<string, unknown> | undefined,
  result: unknown,
  status: KandevStatus,
): StartErrandView {
  const errandId = stringField(result, "errand_id") ?? stringField(result, "session_id");
  return {
    title: stringField(args, "title"),
    prompt: stringField(args, "prompt"),
    errandId: status === "error" ? null : errandId,
    status: parseErrandStatus(pickString(result, "status")),
    profileId: stringField(result, "agent_profile_id"),
    profileName: stringField(result, "agent_profile_name"),
    errorText: resolveErrorText(result, status, errandId !== null),
  };
}

export type AwaitErrandRow = {
  errandId: string;
  /** Settled or live status the await reported; null while unknown or pending. */
  status: ErrandStatus | null;
  /** The await timed out before this errand settled. */
  pending: boolean;
  /** This await carried the errand's result text to the parent. */
  resultReceived: boolean;
};

export type AwaitErrandsView = {
  mode: "all" | "any";
  rows: AwaitErrandRow[];
  errorText: string | null;
};

function requestedErrandIds(args: Record<string, unknown> | undefined): string[] {
  const ids: string[] = [];
  for (const value of pickArray(args, "errand_ids") ?? []) {
    const id = nonEmptyString(value);
    if (id && !ids.includes(id)) ids.push(id);
  }
  return ids.slice(0, MAX_LISTED_ERRANDS);
}

function awaitResultRows(result: unknown): Map<string, AwaitErrandRow> {
  const rows = new Map<string, AwaitErrandRow>();
  for (const entry of pickArray(result, "errands") ?? []) {
    const errandId = stringField(entry, "errand_id");
    if (!errandId || rows.has(errandId)) continue;
    const rawStatus = pickString(entry, "status");
    rows.set(errandId, {
      errandId,
      status: parseErrandStatus(rawStatus),
      pending: rawStatus === "pending",
      resultReceived: typeof pickString(entry, "result") === "string",
    });
  }
  return rows;
}

export function parseAwaitErrands(
  args: Record<string, unknown> | undefined,
  result: unknown,
  status: KandevStatus,
): AwaitErrandsView {
  const reported = awaitResultRows(result);
  const ids = requestedErrandIds(args);
  for (const id of reported.keys()) if (!ids.includes(id)) ids.push(id);
  const rows = ids
    .slice(0, MAX_LISTED_ERRANDS)
    .map(
      (errandId) =>
        reported.get(errandId) ?? { errandId, status: null, pending: false, resultReceived: false },
    );
  return {
    mode: pickString(args, "mode") === "any" ? "any" : "all",
    rows,
    errorText: resolveErrorText(result, status, reported.size > 0),
  };
}

export type CancelErrandView = {
  errandId: string | null;
  status: ErrandStatus | null;
  errorText: string | null;
};

export function parseCancelErrand(
  args: Record<string, unknown> | undefined,
  result: unknown,
  status: KandevStatus,
): CancelErrandView {
  const resultStatus = parseErrandStatus(pickString(result, "status"));
  return {
    errandId: stringField(args, "errand_id") ?? stringField(result, "errand_id"),
    status: resultStatus,
    errorText: resolveErrorText(result, status, resultStatus !== null),
  };
}

/** One delivered result, snapshotted on the parent message that carried it. */
export type ErrandDeliveryItem = {
  errandId: string;
  title: string;
  status: ErrandStatus | null;
  preview: string;
};

function capPreview(value: string): string {
  const chars = Array.from(value);
  return chars.length > ERRAND_PREVIEW_MAX_CHARS
    ? chars.slice(0, ERRAND_PREVIEW_MAX_CHARS).join("")
    : value;
}

function parseDeliveryItem(value: unknown): ErrandDeliveryItem | null {
  const errandId = stringField(value, "errand_id");
  if (!errandId) return null;
  return {
    errandId,
    title: pickString(value, "title") ?? "",
    status: parseErrandStatus(pickString(value, "status")),
    preview: capPreview(pickString(value, "preview") ?? ""),
  };
}

/** Items of a parent message's `errand_delivery` metadata; empty when absent or malformed. */
export function parseErrandDelivery(metadata: unknown): ErrandDeliveryItem[] {
  const delivery = pickObject(metadata, "errand_delivery");
  const items: ErrandDeliveryItem[] = [];
  for (const value of pickArray(delivery, "items") ?? []) {
    const item = parseDeliveryItem(value);
    if (item) items.push(item);
    if (items.length >= MAX_DELIVERY_ITEMS) break;
  }
  return items;
}
