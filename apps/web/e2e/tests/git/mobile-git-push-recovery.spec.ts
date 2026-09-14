import { test, expect } from "../../fixtures/test-base";
import {
  createEmptyRemoteRepository,
  openTaskByID,
  remoteRef,
  removeTestRepository,
  taskWorktreeGit,
  waitForTaskWorktree,
} from "../../helpers/empty-remote-repository";
import {
  installRejectingPreReceiveHook,
  removeRejectingPreReceiveHook,
} from "./git-push-recovery-helpers";

test.describe("Mobile Git push recovery", () => {
  test.setTimeout(180_000);

  test("clears failed push feedback after a successful touch retry and reload", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const fixture = createEmptyRemoteRepository(backend.tmpDir, "recovery-mobile");
    let repositoryID = "";
    let rejectingHookPath = "";

    try {
      const repository = await apiClient.createRepository(
        seedData.workspaceId,
        fixture.localPath,
        "main",
        { name: "Git Push Recovery Mobile" },
      );
      repositoryID = repository.id;
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Git push recovery mobile",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          executor_profile_id: seedData.worktreeExecutorProfileId,
          repositories: [{ repository_id: repositoryID, base_branch: "main" }],
        },
      );

      const session = await openTaskByID(testPage, task.id);
      await session.waitForChatIdle({ timeout: 60_000 });
      const worktreePath = await waitForTaskWorktree(apiClient, task.id, repositoryID);
      const git = taskWorktreeGit(worktreePath, fixture.gitEnv);

      git.createFile("git-push-recovery-mobile.txt", "retry the push\n");
      git.stageAll();
      const taskCommit = git.commit("Add push recovery mobile fixture");
      const taskBranch = git.exec("git branch --show-current").trim();
      rejectingHookPath = installRejectingPreReceiveHook(fixture.remotePath);

      await testPage.getByRole("button", { name: "Changes" }).tap();
      const changesPanel = testPage.getByTestId("mobile-changes-panel");
      await expect(changesPanel).toBeVisible({ timeout: 15_000 });
      const commitsToggle = changesPanel.getByTestId("commits-section-collapse-toggle");
      await expect(commitsToggle).toBeVisible({ timeout: 15_000 });
      await expect
        .poll(
          async () => {
            if ((await commitsToggle.getAttribute("aria-expanded")) === "true") return true;
            await commitsToggle.tap();
            return (await commitsToggle.getAttribute("aria-expanded")) === "true";
          },
          { timeout: 15_000 },
        )
        .toBe(true);

      const pushButton = changesPanel.getByTestId("commits-repo-push");
      await expect(pushButton).toBeVisible({ timeout: 15_000 });
      await pushButton.tap();

      await testPage.getByRole("button", { name: "Chat", exact: true }).tap();
      const errorMessage = session.gitOperationErrorMessage();
      await expect(errorMessage).toBeVisible({ timeout: 30_000 });
      await expect(errorMessage.getByText("Git push failed", { exact: true })).toBeVisible();
      await expect(session.gitFixButton()).toBeVisible();

      removeRejectingPreReceiveHook(rejectingHookPath);
      rejectingHookPath = "";
      await testPage.getByRole("button", { name: "Changes" }).tap();
      await expect(changesPanel).toBeVisible({ timeout: 15_000 });
      await expect(changesPanel.getByTestId("commits-repo-push")).toBeVisible({ timeout: 30_000 });
      const retryPushButton = changesPanel.getByTestId("commits-repo-push");
      await expect(retryPushButton).toBeEnabled({ timeout: 30_000 });
      await retryPushButton.tap();

      await expect(
        testPage.getByTestId("toast-message").filter({ hasText: "Push successful" }),
      ).toBeVisible({ timeout: 30_000 });
      await expect
        .poll(() => remoteRef(git, taskBranch), {
          timeout: 30_000,
          message: "Expected the retried push to publish the task branch",
        })
        .toBe(taskCommit);
      await testPage.getByRole("button", { name: "Chat", exact: true }).tap();
      await expect(session.gitFixButton()).toHaveCount(0, { timeout: 30_000 });
      await expect(session.chat.getByText("Git push failed", { exact: true })).toHaveCount(0);

      await testPage.reload();
      await testPage.getByRole("button", { name: "Chat", exact: true }).tap();
      await session.waitForLoad();
      await expect(session.gitFixButton()).toHaveCount(0, { timeout: 15_000 });
      await expect(session.chat.getByText("Git push failed", { exact: true })).toHaveCount(0);
    } finally {
      if (rejectingHookPath) removeRejectingPreReceiveHook(rejectingHookPath);
      await apiClient.e2eReset(seedData.workspaceId, [seedData.workflowId]).catch(() => undefined);
      if (repositoryID) await removeTestRepository(apiClient, repositoryID);
      fixture.cleanup();
    }
  });
});
