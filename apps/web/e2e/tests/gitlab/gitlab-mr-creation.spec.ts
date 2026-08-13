import { test, expect } from "../../fixtures/test-base";
import { openChangesTab } from "../git/diff-update-helpers";
import { GITLAB_PROJECT, gitLabMR } from "../../helpers/gitlab";
import {
  assertLocatorWithinViewportX,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";
import { GitLabPage } from "../../pages/gitlab-page";
import { SessionPage } from "../../pages/session-page";

test.describe("GitLab merge request creation", () => {
  test("creates an MR through the runtime and automatically persists the task link", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(180_000);
    const remoteURL = `${backend.baseUrl}/${GITLAB_PROJECT}.git`;
    await apiClient.configureGitLab(seedData.workspaceId, backend.baseUrl);
    await apiClient.configureGitLabRepositoryRemote(seedData.repositoryId, remoteURL);
    await apiClient.updateRepository(seedData.repositoryId, {
      provider: "gitlab",
      provider_host: backend.baseUrl,
      provider_owner: "platform",
      provider_name: "kandev",
    });

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Create GitLab merge request",
      seedData.agentProfileId,
      {
        description: "/e2e:diff-update-setup",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        executor_profile_id: seedData.worktreeExecutorProfileId,
      },
    );
    if (!task.session_id) throw new Error("GitLab creation task did not return a session");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(
      session.chat.getByText("diff-update-setup complete", { exact: false }),
    ).toBeVisible({
      timeout: 45_000,
    });

    await openChangesTab(testPage);
    const create = testPage.getByTestId("commits-repo-create-pr").first();
    await expect(create).toBeVisible();
    await create.click();
    const dialog = testPage.getByRole("dialog", { name: "Create merge request" });
    await expect(dialog).toBeVisible();
    await assertLocatorWithinViewportX(dialog, "desktop create MR dialog");
    await dialog
      .getByRole("textbox", { name: "Merge Request title", exact: true })
      .fill("Runtime-created GitLab MR");
    await dialog
      .getByRole("textbox", { name: "Description", exact: true })
      .fill("Created through worktree.create_pr.");
    const draft = dialog.getByLabel("Create as draft");
    await expect(draft).toBeChecked();
    await dialog.getByRole("button", { name: "Create MR", exact: true }).click();

    const gitlab = new GitLabPage(testPage);
    await expect
      .poll(async () => {
        try {
          return (await apiClient.getGitLabPushRecord(seedData.repositoryId)).args;
        } catch {
          return "";
        }
      })
      .toBe("push --set-upstream origin HEAD");
    await expect(testPage.getByTestId("mr-topbar-button")).toHaveAttribute("data-mr-iid", "100", {
      timeout: 120_000,
    });
    await apiClient.mockGitLabAddMRs(seedData.workspaceId, GITLAB_PROJECT, [
      gitLabMR(100, "Runtime-created GitLab MR", {
        source_project_path: "fork/platform/kandev",
        source_project_id: 202,
        target_project_path: GITLAB_PROJECT,
        target_project_id: 101,
      }),
    ]);
    await apiClient.linkTaskGitLabMR(seedData.workspaceId, {
      task_id: task.id,
      repository_id: seedData.repositoryId,
      mr_url: `${backend.baseUrl}/${GITLAB_PROJECT}/-/merge_requests/100`,
    });
    await gitlab.openLinkedMR(100);
    await expect(
      testPage.getByTestId("mr-detail-panel").last().getByText("Runtime-created GitLab MR"),
    ).toBeVisible();

    const taskMRsResponse = await apiClient.rawRequest(
      "GET",
      `/api/v1/gitlab/workspaces/${encodeURIComponent(seedData.workspaceId)}/task-mrs`,
    );
    expect(taskMRsResponse.ok).toBe(true);
    const taskMRs = (await taskMRsResponse.json()) as {
      task_mrs?: Record<string, Array<Record<string, unknown>>>;
    };
    const linkedMR = taskMRs.task_mrs?.[task.id]?.find((mr) => mr.mr_iid === 100);
    expect(linkedMR).toMatchObject({
      host: backend.baseUrl,
      project_path: GITLAB_PROJECT,
      source_host: backend.baseUrl,
      source_project_path: "fork/platform/kandev",
      target_host: backend.baseUrl,
      target_project_path: GITLAB_PROJECT,
      head_branch: expect.any(String),
      base_branch: "main",
    });

    await testPage.reload();
    await expect(testPage.getByTestId("mr-topbar-button")).toHaveAttribute("data-mr-iid", "100", {
      timeout: 30_000,
    });
    await assertNoDocumentHorizontalOverflow(testPage, "desktop created MR task");
  });
});
