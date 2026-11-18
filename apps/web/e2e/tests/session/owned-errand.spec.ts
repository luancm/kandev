import { test } from "../../fixtures/test-base";
import {
  exerciseOwnedErrandAutomaticPermission,
  exerciseOwnedErrandContinuation,
  exerciseOwnedErrandDeepLink,
  exerciseOwnedErrandView,
} from "../../helpers/owned-errand";

test("desktop opens a finished owned Errand from a direct link", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  await exerciseOwnedErrandDeepLink({ page, api, seed, backend, phone: false });
});

test("desktop opens an owned Errand and returns to its parent", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  await exerciseOwnedErrandView({ page, api, seed, backend, phone: false });
});

test("desktop stops an owned Errand and retains its transcript", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  await exerciseOwnedErrandView({ page, api, seed, backend, phone: false, stop: true });
});

test("desktop approves a live Errand tool action", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  await exerciseOwnedErrandView({ page, api, seed, backend, phone: false, permission: true });
});

test("desktop retains a tool action approved by the Errand's saved policy", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  await exerciseOwnedErrandAutomaticPermission({ page, api, seed, backend, phone: false });
});

test("desktop continues a completed owned Errand and retains its original parent result", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  await exerciseOwnedErrandContinuation({ page, api, seed, backend });
});

test("desktop sends a follow-up from a finished owned Errand", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  await exerciseOwnedErrandContinuation({ page, api, seed, backend, browser: true });
});
