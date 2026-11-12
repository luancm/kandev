import { randomUUID } from "node:crypto";
import { expect } from "@playwright/test";
import type { Errand } from "../../lib/types/errands";
import type { ApiClient } from "./api-client";
import { deliverySnapshots } from "./errands";

export type ErrandDeliverySnapshot = {
  errand_id: string;
  duration_ms?: number;
  [key: string]: unknown;
};

function isErrandDeliverySnapshot(value: unknown): value is ErrandDeliverySnapshot {
  return (
    value !== null &&
    typeof value === "object" &&
    "errand_id" in value &&
    typeof value.errand_id === "string"
  );
}

function epochNanoseconds(value: string): bigint | null {
  const dot = value.indexOf(".");
  let wholePart = value;
  let fractionDigits = "";
  if (dot !== -1) {
    const rest = value.slice(dot + 1);
    const digitMatch = /^\d+/.exec(rest);
    fractionDigits = digitMatch ? digitMatch[0] : "";
    wholePart = value.slice(0, dot) + rest.slice(fractionDigits.length);
  }
  const parsed = Date.parse(wholePart);
  if (Number.isNaN(parsed)) return null;
  const seconds = BigInt(Math.floor(parsed / 1000));
  const fractionNanoseconds = BigInt(fractionDigits.padEnd(9, "0").slice(0, 9) || "0");
  return seconds * 1_000_000_000n + fractionNanoseconds;
}

export async function recordedDurationSnapshot(
  api: ApiClient,
  parentId: string,
  errandId: string,
): Promise<ErrandDeliverySnapshot> {
  const snapshot = (await deliverySnapshots(api, parentId)).find(
    (value): value is ErrandDeliverySnapshot =>
      isErrandDeliverySnapshot(value) && value.errand_id === errandId,
  );
  if (!snapshot) throw new Error(`Expected a delivered result snapshot for errand ${errandId}`);
  return snapshot;
}

export async function recordedDurationSnapshotsForErrand(
  api: ApiClient,
  parentId: string,
  errandId: string,
): Promise<ErrandDeliverySnapshot[]> {
  return (await deliverySnapshots(api, parentId)).filter(
    (value): value is ErrandDeliverySnapshot =>
      isErrandDeliverySnapshot(value) && value.errand_id === errandId,
  );
}

export async function waitNewTurnSettled(
  api: ApiClient,
  sessionId: string,
  previousTurnIds: string[],
) {
  const previous = new Set(previousTurnIds);
  await expect
    .poll(
      async () => {
        const turns = (await api.listSessionTurns(sessionId)).turns.filter(
          (turn) => !previous.has(turn.id) && turn.metadata?.lifecycle_only !== true,
        );
        if (turns.length > 1) {
          throw new Error(`Expected one child follow-up turn; observed ${JSON.stringify(turns)}`);
        }
        const messages = (await api.listSessionMessages(sessionId)).messages;
        return {
          completed: turns.length === 1 && Boolean(turns[0].completed_at),
          promptErrors: messages
            .filter((message) => message.type === "error" && !previous.has(message.turn_id ?? ""))
            .map((message) => message.metadata?.error ?? message.content),
        };
      },
      { message: "The child follow-up turn completes", timeout: 60_000 },
    )
    .toEqual({ completed: true, promptErrors: [] });
}

export function recordedElapsedDurationMs(errand: Errand): number {
  if (!errand.settled_at) throw new Error(`Expected errand ${errand.errand_id} to be settled`);
  const createdAt = epochNanoseconds(errand.created_at);
  const settledAt = epochNanoseconds(errand.settled_at);
  if (createdAt === null || settledAt === null) {
    throw new Error(`Expected parseable timestamps for errand ${errand.errand_id}`);
  }
  const elapsedNanoseconds = settledAt - createdAt;
  expect(elapsedNanoseconds >= 0n).toBe(true);
  return Number(elapsedNanoseconds / 1_000_000n);
}

export function expectRecordedDuration(snapshot: ErrandDeliverySnapshot, errand: Errand) {
  expect(snapshot.duration_ms).toBe(recordedElapsedDurationMs(errand));
  expect(snapshot.duration_ms).toBeGreaterThan(0);
}

/** Add isolated long history for rendered-fit checks; this does not claim a mock run took hours. */
export async function seedLongDurationHistory(api: ApiClient, parentId: string) {
  const errandId = `duration-visual-${randomUUID()}`;
  const message = await api.seedSessionMessage(parentId, {
    type: "message",
    authorType: "user",
    metadata: {
      errand_delivery: {
        token: `duration-visual-${randomUUID()}`,
        route: "notification",
        items: [
          {
            errand_id: errandId,
            title: "Long recorded duration fixture",
            status: "done",
            preview: "This isolated history row checks that long duration labels fit on a phone.",
            duration_ms: (123 * 3600 + 45 * 60 + 56) * 1000,
          },
        ],
      },
    },
  });
  return { errandId, messageId: message.messageId };
}
