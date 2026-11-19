"use client";

import { useRef, useState } from "react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { isErrandRecoveryRefusal } from "@/lib/chat/message-send-error";
import { generateUUID } from "@/lib/uuid";
import { getWebSocketClient } from "@/lib/ws/connection";
import { TipTapInput } from "../tiptap-input";

type Props = {
  taskId: string;
  errandId: string;
  connected: boolean;
  available: boolean;
  onSent: () => void;
};

export function OwnedErrandContinuation({ taskId, errandId, connected, available, onSent }: Props) {
  const { t } = useTranslation();
  const submitKey = useAppStore((state) => state.userSettings.chatSubmitKey);
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [error, setError] = useState<"recovery" | "send" | null>(null);
  const inFlight = useRef(false);
  const lastRequest = useRef<{ content: string; id: string } | null>(null);
  const content = draft.trim();

  async function send() {
    const client = getWebSocketClient();
    if (!content || !available || !connected || !client || inFlight.current) return;
    const request = lastRequest.current;
    const id = request?.content === content ? request.id : generateUUID();
    lastRequest.current = { content, id };
    inFlight.current = true;
    setSending(true);
    setError(null);
    try {
      await client.request("errand.continue", {
        task_id: taskId,
        errand_id: errandId,
        client_message_id: id,
        content,
      });
      lastRequest.current = null;
      setDraft("");
      onSent();
    } catch (failure) {
      setError(isErrandRecoveryRefusal(failure) ? "recovery" : "send");
    } finally {
      inFlight.current = false;
      setSending(false);
    }
  }

  if (!available && !sending) return null;

  return (
    <div
      data-testid="errand-continuation"
      className="shrink-0 border-t bg-background p-2 max-md:pb-[max(.5rem,env(safe-area-inset-bottom))]"
    >
      {error && (
        <p role="alert" className="mb-2 text-sm text-destructive">
          {error === "recovery"
            ? t("task:errandRecoverySendRefused")
            : t("task:failedToSendMessage")}
        </p>
      )}
      <div className="flex min-w-0 items-end gap-2">
        <TipTapInput
          sessionId={null}
          taskId={taskId}
          value={draft}
          onChange={setDraft}
          onSubmit={() => void send()}
          submitKey={submitKey}
          disabled={sending}
          placeholder={t("task:askToMakeChanges")}
          className="min-w-0 flex-1"
        />
        <Button
          type="button"
          size="sm"
          className="shrink-0 max-md:min-h-11 max-md:min-w-11 pointer-coarse:min-h-11 pointer-coarse:min-w-11"
          disabled={!content || !available || !connected || sending}
          onClick={() => void send()}
        >
          {t("task:send")}
        </Button>
      </div>
    </div>
  );
}
