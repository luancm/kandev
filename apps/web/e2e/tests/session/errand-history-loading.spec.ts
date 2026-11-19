import { test } from "../../fixtures/test-base";
import { exerciseHistoryAcceptance } from "../../helpers/errand-history-acceptance";

test("desktop history failure and revision changes preserve explicit loading and live controls", async ({
  testPage: page,
  apiClient: api,
  seedData: seed,
  backend,
}, testInfo) => {
  await exerciseHistoryAcceptance({ page, api, seed, backend, testInfo, mobile: false });
});
