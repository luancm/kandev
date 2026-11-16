import { test } from "../../fixtures/test-base";
import { exerciseOrdinaryMainComparison } from "../../helpers/ordinary-main-comparison";

test("ordinary page main comparison", async ({
  testPage: page,
  browser,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  await exerciseOrdinaryMainComparison({
    page,
    browser,
    api,
    seed,
    backend,
    testInfo,
    mobile: false,
  });
});
