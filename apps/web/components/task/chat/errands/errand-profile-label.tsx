"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { AgentLogo } from "@/components/agent-logo";
import { useAppStore } from "@/components/state-provider";
import { cn } from "@/lib/utils";
import { resolveErrandProfile, type ErrandProfileDisplay } from "./errand-display";

export function useErrandProfile(
  profileId: string | null | undefined,
  fallbackName?: string | null,
): ErrandProfileDisplay {
  const profiles = useAppStore((state) => state.agentProfiles.items);
  return useMemo(
    () => resolveErrandProfile(profiles, profileId, fallbackName),
    [profiles, profileId, fallbackName],
  );
}

/** Agent logo, profile name, and model of an errand, truncating as one unit. */
export function ErrandProfileLabel({
  profileId,
  fallbackName,
  className,
  showLogo = true,
}: {
  profileId: string | null | undefined;
  fallbackName?: string | null;
  className?: string;
  showLogo?: boolean;
}) {
  const { t } = useTranslation();
  const profile = useErrandProfile(profileId, fallbackName);
  const name = profile.name ?? t("task:errandUnknownProfile");
  const full = profile.model
    ? t("task:errandProfileWithModel", { name, model: profile.model })
    : name;
  return (
    <span
      data-testid="errand-profile"
      title={full}
      className={cn("inline-flex min-w-0 items-center gap-1 text-muted-foreground", className)}
    >
      {showLogo && profile.agentName && (
        <AgentLogo agentName={profile.agentName} size={12} className="shrink-0" />
      )}
      <span className="truncate">{name}</span>
      {profile.model && (
        <>
          <span aria-hidden="true" className="shrink-0 text-muted-foreground/40">
            ·
          </span>
          <span className="truncate font-mono text-[10px]">{profile.model}</span>
        </>
      )}
    </span>
  );
}
