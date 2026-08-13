import { createElement, type ReactNode } from "react";
import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import { repositoryId, sessionId, taskId, workspaceId, type Repository } from "@/lib/types/http";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import type { AzureDevOpsTaskPullRequest } from "@/lib/types/azure-devops";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";

const loaderMocks = vi.hoisted(() => ({
  github: vi.fn(),
  gitlab: vi.fn(),
  azure: vi.fn(),
}));

vi.mock("@/hooks/domains/github/use-task-pr", () => ({
  useTaskPR: (value: string | null) => loaderMocks.github(value),
}));
vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({
  useWorkspaceMRs: (value: string | null) => loaderMocks.gitlab(value),
}));
vi.mock("@/hooks/domains/azure-devops/use-azure-devops-task-pull-requests", () => ({
  useAzureDevOpsTaskPullRequests: (workspace: string | null, task: string | null) =>
    loaderMocks.azure(workspace, task),
}));

import {
  useExternalVcsFileLink,
  useExternalVcsFileLinkHydration,
} from "./use-external-vcs-file-link";

const WORKSPACE_ID = workspaceId("workspace-1");
const TASK_ID = taskId("task-1");
const SESSION_ID = sessionId("session-1");
const GITHUB_REPOSITORY_ID = "repo-github";
const GITLAB_REPOSITORY_ID = "repo-gitlab";
const FIRST_BRANCH = "feature/one";
const SECOND_BRANCH = "feature/two";

