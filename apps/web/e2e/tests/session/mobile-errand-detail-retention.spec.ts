import fs from "node:fs";
import { test, expect } from "../../fixtures/test-base";
import { exerciseErrandHistoryBaseline } from "../../helpers/errand-history-baseline";

test("releases full child caches after repeated Open and Back", async ({
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
    finishedCount: 100,
    mobile: true,
  });
  const sample = JSON.parse(
    fs.readFileSync(testInfo.outputPath("errand-history-sample.json"), "utf8"),
  );
  expect(sample.measurements.retainedChildDetailAfterBack).toEqual({
    sessionRows: 0,
    transcripts: 0,
    turnCaches: 0,
  });
});
