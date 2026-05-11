"use client";

import { useCallback, useMemo, useState } from "react";
import { Tabs, TabsList, TabsTrigger } from "@kandev/ui/tabs";
import { useAppStore } from "@/components/state-provider";
import { useReviewSources } from "@/hooks/domains/session/use-review-sources";
import { TaskChangesPanel } from "../task-changes-panel";
import type { SelectedDiff } from "../task-layout";
import {
  STORAGE_KEY,
  availableTabs,
  pickInitialTab,
  type TabId,
} from "./mobile-changes-tabs-helpers";

type MobileChangesTabsProps = {
  selectedDiff: SelectedDiff | null;
  onClearSelected: () => void;
  onOpenFile?: (filePath: string) => void;
};

const TAB_LABELS: Record<TabId, string> = {
  uncommitted: "Uncommitted",
  committed: "Committed",
  pr: "PR",
};

function readSavedTab(): string | null {
  if (typeof window === "undefined") return null;
  try {
    return window.localStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

function writeSavedTab(tab: TabId) {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(STORAGE_KEY, tab);
  } catch {
    // Ignore quota / disabled storage.
  }
}

export function MobileChangesTabs({
  selectedDiff,
  onClearSelected,
  onOpenFile,
}: MobileChangesTabsProps) {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const { allFiles, sourceCounts, hasPR } = useReviewSources(activeSessionId);

  const tabs = useMemo(() => availableTabs(sourceCounts, hasPR), [sourceCounts, hasPR]);

  // `savedTab` holds the user's last explicit pick — initialized from
  // localStorage, updated only when the user taps a tab. Derived state
  // (current tab, sourceFilter) is computed during render.
  const [savedTab, setSavedTab] = useState<string | null>(() => readSavedTab());

  const currentTab = useMemo<TabId | null>(() => {
    // External selectedDiff wins: switch to the tab containing that file
    // so the scroll effect inside TaskChangesPanel finds the ref. Once the
    // diff is cleared by TaskChangesPanel's scroll handler, the saved tab
    // takes over again.
    if (selectedDiff?.path) {
      const file = allFiles.find((f) => f.path === selectedDiff.path);
      if (file && tabs.includes(file.source)) return file.source;
    }
    return pickInitialTab(savedTab, sourceCounts, hasPR);
  }, [selectedDiff, allFiles, tabs, savedTab, sourceCounts, hasPR]);

  const handleTabChange = useCallback((next: string) => {
    setSavedTab(next);
    writeSavedTab(next as TabId);
  }, []);

  const sourceFilter = currentTab ?? "all";
  const showBar = tabs.length >= 2;

  return (
    <div className="flex flex-1 min-h-0 flex-col">
      {showBar && currentTab && (
        <Tabs value={currentTab} onValueChange={handleTabChange} className="px-2 pt-1">
          <TabsList variant="line" className="w-full justify-start">
            {tabs.map((id) => (
              <TabsTrigger key={id} value={id} className="cursor-pointer gap-1.5">
                <span>{TAB_LABELS[id]}</span>
                {sourceCounts[id] > 0 && (
                  <span className="text-muted-foreground text-[10px]">{sourceCounts[id]}</span>
                )}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      )}
      <div className="flex-1 min-h-0">
        <TaskChangesPanel
          selectedDiff={selectedDiff}
          onClearSelected={onClearSelected}
          onOpenFile={onOpenFile}
          sourceFilter={sourceFilter}
        />
      </div>
    </div>
  );
}
