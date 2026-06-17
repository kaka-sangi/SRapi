import { describe, expect, it } from "vitest";
import {
  MODEL_CAPABILITY_OPTIONS,
  capabilityKeysToDescriptors,
  descriptorsToCapabilityKeys,
} from "@/lib/capabilities";

// The capability descriptor shape capabilityKeysToDescriptors emits is what
// the scheduler hard-filters on — every model save round-trips through this
// pair. The "v1 / stable / required" tuple is intentionally fixed (per-
// capability version/status/level tuning is intentionally not exposed
// graphically). Pinning that contract so a future "let admin pick version"
// PR has to explicitly update the test, not just quietly change the default.

describe("capabilityKeysToDescriptors", () => {
  it("emits the canonical {key, version: 'v1', status: 'stable', level: 'required'} tuple", () => {
    const out = capabilityKeysToDescriptors(["chat_completions", "embeddings"]);
    expect(out).toEqual([
      { key: "chat_completions", version: "v1", status: "stable", level: "required" },
      { key: "embeddings", version: "v1", status: "stable", level: "required" },
    ]);
  });

  it("preserves caller order so the saved descriptor list is deterministic", () => {
    const out = capabilityKeysToDescriptors(["images", "embeddings", "chat_completions"]);
    expect(out.map((d) => d.key)).toEqual(["images", "embeddings", "chat_completions"]);
  });

  it("returns an empty array for no keys", () => {
    expect(capabilityKeysToDescriptors([])).toEqual([]);
  });
});

describe("descriptorsToCapabilityKeys", () => {
  it("extracts .key from each descriptor, preserving order", () => {
    expect(
      descriptorsToCapabilityKeys([
        { key: "images", version: "v1", status: "stable", level: "required" },
        { key: "rerank", version: "v2", status: "beta", level: "preferred" },
      ]),
    ).toEqual(["images", "rerank"]);
  });

  it("treats undefined as 'no descriptors' rather than throwing", () => {
    // The chip selector calls this with model.capabilities which can be
    // undefined on a brand-new model. Must not crash the form.
    expect(descriptorsToCapabilityKeys(undefined)).toEqual([]);
  });

  it("returns [] for an empty descriptor array", () => {
    expect(descriptorsToCapabilityKeys([])).toEqual([]);
  });
});

describe("keys <-> descriptors round-trip", () => {
  it("is lossless on the key dimension (the only thing the chip UI exposes)", () => {
    const keys = ["chat_completions", "responses", "audio_transcriptions"];
    expect(descriptorsToCapabilityKeys(capabilityKeysToDescriptors(keys))).toEqual(keys);
  });
});

describe("MODEL_CAPABILITY_OPTIONS", () => {
  it("includes every endpoint the scheduler currently knows about", () => {
    // Pin the union here so a "remove an endpoint" PR has to explicitly
    // touch this test — the option list is what operators see when they
    // tag a model, and silently dropping an option would invisibly block
    // models from being tagged for that endpoint.
    const values = MODEL_CAPABILITY_OPTIONS.map((o) => o.value);
    expect(values).toEqual([
      "chat_completions",
      "responses",
      "responses_compact",
      "embeddings",
      "images",
      "audio_transcriptions",
      "audio_speech",
      "moderations",
      "rerank",
      "token_counting",
      "realtime_websocket",
    ]);
  });

  it("has a non-empty label for every option (so the chip never renders blank)", () => {
    for (const o of MODEL_CAPABILITY_OPTIONS) {
      expect(o.label.trim().length).toBeGreaterThan(0);
    }
  });
});
