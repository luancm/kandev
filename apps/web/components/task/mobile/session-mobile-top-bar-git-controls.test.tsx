import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { GitActionsDropdown, computeMobileGitStats } from "./session-mobile-top-bar-git-controls";

vi.mock("@/hooks/use-git-operations", () => ({
  useChangeRequestTerminology: () => ({ shortName: "PR" }),
}));

vi.mock("@kandev/ui/button", () => ({
  Button: ({ children, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props}>{children}</button>
  ),
}));

vi.mock("@kandev/ui/dropdown-menu", () => ({
  DropdownMenu: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuItem: ({ children, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props}>{children}</button>
  ),
  DropdownMenuTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  DropdownMenuSeparator: () => null,
  DropdownMenuSub: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  DropdownMenuSubTrigger: ({
    children,
    ...props
  }: React.ButtonHTMLAttributes<HTMLButtonElement>) => <button {...props}>{children}</button>,
  DropdownMenuSubContent: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

afterEach(cleanup);

const baseProps = {
  sessionId: "session-1",
  isGitLoading: false,
  uncommittedCount: 2,
  baseBranch: "main",
  onCommitClick: vi.fn(),
  onPRClick: vi.fn(),
  onPull: vi.fn(),
  onPush: vi.fn(),
  onRebase: vi.fn(),
  onMerge: vi.fn(),
  canCreatePR: true,
};

describe("GitActionsDropdown remote safety", () => {
  it("aggregates uncommitted files across repository-scoped statuses", () => {
    const result = computeMobileGitStats(
      [
        {
          status: {
            files: {
              "frontend.tsx": { additions: 2, deletions: 1 },
            },
          } as never,
        },
        {
          status: {
            files: {
              "backend.go": { additions: 4, deletions: 3 },
            },
          } as never,
        },
      ],
      undefined,
      [{ insertions: 5, deletions: 2 }],
    );

    expect(result).toMatchObject({
      additions: 6,
      deletions: 4,
      count: 2,
      commitAdditions: 5,
      commitDeletions: 2,
    });
  });

  it("keeps Commit available while disabling Pull and both Push variants", () => {
    render(<GitActionsDropdown {...baseProps} pushDisabled pullDisabled />);

    expect(screen.getByRole("button", { name: /Commit/ })).not.toHaveProperty("disabled", true);
    expect(screen.getByRole("button", { name: /Pull/ })).toHaveProperty("disabled", true);
    expect(
      screen
        .getAllByRole("button", { name: /^Push$/i })
        .every((button) => button.hasAttribute("disabled")),
    ).toBe(true);
    expect(screen.getByRole("button", { name: /Force Push/ })).toHaveProperty("disabled", true);
  });

  it("keeps Pull available for a provider-ahead checkout when only Push is blocked", () => {
    render(<GitActionsDropdown {...baseProps} pushDisabled />);

    expect(screen.getByRole("button", { name: /Pull/ })).not.toHaveProperty("disabled", true);
    expect(
      screen
        .getAllByRole("button", { name: /^Push$/i })
        .every((button) => button.hasAttribute("disabled")),
    ).toBe(true);
  });

  it("disables comparison actions when the delivered comparison target is unavailable", () => {
    render(<GitActionsDropdown {...baseProps} comparisonDisabled />);

    expect(screen.getByRole("button", { name: /Rebase/ })).toHaveProperty("disabled", true);
    expect(screen.getByRole("button", { name: /Merge/ })).toHaveProperty("disabled", true);
  });

  it("disables Create PR when the shared model has no complete source evidence", () => {
    render(<GitActionsDropdown {...baseProps} canCreatePR={false} />);

    expect(screen.getByRole("button", { name: /Create PR/ })).toHaveProperty("disabled", true);
  });

  it("offers scoped contribution version choices without the generic push submenu", () => {
    const onReplaceContribution = vi.fn();
    const onUseContribution = vi.fn();
    const onViewPRVersion = vi.fn();
    render(
      <GitActionsDropdown
        {...baseProps}
        showContributionResolution
        onReplaceContribution={onReplaceContribution}
        onUseContribution={onUseContribution}
        onViewPRVersion={onViewPRVersion}
        prNumber={42}
      />,
    );

    expect(screen.getByRole("button", { name: /Commit/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /Replace PR branch/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /Use PR version/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /PR #42 version/ })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Force Push/ })).toBeNull();
    expect(screen.queryByRole("button", { name: /^Pull$/i })).toBeNull();

    screen.getByRole("button", { name: /Replace PR branch/ }).click();
    screen.getByRole("button", { name: /Use PR version/ }).click();
    screen.getByRole("button", { name: /PR #42 version/ }).click();
    expect(onReplaceContribution).toHaveBeenCalledOnce();
    expect(onUseContribution).toHaveBeenCalledOnce();
    expect(onViewPRVersion).toHaveBeenCalledOnce();
  });
});
