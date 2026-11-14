import { test } from "../../fixtures/test-base";
import { exerciseErrandHistoryBaseline } from "../../helpers/errand-history-baseline";

for (const finishedCount of [0, 100, 1_000]) {
  test(`phone history baseline: ${finishedCount} finished Errands`, async ({
    testPage: page,
    browser,
    apiClient: api,
    seedData: seed,
    backend,
  }, testInfo) => {
    await exerciseErrandHistoryBaseline({
      page,
      browser,
      api,
      seed,
      backend,
      testInfo,
      finishedCount,
      mobile: true,
    });
  });
}