function repository(overrides: Partial<Repository> = {}): Repository {
  return {
    id: repositoryId(GITHUB_REPOSITORY_ID),
    workspace_id: WORKSPACE_ID,
    name: "web",
    source_type: "remote",
    local_path: "",
    provider: "github",
    provider_repo_id: "provider-repo-1",
    provider_host: "https://github.com",
    provider_owner: "acme",
    provider_name: "web",
    remote_url: "https://github.com/acme/web.git",
    default_branch: "main",
    worktree_branch_prefix: "kandev/",
    pull_before_worktree: false,
    setup_script: "",
    cleanup_script: "",
    dev_script: "",
    copy_files: "",
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

function githubPR(overrides: Partial<TaskPR> = {}): TaskPR {
  return {
    id: "pr-link-1",
    task_id: TASK_ID,
    repository_id: GITHUB_REPOSITORY_ID,
    owner: "acme",
    repo: "web",
    pr_number: 42,
    pr_url: "https://github.com/acme/web/pull/42",
    pr_title: "Share links",
    head_branch: "feature/share",
    base_branch: "main",
    head_host: "github.com",
    head_owner: "acme",
    head_repo: "web",
    head_repo_id: 11,
    base_host: "github.com",
    base_owner: "acme",
    base_repo: "web",
    base_repo_id: 11,
    author_login: "ada",
    state: "open",
    review_state: "",
    checks_state: "",
    mergeable_state: "",
    review_count: 0,
    pending_review_count: 0,
    comment_count: 0,
    unresolved_review_threads: 0,
    checks_total: 0,
    checks_passing: 0,
    additions: 0,
    deletions: 0,
    created_at: "",
    merged_at: null,
    closed_at: null,
    last_synced_at: null,
    updated_at: "",
    ...overrides,
  };
}

function gitlabMR(overrides: Partial<TaskMR> = {}): TaskMR {
  return {
    id: "mr-link-1",
    task_id: TASK_ID,
    repository_id: GITLAB_REPOSITORY_ID,
    host: "https://gitlab.example.com",
    project_path: "platform/api",
    mr_iid: 7,
    mr_url: "https://gitlab.example.com/platform/api/-/merge_requests/7",
    mr_title: "Share links",
    head_branch: "feature/gitlab",
    base_branch: "trunk",
    source_host: "https://gitlab.example.com",
    source_project_path: "platform/api",
    source_project_id: 11,
    target_host: "https://gitlab.example.com",
    target_project_path: "platform/api",
    target_project_id: 11,
    author_username: "ada",
    state: "open",
    approval_state: "",
    pipeline_state: "",
    merge_status: "",
    draft: false,
    approval_count: 0,
    required_approvals: 0,
    pipeline_jobs_total: 0,
    pipeline_jobs_pass: 0,
    reviewer_count: 0,
    unapproved_reviewers: 0,
    unresolved_discussions: 0,
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

type InitialOptions = {
  repositories?: Repository[];
  taskRepositories?: Array<{
    id: string;
    repository_id: string;
    base_branch: string;
    checkout_branch?: string;
    position: number;
  }>;
  prs?: TaskPR[];
  mrs?: TaskMR[];
  azurePRs?: AzureDevOpsTaskPullRequest[];
  sessionRepositoryId?: string;
  sessionWorktrees?: Array<{
    id: string;
    worktree_id: string;
    repository_id: ReturnType<typeof repositoryId>;
    position: number;
    worktree_path: string;
    worktree_branch: string;
    session_id: ReturnType<typeof sessionId>;
  }>;
  gitStatus?: Record<string, unknown>;
};

function repeatedTaskRepositories() {
  return [
    {
      id: "task-repo-one",
      repository_id: GITHUB_REPOSITORY_ID,
      base_branch: "main",
      checkout_branch: FIRST_BRANCH,
      position: 0,
    },
    {
      id: "task-repo-two",
      repository_id: GITHUB_REPOSITORY_ID,
      base_branch: "release",
      checkout_branch: SECOND_BRANCH,
      position: 1,
    },
  ];
}

function repeatedSessionWorktrees(): NonNullable<InitialOptions["sessionWorktrees"]> {
  return [
    {
      id: "session-worktree-one",
      session_id: SESSION_ID,
      worktree_id: "worktree-one",
      repository_id: repositoryId(GITHUB_REPOSITORY_ID),
      position: 0,
      worktree_path: "/tmp/web-feature-one",
      worktree_branch: FIRST_BRANCH,
    },
    {
      id: "session-worktree-two",
      session_id: SESSION_ID,
      worktree_id: "worktree-two",
      repository_id: repositoryId(GITHUB_REPOSITORY_ID),
      position: 1,
      worktree_path: "/tmp/web-feature-two",
      worktree_branch: SECOND_BRANCH,
    },
  ];
}

function wrapper(options: InitialOptions = {}) {
  const repositories = options.repositories ?? [repository()];
  const taskRepositories = options.taskRepositories ?? [
    {
      id: "task-repo-1",
      repository_id: GITHUB_REPOSITORY_ID,
      base_branch: "main",
      position: 0,
    },
  ];
  const initialState = {
    ...defaultState,
    tasks: { ...defaultState.tasks, activeTaskId: TASK_ID, activeSessionId: SESSION_ID },
    kanban: {
      ...defaultState.kanban,
      tasks: [
        {
          id: TASK_ID,
          workflowId: "wf-1",
          workflowStepId: "step-1",
          title: "External links",
          position: 0,
          repositories: taskRepositories,
        },
      ],
    },
    repositories: {
      ...defaultState.repositories,
      itemsByWorkspaceId: { [WORKSPACE_ID]: repositories },
    },
    taskSessions: {
      items: {
        [SESSION_ID]: {
          id: SESSION_ID,
          task_id: TASK_ID,
          repository_id: options.sessionRepositoryId
            ? repositoryId(options.sessionRepositoryId)
            : undefined,
          worktrees: options.sessionWorktrees,
          state: "RUNNING" as const,
          started_at: "",
          updated_at: "",
        },
      },
    },
    taskPRs: { ...defaultState.taskPRs, byTaskId: { [TASK_ID]: options.prs ?? [] } },
    taskMRs: {
      ...defaultState.taskMRs,
      byWorkspaceId: { [WORKSPACE_ID]: { [TASK_ID]: options.mrs ?? [] } },
    },
    azureDevOpsTaskPullRequests: {
      ...defaultState.azureDevOpsTaskPullRequests,
      byTaskId: { [TASK_ID]: options.azurePRs ?? [] },
    },
    gitStatus: {
      ...defaultState.gitStatus,
      byEnvironmentRepo: options.gitStatus
        ? { [SESSION_ID]: options.gitStatus as Record<string, GitStatusEntry> }
        : defaultState.gitStatus.byEnvironmentRepo,
    },
  };
  return ({ children }: { children: ReactNode }) =>
    createElement(StateProvider, { initialState, children });
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("useExternalVcsFileLink repository and revision resolution", () => {
  it("uses an explicit repository id and its published review branch", () => {
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/app.ts",
          sessionId: SESSION_ID,
          repositoryId: GITHUB_REPOSITORY_ID,
        }),
      { wrapper: wrapper({ prs: [githubPR()] }) },
    );

    expect(result.current).toMatchObject({
      provider: "github",
      revision: "feature/share",
      url: "https://github.com/acme/web/blob/feature%2Fshare/src/app.ts",
    });
  });

  it("resolves a multi-repository file by repository name without crossing providers", () => {
    const gitlab = repository({
      id: repositoryId(GITLAB_REPOSITORY_ID),
      name: "api",
      provider: "gitlab",
      provider_repo_id: "gitlab-api",
      provider_host: "https://gitlab.example.com",
      provider_owner: "platform",
      provider_name: "api",
      remote_url: "https://gitlab.example.com/platform/api.git",
      default_branch: "trunk",
    });
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "cmd/main.go",
          sessionId: SESSION_ID,
          repositoryName: "api",
        }),
      {
        wrapper: wrapper({
          repositories: [repository(), gitlab],
          taskRepositories: [
            {
              id: "task-repo-web",
              repository_id: GITHUB_REPOSITORY_ID,
              base_branch: "main",
              position: 0,
            },
            {
              id: "task-repo-api",
              repository_id: GITLAB_REPOSITORY_ID,
              base_branch: "trunk",
              position: 1,
            },
          ],
          prs: [githubPR()],
          mrs: [gitlabMR()],
        }),
      },
    );

    expect(result.current).toMatchObject({
      provider: "gitlab",
      revision: "feature/gitlab",
      url: "https://gitlab.example.com/platform/api/-/blob/feature%2Fgitlab/cmd/main.go",
    });
  });

  it("uses the sole legacy session repository and falls back to its task base branch", () => {
    const { result } = renderHook(
      () => useExternalVcsFileLink({ filePath: "README.md", sessionId: SESSION_ID }),
      { wrapper: wrapper({ sessionRepositoryId: GITHUB_REPOSITORY_ID }) },
    );

    expect(result.current).toMatchObject({ provider: "github", revision: "main" });
  });

  it("uses the sole linked task repository when commit detail has no session identity", () => {
    const { result } = renderHook(
      () => useExternalVcsFileLink({ filePath: "src/commit-detail.ts" }),
      { wrapper: wrapper() },
    );

    expect(result.current).toMatchObject({
      provider: "github",
      revision: "main",
      url: "https://github.com/acme/web/blob/main/src/commit-detail.ts",
    });
  });
});

