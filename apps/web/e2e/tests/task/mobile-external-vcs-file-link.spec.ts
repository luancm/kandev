import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import type { BackendContext } from "../../fixtures/backend";
import { test, expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import {
  configureWritableForkRemote,
  GitHelper,
  makeGitEnv,
} from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

const MOBILE_FILE = "mobile-external-link.ts";
const MOBILE_ADDED = "mobile-added.ts";
const MOBILE_DELETED = "mobile-deleted.ts";
const MOBILE_RENAMED = "mobile-renamed.ts";
const MOBILE_RENAMED_OLD = "mobile-old-renamed.ts";
const MOBILE_REMOTE = "https://github.com/testorg/mobile-external-links.git";
const MOBILE_PR = 91;
function initializeMobileRepository(backend: BackendContext): string {
  const repositoryPath = path.join(backend.tmpDir, "repos", `mobile-link-${Date.now()}`);
  fs.mkdirSync(repositoryPath, { recursive: true });
  const gitEnvironment = makeGitEnv(backend.tmpDir);
  execFileSync("git", ["init", "-b", "main"], { cwd: repositoryPath, env: gitEnvironment });
  fs.writeFileSync(path.join(repositoryPath, MOBILE_FILE), "export const mobile = true;\n");
  execFileSync("git", ["add", "-A"], { cwd: repositoryPath, env: gitEnvironment });
  execFileSync("git", ["commit", "-m", "seed mobile provider repository"], {
    cwd: repositoryPath,
    env: gitEnvironment,
  });
  return repositoryPath;
}

async function createMobileRepository(
  apiClient: ApiClient,
  workspaceId: string,
  repositoryPath: string,
) {
  const options = {
    name: "mobile-external-links",
    provider: "github",
    provider_host: "https://github.com",
    provider_owner: "testorg",
    provider_name: "mobile-external-links",
  };
  return apiClient.createRepository(workspaceId, repositoryPath, "main", options);
}

function trustedMobileTaskRepository() {
  return {
    remote_url: MOBILE_REMOTE,
    provider: "github",
    provider_owner: "testorg",
    provider_name: "mobile-external-links",
    base_branch: "main",
  };
}

test.describe("Mobile external VCS file link", () => {
  test.describe.configure({ timeout: 120_000 });

  test("opens the provider file with a touch-sized action and no horizontal overflow", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    const repositoryPath = initializeMobileRepository(backend);
    const repository = await createMobileRepository(apiClient, seedData.workspaceId, repositoryPath);
    await apiClient.mockGitHubSetWorkspaceConnection(seedData.workspaceId, {
      source: "legacy_shared",
      status: "active",
    });
    await apiClient.mockGitHubReset();
    await apiClient.mockGitHubSetUser("reviewer");
    await apiClient.mockGitHubAddPRs([
      {
        number: MOBILE_PR,
        title: "Mobile fork external link",
        state: "open",
        head_branch: "feature/mobile-links",
        base_branch: "main",
        author_login: "reviewer",
        repo_owner: "testorg",
        repo_name: "mobile-external-links",
      },
    ]);
    await apiClient.mockGitHubAddPRFiles("testorg", "mobile-external-links", MOBILE_PR, [
      {
        filename: MOBILE_FILE,
        status: "modified",
        additions: 1,
        deletions: 1,
        patch: "@@ -1 +1 @@\n-export const mobile = false;\n+export const mobile = true;",
      },
      {
        filename: MOBILE_ADDED,
        status: "added",
        additions: 1,
        deletions: 0,
        patch: "@@ -0,0 +1 @@\n+export const added = true;",
      },
      {
        filename: MOBILE_DELETED,
        status: "deleted",
        additions: 0,
        deletions: 1,
        patch: "@@ -1 +0,0 @@\n-export const deleted = true;",
      },
      {
        filename: MOBILE_RENAMED,
        old_path: MOBILE_RENAMED_OLD,
        status: "renamed",
        additions: 1,
        deletions: 1,
        patch: "@@ -1 +1 @@\n-export const oldName = false;\n+export const newName = true;",
      },
    ]);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile external file link",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repositories: [trustedMobileTaskRepository()],
      },
    );
    await apiClient.mockGitHubAssociateTaskPR({
      task_id: task.id,
      workspace_id: seedData.workspaceId,
      repository_id: repository.id,
      owner: "testorg",
      repo: "mobile-external-links",
      pr_number: MOBILE_PR,
      pr_url: `https://github.com/testorg/mobile-external-links/pull/${MOBILE_PR}`,
      pr_title: "Mobile fork external link",
      head_branch: "feature/mobile-links",
      base_branch: "main",
      author_login: "reviewer",
      head_host: "https://github.com",
      head_owner: "mobile-reviewer",
      head_repo: "mobile-external-links",
      head_repo_id: 991,
      base_host: "https://github.com",
      base_owner: "testorg",
      base_repo: "mobile-external-links",
      base_repo_id: 992,
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle();
    const sessions = await apiClient.listTaskSessions(task.id);
    const checkout =
      sessions.sessions[0]?.worktrees?.[0]?.worktree_path ||
      sessions.sessions[0]?.worktree_path ||
      repositoryPath;
    const actionGit = new GitHelper(checkout, makeGitEnv(backend.tmpDir));
    actionGit.exec("git checkout -B feature/mobile-links");
    const writableFork = configureWritableForkRemote(
      actionGit,
      backend.tmpDir,
      "https://github.com/mobile-reviewer/mobile-external-links.git",
    );
    expect(writableFork.branch).toBe("feature/mobile-links");
    await testPage.getByRole("button", { name: "Changes" }).tap();
    await expect(testPage.getByTestId("mobile-changes-panel")).toBeVisible();
    await testPage
      .getByTestId("mobile-changes-panel")
      .getByRole("button", { name: "Review", exact: true })
      .tap();
    const review = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(review).toBeVisible();
    await expect(
      review.getByTestId("review-file-header").filter({ hasText: MOBILE_FILE }).getByRole("link"),
    ).toHaveAttribute(
      "href",
      "https://github.com/mobile-reviewer/mobile-external-links/blob/feature%2Fmobile-links/mobile-external-link.ts",
    );
    await expect(
      review.getByTestId("review-file-header").filter({ hasText: MOBILE_ADDED }).getByRole("link"),
    ).toHaveAttribute(
      "href",
      "https://github.com/mobile-reviewer/mobile-external-links/blob/feature%2Fmobile-links/mobile-added.ts",
    );
    await expect(
      review.getByTestId("review-file-header").filter({ hasText: MOBILE_DELETED }).getByRole("link"),
    ).toHaveAttribute(
      "href",
      "https://github.com/testorg/mobile-external-links/blob/main/mobile-deleted.ts",
    );
    await expect(
      review
        .getByTestId("review-file-header")
        .filter({ hasText: MOBILE_RENAMED })
        .getByRole("link"),
    ).toHaveAttribute(
      "href",
      "https://github.com/mobile-reviewer/mobile-external-links/blob/feature%2Fmobile-links/mobile-renamed.ts",
    );
    await review.getByRole("button", { name: "Close review" }).tap();
    const taskCheckout = checkout;
    fs.writeFileSync(path.join(taskCheckout, MOBILE_FILE), "export const mobile = false;\n");
    const localOld = "mobile-base-old.ts";
    const localRenamed = "mobile-base-renamed.ts";
    fs.writeFileSync(path.join(taskCheckout, localOld), "export const oldBase = true;\n");
    execFileSync("git", ["mv", localOld, localRenamed], {
      cwd: taskCheckout,
      env: makeGitEnv(backend.tmpDir),
    });
    await testPage.getByRole("button", { name: "Files" }).tap();
    const fileNode = testPage.locator(`[data-testid="file-tree-node"][data-path="${MOBILE_FILE}"]`);
    await expect(fileNode).toBeVisible({ timeout: 15_000 });
    await fileNode.tap();

    await testPage.getByRole("button", { name: "Files" }).tap();
    const renamedNode = testPage.locator(
      `[data-testid="file-tree-node"][data-path="${localRenamed}"]`,
    );
    await expect(renamedNode).toBeVisible({ timeout: 15_000 });
    await renamedNode.tap();
    await expect(testPage.getByTestId("mobile-file-viewer-panel").getByRole("link")).toHaveAttribute(
      "href",
      "https://github.com/testorg/mobile-external-links/blob/main/mobile-base-old.ts",
    );

    const viewer = testPage.getByTestId("mobile-file-viewer-panel");
    await expect(viewer).toBeVisible();
    const link = viewer.getByRole("link", { name: "Open file in GitHub" });
    const expectedURL =
      "https://github.com/mobile-reviewer/mobile-external-links/blob/feature%2Fmobile-links/mobile-external-link.ts";
    await expect(link).toHaveAttribute("href", expectedURL);

    const bounds = await link.boundingBox();
    expect(bounds).not.toBeNull();
    expect(bounds!.width).toBeGreaterThanOrEqual(44);
    expect(bounds!.height).toBeGreaterThanOrEqual(44);
    const overflow = await testPage.evaluate(() => ({
      viewportWidth: window.innerWidth,
      documentWidth: document.documentElement.scrollWidth,
    }));
    expect(overflow.documentWidth).toBeLessThanOrEqual(overflow.viewportWidth + 1);

    await testPage.context().route("https://github.com/**", async (route) => {
      await route.fulfill({ status: 200, contentType: "text/html", body: "external file" });
    });
    const popupPromise = testPage.waitForEvent("popup");
    await link.tap();
    const popup = await popupPromise;
    await popup.waitForLoadState("domcontentloaded");
    expect(popup.url()).toBe(expectedURL);
    await popup.close();
  });
});
