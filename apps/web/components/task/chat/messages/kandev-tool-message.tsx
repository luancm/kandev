"use client";

import { useCallback, useState, memo } from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import type { Message } from "@/lib/types/http";
import type { PermissionActionType, PermissionOptionKind } from "@/lib/types/permission";
import type { ToolCallMetadata } from "@/components/task/chat/types";
import { extractKandevStem, extractMcpResult } from "./kandev/parse";
import { getKandevRenderer } from "./kandev/registry";
import { PermissionActionRow } from "./permission-action-row";

type KandevToolMessageProps = {
  comment: Message;
  permissionMessage?: Message;
};

type PermissionOption = {
  option_id: string;
  name: string;
  kind: PermissionOptionKind;
};

type PermissionRequestMetadata = {
  pending_id: string;
  tool_call_id: string;
  options: PermissionOption[];
  action_type: PermissionActionType;
  action_details: { command?: string; path?: string; cwd?: string };
  status?: "pending" | "approved" | "rejected";
};

function parsePermission(permissionMessage: Message | undefined) {
  const permissionMetadata = permissionMessage?.metadata as PermissionRequestMetadata | undefined;
  const permissionStatus = permissionMetadata?.status;
  const isPermissionPending =
    !!permissionMessage && permissionStatus !== "approved" && permissionStatus !== "rejected";
  return { permissionMetadata, isPermissionPending };
}

function usePermissionResponseHandlers(
  permissionMetadata: PermissionRequestMetadata | undefined,
  permissionMessage: Message | undefined,
) {
  const [isResponding, setIsResponding] = useState(false);

  const handleRespond = useCallback(
    async (optionId: string, cancelled: boolean = false) => {
      if (!permissionMetadata || !permissionMessage) return;
      const client = getWebSocketClient();
      if (!client) return;
      setIsResponding(true);
      try {
        await client.request("permission.respond", {
          session_id: permissionMessage.session_id,
          pending_id: permissionMetadata.pending_id,
          option_id: cancelled ? undefined : optionId,
          cancelled,
        });
      } catch (error) {
        console.error("Failed to respond to permission request:", error);
      } finally {
        setIsResponding(false);
      }
    },
    [permissionMessage, permissionMetadata],
  );

  const handleApprove = useCallback(() => {
    const opt = permissionMetadata?.options.find(
      (o) => o.kind === "allow_once" || o.kind === "allow_always",
    );
    if (opt) handleRespond(opt.option_id);
  }, [permissionMetadata, handleRespond]);

  const handleReject = useCallback(() => {
    const opt = permissionMetadata?.options.find(
      (o) => o.kind === "reject_once" || o.kind === "reject_always",
    );
    if (opt) {
      handleRespond(opt.option_id);
    } else {
      handleRespond("", true);
    }
  }, [permissionMetadata, handleRespond]);

  return { isResponding, handleApprove, handleReject };
}

// kandevStemOf scans the several fields that may carry the raw MCP tool name
// and returns the first one that parses to a known kandev stem. The fields
// disagree in practice:
//   - `metadata.tool_name`     — not set by the orchestrator today (null).
//   - `metadata.title`         — the raw `mcp__kandev__<tool>_kandev` string.
//   - `comment.content`        — same raw string, redundant with title.
//   - `metadata.normalized.generic.name` — the ACP adapter's *category*
//     (often `"other"`) rather than the tool name, so it cannot be matched on.
// We iterate candidates instead of picking a single "preferred" one because
// the live data showed `generic.name = "other"`, which would short-circuit
// any priority-based ordering on the wrong field.
function kandevStemOf(comment: Message): string | null {
  const meta = comment.metadata as ToolCallMetadata | undefined;
  const candidates: Array<string | undefined> = [
    meta?.tool_name,
    meta?.title,
    comment.content || undefined,
    meta?.normalized?.generic?.name,
  ];
  for (const candidate of candidates) {
    const stem = extractKandevStem(candidate);
    if (stem) return stem;
  }
  return null;
}

// hasKandevRenderer is the matcher used by the message dispatcher. It accepts
// any `tool_call` whose tool name is recognised as a Kandev MCP tool AND for
// which a per-tool renderer is registered. We require both because we still
// want unregistered Kandev tools to fall through to the generic ToolCallMessage
// (rather than rendering an empty row) until a dedicated renderer ships.
export function hasKandevRenderer(comment: Message): boolean {
  if (comment.type !== "tool_call") return false;
  return !!getKandevRenderer(kandevStemOf(comment));
}

// KandevToolMessage is the rendered entry point for every Kandev tool call.
// It parses the metadata once, looks up the per-tool renderer, and hands the
// renderer pre-parsed args + result. If the renderer lookup fails we render
// nothing rather than crashing; the matcher above guards against this in
// practice, but defensive nulling avoids a bad runtime error if the dispatcher
// rules drift out of sync with the registry.
export const KandevToolMessage = memo(function KandevToolMessage({
  comment,
  permissionMessage,
}: KandevToolMessageProps) {
  const meta = comment.metadata as ToolCallMetadata | undefined;
  const renderer = getKandevRenderer(kandevStemOf(comment));

  const { permissionMetadata, isPermissionPending } = parsePermission(permissionMessage);
  const { isResponding, handleApprove, handleReject } = usePermissionResponseHandlers(
    permissionMetadata,
    permissionMessage,
  );

  if (!renderer) return null;

  const generic = meta?.normalized?.generic;
  const argsCandidate = (generic?.input as Record<string, unknown> | undefined) ?? meta?.args;
  const args = argsCandidate && typeof argsCandidate === "object" ? argsCandidate : undefined;
  const rawResult = generic?.output ?? meta?.result;
  const result = extractMcpResult(rawResult);

  if (!isPermissionPending) {
    return renderer({ args, result, status: meta?.status });
  }

  return (
    <>
      {renderer({ args, result, status: meta?.status })}
      <div className="pl-4 border-l-2 border-border/30 mt-1">
        <PermissionActionRow
          onApprove={handleApprove}
          onReject={handleReject}
          isResponding={isResponding}
        />
      </div>
    </>
  );
});