describe("useExternalVcsFileLink GitHub published revisions", () => {
  it("keeps the published branch when fork provenance is unavailable", () => {
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/app.ts",
          sessionId: SESSION_ID,
          repositoryId: GITHUB_REPOSITORY_ID,
        }),
      {
        wrapper: wrapper({
          prs: [githubPR({ head_branch: "contributor:feature/share", pr_number: 42 })],
        }),
      },
    );

    expect(result.current).toMatchObject({
      revision: "contributor:feature/share",
      url: "https://github.com/acme/web/blob/contributor%3Afeature%2Fshare/src/app.ts",
    });
  });
});

describe("useExternalVcsFileLink repeated repository matching", () => {
  it("fails closed when repeated linked changes have no exact action identity", () => {
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/repeated.ts",
          sessionId: SESSION_ID,
          repositoryId: GITHUB_REPOSITORY_ID,
          repositoryName: "web-feature-two",
        }),
      {
        wrapper: wrapper({
          taskRepositories: repeatedTaskRepositories(),
          prs: [
            githubPR({ id: "pr-one", head_branch: FIRST_BRANCH, base_branch: "main" }),
            githubPR({
              id: "pr-two",
              pr_number: 43,
              head_branch: SECOND_BRANCH,
              base_branch: "release",
            }),
          ],
          sessionWorktrees: repeatedSessionWorktrees(),
        }),
      },
    );

    expect(result.current).toBeNull();
  });

  it("does not reuse a sibling worktree's published branch", () => {
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/repeated.ts",
          sessionId: SESSION_ID,
          repositoryId: GITHUB_REPOSITORY_ID,
          repositoryName: "web-feature-two",
        }),
      {
        wrapper: wrapper({
          taskRepositories: repeatedTaskRepositories(),
          prs: [githubPR({ head_branch: FIRST_BRANCH })],
          sessionWorktrees: repeatedSessionWorktrees(),
        }),
      },
    );

    expect(result.current?.revision).toBe(FIRST_BRANCH);
  });

  it("fails closed for an ambiguous production-shaped named worktree", () => {
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/editor.ts",
          sessionId: SESSION_ID,
          repositoryName: "web-feature-two",
        }),
      {
        wrapper: wrapper({
          taskRepositories: repeatedTaskRepositories(),
          prs: [
            githubPR({ id: "pr-one", head_branch: FIRST_BRANCH, base_branch: "main" }),
            githubPR({
              id: "pr-two",
              pr_number: 43,
              head_branch: SECOND_BRANCH,
              base_branch: "release",
            }),
          ],
          sessionWorktrees: repeatedSessionWorktrees(),
        }),
      },
    );

    expect(result.current).toBeNull();
  });
});

