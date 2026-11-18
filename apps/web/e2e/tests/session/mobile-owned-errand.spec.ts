import { test } from "../../fixtures/test-base";
import { exerciseOwnedErrandView } from "../../helpers/owned-errand";

test("phone opens an owned Errand from the drawer and returns to its parent", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}) => {
  test.setTimeout(120_000);
  await exerciseOwnedErrandView({ page, api, seed, backend, phone: true });
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
