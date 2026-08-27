"use client";

import { useRef, type RefObject } from "react";
import {
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
} from "@kandev/ui/context-menu";
import {
  DropdownMenuItem,
  DropdownMenuPortal,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
} from "@kandev/ui/dropdown-menu";
import type { KanbanCardMenuEntry } from "./kanban-card-menu-items";

type MenuEntryProps = {
  entry: KanbanCardMenuEntry;
  fallbackAnchorRef?: RefObject<HTMLElement | null>;
};

export function ContextEntry({ entry, fallbackAnchorRef }: MenuEntryProps) {
  const pointerTypeRef = useRef<string | null>(null);
  const submenuAnchorRef = useRef<HTMLDivElement | null>(null);
  if (entry.kind === "separator") return <ContextMenuSeparator />;
  if (entry.kind === "submenu") {
    return (
      <ContextMenuSub>
        <ContextMenuSubTrigger
          ref={submenuAnchorRef}
          data-testid={entry.testId}
          disabled={entry.disabled}
          onPointerDown={(event) => {
            event.stopPropagation();
            pointerTypeRef.current = event.pointerType;
          }}
          onPointerCancel={() => {
            pointerTypeRef.current = null;
          }}
          onClick={(event) => {
            event.stopPropagation();
            const pointerType = pointerTypeRef.current;
            pointerTypeRef.current = null;
            const rect = event.currentTarget.getBoundingClientRect();
            const tappedChevron =
              (pointerType === "touch" || pointerType === "pen") &&
              rect.width > 0 &&
              event.clientX >= rect.right - 32;
            if (!tappedChevron && !entry.disabled && entry.onSelect) {
              event.preventDefault();
              entry.onSelect(event.currentTarget as HTMLElement);
            }
          }}
          onKeyDown={(event) => {
            if (!entry.disabled && entry.onSelect && (event.key === "Enter" || event.key === " ")) {
              event.stopPropagation();
              event.preventDefault();
              entry.onSelect(event.currentTarget as HTMLElement);
            }
          }}
        >
          {entry.icon}
          {entry.label}
        </ContextMenuSubTrigger>
        <ContextMenuSubContent className={entry.className}>
          {entry.children.map((child) => (
            <ContextEntry
              key={child.key}
              entry={child}
              fallbackAnchorRef={fallbackAnchorRef ?? submenuAnchorRef}
            />
          ))}
        </ContextMenuSubContent>
      </ContextMenuSub>
    );
  }

  return (
    <ContextMenuItem
      data-testid={entry.testId}
      disabled={entry.disabled}
      className={entry.destructive ? "text-destructive focus:text-destructive" : undefined}
      // React events bubble through a React portal; stop here so the card's onClick does not navigate.
      onClick={(event) => event.stopPropagation()}
      onSelect={(event) => {
        if (entry.keepMenuOpen) event.preventDefault();
        if (!entry.disabled) {
          entry.onSelect?.(fallbackAnchorRef?.current ?? (event.currentTarget as HTMLElement));
        }
      }}
    >
      {entry.icon}
      {entry.leading}
      {entry.label}
      {entry.trailing}
    </ContextMenuItem>
  );
}

export function DropdownEntry({ entry, fallbackAnchorRef }: MenuEntryProps) {
  const pointerTypeRef = useRef<string | null>(null);
  const submenuAnchorRef = useRef<HTMLDivElement | null>(null);
  if (entry.kind === "separator") return <DropdownMenuSeparator />;
  if (entry.kind === "submenu") {
    return (
      <DropdownMenuSub>
        <DropdownMenuSubTrigger
          ref={submenuAnchorRef}
          data-testid={entry.testId}
          disabled={entry.disabled}
          onClick={(event) => {
            event.stopPropagation();
            const pointerType = pointerTypeRef.current;
            pointerTypeRef.current = null;
            const rect = event.currentTarget.getBoundingClientRect();
            const tappedChevron =
              (pointerType === "touch" || pointerType === "pen") &&
              rect.width > 0 &&
              event.clientX >= rect.right - 32;
            if (!tappedChevron && !entry.disabled && entry.onSelect) {
              event.preventDefault();
              entry.onSelect(event.currentTarget as HTMLElement);
            }
          }}
          onPointerDown={(event) => {
            event.stopPropagation();
            pointerTypeRef.current = event.pointerType;
          }}
          onPointerCancel={() => {
            pointerTypeRef.current = null;
          }}
          onKeyDown={(event) => {
            if (!entry.disabled && entry.onSelect && (event.key === "Enter" || event.key === " ")) {
              event.stopPropagation();
              event.preventDefault();
              entry.onSelect(event.currentTarget as HTMLElement);
            }
          }}
        >
          {entry.icon}
          {entry.label}
        </DropdownMenuSubTrigger>
        <DropdownMenuPortal>
          <DropdownMenuSubContent className={entry.className}>
            {entry.children.map((child) => (
              <DropdownEntry
                key={child.key}
                entry={child}
                fallbackAnchorRef={fallbackAnchorRef ?? submenuAnchorRef}
              />
            ))}
          </DropdownMenuSubContent>
        </DropdownMenuPortal>
      </DropdownMenuSub>
    );
  }

  return (
    <DropdownMenuItem
      data-testid={entry.testId}
      disabled={entry.disabled}
      className={entry.destructive ? "text-destructive focus:text-destructive" : undefined}
      // React events bubble through a portal; stop here so click/pointer do not reach the card.
      onClick={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
      onSelect={(event) => {
        if (entry.keepMenuOpen) event.preventDefault();
        event.stopPropagation();
        if (!entry.disabled) {
          entry.onSelect?.(fallbackAnchorRef?.current ?? (event.currentTarget as HTMLElement));
        }
      }}
    >
      {entry.icon}
      {entry.leading}
      {entry.label}
      {entry.trailing}
    </DropdownMenuItem>
  );
}