describe("useExternalVcsFileLink ambiguity and legacy identity", () => {
  it("fails closed when repeated repository rows cannot be disambiguated", () => {
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/ambiguous.ts",
          sessionId: SESSION_ID,
          repositoryId: GITHUB_REPOSITORY_ID,
        }),
      {
        wrapper: wrapper({
          taskRepositories: [
            { id: "one", repository_id: GITHUB_REPOSITORY_ID, base_branch: "main", position: 0 },
            {
              id: "two",
              repository_id: GITHUB_REPOSITORY_ID,
              base_branch: "release",
              position: 1,
            },
          ],
        }),
      },
    );

    expect(result.current).toBeNull();
  });

  it("fails closed when a repository name is ambiguous", () => {
    const duplicate = repository({
      id: repositoryId("repo-other"),
      provider_owner: "other",
      remote_url: "https://github.com/other/web.git",
    });
    const { result } = renderHook(
      () => useExternalVcsFileLink({ filePath: "src/app.ts", repositoryName: "web" }),
      {
        wrapper: wrapper({
          repositories: [repository(), duplicate],
          taskRepositories: [
            { id: "one", repository_id: GITHUB_REPOSITORY_ID, base_branch: "main", position: 0 },
            { id: "two", repository_id: "repo-other", base_branch: "main", position: 1 },
          ],
        }),
      },
    );

    expect(result.current).toBeNull();
  });
});

describe("useExternalVcsFileLink named worktree ambiguity", () => {
  it("fails closed when a named session worktree is ambiguous across repositories", () => {
    const otherRepositoryId = "repo-other";
    const duplicate = repository({
      id: repositoryId(otherRepositoryId),
      name: "api",
      provider_owner: "other",
      provider_name: "api",
      remote_url: "https://github.com/other/api.git",
    });
    const sharedPath = "/tmp/shared-feature";
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/ambiguous.ts",
          sessionId: SESSION_ID,
          repositoryName: "shared-feature",
        }),
      {
        wrapper: wrapper({
          repositories: [repository(), duplicate],
          taskRepositories: [
            { id: "one", repository_id: GITHUB_REPOSITORY_ID, base_branch: "main", position: 0 },
            { id: "two", repository_id: otherRepositoryId, base_branch: "main", position: 1 },
          ],
          sessionWorktrees: [
            {
              id: "worktree-one",
              worktree_id: "worktree-one",
              repository_id: repositoryId(GITHUB_REPOSITORY_ID),
              position: 0,
              worktree_path: sharedPath,
              worktree_branch: FIRST_BRANCH,
              session_id: SESSION_ID,
            },
            {
              id: "worktree-two",
              worktree_id: "worktree-two",
              repository_id: repositoryId(otherRepositoryId),
              position: 1,
              worktree_path: sharedPath,
              worktree_branch: SECOND_BRANCH,
              session_id: SESSION_ID,
            },
          ],
        }),
      },
    );

    expect(result.current).toBeNull();
  });
});

describe("useExternalVcsFileLink legacy provider identity", () => {
  it("fails closed when a linked GitHub association has no exact source identity", () => {
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/legacy.ts",
          sessionId: SESSION_ID,
          repositoryId: GITHUB_REPOSITORY_ID,
        }),
      {
        wrapper: wrapper({
          prs: [githubPR({
            repository_id: undefined,
            head_host: undefined,
            head_owner: undefined,
            head_repo: undefined,
            head_repo_id: undefined,
            base_host: undefined,
            base_owner: undefined,
            base_repo: undefined,
            base_repo_id: undefined,
          })],
        }),
      },
    );

    expect(result.current).toBeNull();
  });
});

