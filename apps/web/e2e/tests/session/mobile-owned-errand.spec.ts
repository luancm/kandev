import { test } from "../../fixtures/test-base";
import {
  exerciseOwnedErrandAutomaticPermission,
  exerciseOwnedErrandContinuation,
  exerciseOwnedErrandDeepLink,
  exerciseOwnedErrandShellOutput,
  exerciseOwnedErrandView,
} from "../../helpers/owned-errand";

test("phone opens a finished owned Errand from a direct link", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  await exerciseOwnedErrandDeepLink({ page, api, seed, backend, phone: true });
});

test("phone reads owned command output only after tapping its disclosure", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  await exerciseOwnedErrandShellOutput({ page, api, seed, backend, phone: true });
});

test("phone opens an owned Errand from the drawer and returns to its parent", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  await exerciseOwnedErrandView({ page, api, seed, backend, phone: true });
});

test("phone sends a follow-up from a finished owned Errand", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  await exerciseOwnedErrandContinuation({ page, api, seed, backend, phone: true, browser: true });
});

test("phone stops an owned Errand from its Actions menu", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  await exerciseOwnedErrandView({ page, api, seed, backend, phone: true, stop: true });
});

test("phone approves a live Errand tool action", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  await exerciseOwnedErrandView({ page, api, seed, backend, phone: true, permission: true });
});

test("phone retains a tool action approved by the Errand's saved policy", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  await exerciseOwnedErrandAutomaticPermission({ page, api, seed, backend, phone: true });
});
