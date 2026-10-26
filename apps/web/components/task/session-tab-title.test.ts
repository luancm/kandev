import { describe, expect, it } from "vitest";
import type { AppState } from "@/lib/state/store";
import { resolveSessionTabTitle, selectSessionTabTitle } from "./session-tab-title";

const SPARK_MODEL_ID = "gpt-5.3-codex-spark";
const SPARK_MODEL_NAME = "GPT-5.3-Codex-Spark";
const DEFAULT_MODEL_ID = "mock-default";
const PROFILE_LABEL = "GPT-5.5 (medium)";

const baseArgs = {
  agentLabel: PROFILE_LABEL,
  activeModelId: null,
  currentModelId: null,
  snapshotModel: null,
  modelOptions: [],
  configOptions: [],
};

describe("resolveSessionTabTitle", () => {
  it("prefers the user-supplied custom name over every derived title", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
        customName: "reviewer",
        activeModelId: SPARK_MODEL_ID,
        modelOptions: [{ id: SPARK_MODEL_ID, name: SPARK_MODEL_NAME }],
      }),
    ).toBe("reviewer");
  });

  it("falls back to derived titles when the custom name is empty or absent", () => {
    expect(resolveSessionTabTitle({ ...baseArgs, customName: "" })).toBe(PROFILE_LABEL);
    expect(resolveSessionTabTitle({ ...baseArgs, customName: null })).toBe(PROFILE_LABEL);
  });

  it("uses an explicitly selected model over the saved profile label", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
        activeModelId: SPARK_MODEL_ID,
        modelOptions: [{ id: SPARK_MODEL_ID, name: SPARK_MODEL_NAME }],
      }),
    ).toBe(SPARK_MODEL_NAME);
  });

  it("uses the authoritative current model over the saved profile label", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
        currentModelId: SPARK_MODEL_ID,
        modelOptions: [{ id: SPARK_MODEL_ID, name: SPARK_MODEL_NAME }],
      }),
    ).toBe(SPARK_MODEL_NAME);
  });

  it("prefers the authoritative current model over a contradictory model option", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
        currentModelId: SPARK_MODEL_ID,
        configOptions: [
          {
            type: "select",
            id: "model",
            name: "Model",
            currentValue: DEFAULT_MODEL_ID,
            options: [
              { value: DEFAULT_MODEL_ID, name: "Mock Default" },
              { value: SPARK_MODEL_ID, name: SPARK_MODEL_NAME },
            ],
          },
        ],
      }),
    ).toBe(SPARK_MODEL_NAME);
  });

  it("prefers a provider-updated model config over a stale active model", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
        activeModelId: DEFAULT_MODEL_ID,
        configOptions: [
          {
            type: "select",
            id: "model",
            name: "Model",
            currentValue: SPARK_MODEL_ID,
            options: [
              { value: DEFAULT_MODEL_ID, name: "Mock Default" },
              { value: SPARK_MODEL_ID, name: SPARK_MODEL_NAME },
            ],
          },
        ],
      }),
    ).toBe(SPARK_MODEL_NAME);
  });
});

describe("resolveSessionTabTitle fallbacks", () => {
  it("excludes non-model config selections from the title", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
        agentLabel: null,
        activeModelId: SPARK_MODEL_ID,
        configOptions: [
          {
            type: "select",
            id: "mode",
            name: "Mode",
            currentValue: "default",
            options: [
              { value: "default", name: "Default" },
              { value: "plan", name: "Plan" },
            ],
          },
          {
            type: "select",
            id: "model",
            name: "Model",
            currentValue: SPARK_MODEL_ID,
            options: [
              { value: "gpt-5.5", name: "GPT-5.5" },
              { value: SPARK_MODEL_ID, name: SPARK_MODEL_NAME },
            ],
          },
          {
            type: "select",
            id: "effort",
            name: "Effort",
            currentValue: "high",
            options: [
              { value: "medium", name: "Medium" },
              { value: "high", name: "High" },
            ],
          },
        ],
      }),
    ).toBe(SPARK_MODEL_NAME);
  });

  it("falls back to the agent label when live model state is unavailable", () => {
    expect(resolveSessionTabTitle(baseArgs)).toBe(PROFILE_LABEL);
  });

  it("falls back to currentModelId when active model id is missing", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
        agentLabel: null,
        currentModelId: SPARK_MODEL_ID,
        modelOptions: [{ id: SPARK_MODEL_ID, name: SPARK_MODEL_NAME }],
      }),
    ).toBe(SPARK_MODEL_NAME);
  });

  it("falls back to snapshot model when both agent and live model states are unavailable", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
        agentLabel: null,
        snapshotModel: SPARK_MODEL_ID,
        modelOptions: [{ id: SPARK_MODEL_ID, name: SPARK_MODEL_NAME }],
      }),
    ).toBe(SPARK_MODEL_NAME);
  });

  it("keeps the agent label over the start-time snapshot model", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
        snapshotModel: "gpt-5.5",
      }),
    ).toBe(PROFILE_LABEL);
  });
});

describe("selectSessionTabTitle errand title", () => {
  const ERRAND_ID = "errand-1";

  function state(errandsEnabled: boolean, name?: string): AppState {
    return {
      features: { errands: errandsEnabled },
      taskSessions: {
        items: {
          [ERRAND_ID]: {
            id: ERRAND_ID,
            task_id: "task-1",
            role: "errand",
            agent_profile_id: "profile-1",
            name,
          },
        },
      },
      errands: { byTaskId: { "task-1": { [ERRAND_ID]: { title: "API audit" } } } },
      sessionModels: { bySessionId: {} },
      activeModel: { bySessionId: {} },
      agentProfiles: { items: [{ id: "profile-1", label: "Codex • Deep review" }] },
    } as unknown as AppState;
  }

  it("names an errand tab after its errand while the flag is on", () => {
    expect(selectSessionTabTitle(state(true), ERRAND_ID)).toBe("Errand: API audit");
  });

  it("keeps a user-supplied session name", () => {
    expect(selectSessionTabTitle(state(true, "audit"), ERRAND_ID)).toBe("audit");
  });

  it("keeps the ordinary tab title while the flag is off", () => {
    expect(selectSessionTabTitle(state(false), ERRAND_ID)).toBe("Deep review");
  });
});