describe("useExternalVcsFileLink exact provider sides", () => {
  it("uses a linked GitHub source repository for head-side content and its canonical base for deletes", () => {
    const pr = githubPR({
      head_host: "github.com",
      head_owner: "contributor",
      head_repo: "web-fork",
      head_repo_id: 11,
      base_host: "github.com",
      base_owner: "acme",
      base_repo: "web",
      base_repo_id: 22,
    });
    const status = {
      branch: FIRST_BRANCH,
      action_head: {
        observation_state: "present",
        identity: {
          repository: {
            provider: "github",
            host: "github.com",
            repository_path: "contributor/web-fork",
            provider_repository_id: "11",
          },
          ref: "feature/share",
        },
      },
      files: {},
    };
    const source = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/new.ts",
          status: "added",
          sessionId: SESSION_ID,
          repositoryId: GITHUB_REPOSITORY_ID,
        }),
      { wrapper: wrapper({ prs: [pr], gitStatus: { web: status } }) },
    );
    expect(source.result.current).toMatchObject({
      url: "https://github.com/contributor/web-fork/blob/feature%2Fshare/src/new.ts",
      revision: "feature/share",
    });
    source.unmount();

    const deleted = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/removed.ts",
          status: "deleted",
          sessionId: SESSION_ID,
          repositoryId: GITHUB_REPOSITORY_ID,
        }),
      { wrapper: wrapper({ prs: [pr], gitStatus: { web: status } }) },
    );
    expect(deleted.result.current).toMatchObject({
      url: "https://github.com/acme/web/blob/main/src/removed.ts",
      revision: "main",
    });
  });

  it("fails closed when a complete linked source does not match the action-head identity", () => {
    const pr = githubPR({
      head_host: "github.com",
      head_owner: "contributor",
      head_repo: "web-fork",
      head_repo_id: 11,
      base_host: "github.com",
      base_owner: "acme",
      base_repo: "web",
      base_repo_id: 22,
    });
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/new.ts",
          status: "modified",
          sessionId: SESSION_ID,
          repositoryId: GITHUB_REPOSITORY_ID,
        }),
      {
        wrapper: wrapper({
          prs: [pr],
          gitStatus: {
            web: {
              branch: FIRST_BRANCH,
              action_head: {
                observation_state: "present",
                identity: {
                  repository: {
                    provider: "github",
                    host: "github.com",
                    repository_path: "someone-else/web-fork",
                  },
                  ref: "feature/share",
                },
              },
              files: {},
            },
          },
        }),
      },
    );
    expect(result.current).toBeNull();
  });

  it("uses complete comparison evidence for an unlinked base-side file", () => {
    const { result } = renderHook(
      () =>
        useExternalVcsFileLink({
          filePath: "src/removed.ts",
          status: "deleted",
          sessionId: SESSION_ID,
          repositoryId: GITHUB_REPOSITORY_ID,
        }),
      {
        wrapper: wrapper({
          prs: [],
          gitStatus: {
            web: {
              branch: FIRST_BRANCH,
              comparison: {
                resolution_state: "resolved",
                context_generation: "generation-1",
                resolved_ref: "release/literal/ref",
                base_commit: "base-commit",
                ahead: 1,
                behind: 0,
                additions: 1,
                deletions: 1,
                target: {
                  repository: {
                    provider: "github",
                    host: "github.com",
                    repository_path: "acme/web",
                  },
                  ref: "release/literal/ref",
                },
              },
              files: {},
            },
          },
        }),
      },
    );
    expect(result.current).toMatchObject({
      url: "https://github.com/acme/web/blob/release%2Fliteral%2Fref/src/removed.ts",
      revision: "release/literal/ref",
    });
  });

  it("matches GitLab source and canonical base identities exactly", () => {
    const gitlab = repository({
      id: repositoryId(GITLAB_REPOSITORY_ID),
      name: "api",
      provider: "gitlab",
      provider_repo_id: "gitlab-api",
      provider_host: "https://gitlab.example.com",
      provider_owner: "platform",
      provider_name: "api",
      remote_url: "https://gitlab.example.com/platform/api.git",
    });
    const mr = gitlabMR({
      source_host: "https://gitlab.example.com",
      source_project_path: "fork/api",
      source_project_id: 12,
      target_host: "https://gitlab.example.com",
      target_project_path: "platform/api",
      target_project_id: 11,
    });
    const { result } = renderHook(
      () => useExternalVcsFileLink({ filePath: "src/new.ts", status: "untracked", repositoryId: GITLAB_REPOSITORY_ID }),
      {
        wrapper: wrapper({
          repositories: [gitlab],
          taskRepositories: [{ id: "task-repo-gitlab", repository_id: GITLAB_REPOSITORY_ID, base_branch: "trunk", position: 0 }],
          mrs: [mr],
        }),
      },
    );
    expect(result.current).toMatchObject({
      url: "https://gitlab.example.com/fork/api/-/blob/feature%2Fgitlab/src/new.ts",
      revision: "feature/gitlab",
    });
  });

  it("normalizes Azure sourceOrganizationUrl to the action-head identity shape", () => {
    const azure = repository({
      id: repositoryId("repo-azure"),
      name: "api",
      provider: "azure_devops",
      provider_repo_id: "azure-api",
      provider_host: "https://dev.azure.com/acme",
      provider_owner: "Platform",
      provider_name: "api",
      remote_url: "https://dev.azure.com/acme/Platform/_git/api",
    });
    const pullRequest: AzureDevOpsTaskPullRequest = {
      id: "azure-pr-1",
      taskId: TASK_ID,
      repositoryId: azure.id,
      organizationUrl: "https://dev.azure.com/acme",
      projectId: "project-1",
      azureRepositoryId: "repo-1",
      sourceOrganizationUrl: "https://dev.azure.com/fork-org",
      sourceProjectId: "project-fork",
      sourceProjectName: "Fork Project",
      sourceRepositoryId: "repo-fork",
      sourceRepositoryName: "api",
      targetOrganizationUrl: "https://dev.azure.com/acme",
      targetProjectId: "project-1",
      targetProjectName: "Platform",
      targetRepositoryId: "repo-1",
      targetRepositoryName: "api",
      pullRequestId: 7,
      pullRequestUrl: "https://dev.azure.com/acme/Platform/_git/api/pullrequest/7",
      title: "Azure link",
      sourceBranch: "feature/azure",
      targetBranch: "main",
      authorId: "ada",
      authorName: "Ada",
      status: "active",
      isDraft: false,
      createdAt: "",
      updatedAt: "",
    };
    const { result } = renderHook(
      () => useExternalVcsFileLink({ filePath: "src/new.ts", status: "added", repositoryId: azure.id }),
      {
        wrapper: wrapper({
          repositories: [azure],
          taskRepositories: [{ id: "task-repo-azure", repository_id: azure.id, base_branch: "main", position: 0 }],
          azurePRs: [pullRequest],
        }),
      },
    );
    expect(result.current).toMatchObject({
      url: "https://dev.azure.com/fork-org/Fork%20Project/_git/api?path=%2Fsrc%2Fnew.ts&version=GBfeature%2Fazure",
      revision: "feature/azure",
    });
  });
});

describe("useExternalVcsFileLinkHydration", () => {
  it("enables only providers attached to the task", () => {
    const gitlab = repository({
      id: repositoryId(GITLAB_REPOSITORY_ID),
      provider: "gitlab",
      provider_host: "https://gitlab.example.com",
      provider_owner: "platform",
      provider_name: "api",
      remote_url: "https://gitlab.example.com/platform/api.git",
    });
    const azure = repository({
      id: repositoryId("repo-azure"),
      provider: "azure_devops",
      provider_host: "",
      provider_owner: "Platform",
      provider_name: "api",
      remote_url: "https://dev.azure.com/acme/Platform/_git/api",
    });
    const task = {
      id: TASK_ID,
      workspace_id: WORKSPACE_ID,
      repositories: [
        {
          id: "one",
          task_id: TASK_ID,
          repository_id: repositoryId(GITHUB_REPOSITORY_ID),
          base_branch: "main",
          position: 0,
          created_at: "",
          updated_at: "",
        },
        {
          id: "two",
          task_id: TASK_ID,
          repository_id: repositoryId("repo-azure"),
          base_branch: "main",
          position: 1,
          created_at: "",
          updated_at: "",
        },
      ],
    };

    const { rerender } = renderHook(
      () => useExternalVcsFileLinkHydration(task, [repository(), gitlab, azure]),
      { wrapper: wrapper() },
    );
    rerender();

    expect(loaderMocks.github).toHaveBeenLastCalledWith(TASK_ID);
    expect(loaderMocks.gitlab).toHaveBeenLastCalledWith(null);
    expect(loaderMocks.azure).toHaveBeenLastCalledWith(WORKSPACE_ID, TASK_ID);
  });
});
