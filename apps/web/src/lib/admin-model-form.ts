import type {
  CreateModelAliasRequest,
  CreateModelProviderMappingRequest,
  CreateModelRequest,
  Model,
  ResourceStatus,
  UpdateModelRequest,
} from "../../../../packages/sdk/typescript/src/types.gen";
import {
  capabilityKeysToDescriptors,
  descriptorsToCapabilityKeys,
} from "@/lib/capabilities";

export const MODEL_STATUSES: ResourceStatus[] = ["active", "disabled", "pending", "archived"];

export interface ModelFormState {
  canonicalName: string;
  displayName: string;
  family: string;
  contextWindow: string;
  maxOutputTokens: string;
  qualityTier: string;
  status: ResourceStatus;
  /** Selected capability keys (chips); mapped to descriptors on submit. */
  capabilities: string[];
}

export function emptyModelForm(): ModelFormState {
  return {
    canonicalName: "",
    displayName: "",
    family: "",
    contextWindow: "",
    maxOutputTokens: "",
    qualityTier: "",
    status: "active",
    capabilities: ["chat_completions"],
  };
}

export function modelFormFromModel(model: Model): ModelFormState {
  return {
    canonicalName: model.canonical_name,
    displayName: model.display_name,
    family: model.family ?? "",
    contextWindow: model.context_window != null ? String(model.context_window) : "",
    maxOutputTokens: model.max_output_tokens != null ? String(model.max_output_tokens) : "",
    qualityTier: model.quality_tier ?? "",
    status: model.status,
    capabilities: descriptorsToCapabilityKeys(model.capabilities),
  };
}

export function buildCreateModelBody(form: ModelFormState): CreateModelRequest {
  return {
    canonical_name: requiredText(form.canonicalName, "Canonical name"),
    display_name: requiredText(form.displayName, "Display name"),
    family: optionalText(form.family),
    context_window: optionalInt(form.contextWindow, "Context window"),
    max_output_tokens: optionalInt(form.maxOutputTokens, "Max output tokens"),
    quality_tier: optionalText(form.qualityTier),
    status: form.status,
    capabilities: capabilityKeysToDescriptors(form.capabilities),
  };
}

export function buildUpdateModelBody(form: ModelFormState): UpdateModelRequest {
  return {
    display_name: requiredText(form.displayName, "Display name"),
    family: optionalText(form.family) ?? null,
    context_window: optionalInt(form.contextWindow, "Context window") ?? null,
    max_output_tokens: optionalInt(form.maxOutputTokens, "Max output tokens") ?? null,
    quality_tier: optionalText(form.qualityTier) ?? null,
    status: form.status,
    capabilities: capabilityKeysToDescriptors(form.capabilities),
  };
}

// --- Model alias (alias → this model, with optional fallback chain) ---

export interface ModelAliasFormState {
  alias: string;
  strategyHint: string;
  fallbackModelsText: string;
  status: ResourceStatus;
}

export function emptyModelAliasForm(): ModelAliasFormState {
  return { alias: "", strategyHint: "", fallbackModelsText: "", status: "active" };
}

export function buildCreateModelAliasBody(form: ModelAliasFormState): CreateModelAliasRequest {
  const fallbacks = splitLines(form.fallbackModelsText);
  return {
    alias: requiredText(form.alias, "Alias"),
    strategy_hint: optionalText(form.strategyHint),
    fallback_models: fallbacks.length ? fallbacks : undefined,
    status: form.status,
  };
}

// --- Model → provider mapping (this model served by a provider's upstream name) ---

export interface ModelMappingFormState {
  providerId: string;
  upstreamModelName: string;
  status: ResourceStatus;
  /** Capability keys (chips) overriding the model defaults for this provider. */
  capabilities: string[];
  pricingOverride: Record<string, unknown>;
}

export function emptyModelMappingForm(): ModelMappingFormState {
  return {
    providerId: "",
    upstreamModelName: "",
    status: "active",
    capabilities: [],
    pricingOverride: {},
  };
}

export function buildCreateModelMappingBody(
  form: ModelMappingFormState,
): CreateModelProviderMappingRequest {
  return {
    provider_id: requiredText(form.providerId, "Provider"),
    upstream_model_name: requiredText(form.upstreamModelName, "Upstream model name"),
    status: form.status,
    capability_override: form.capabilities.length
      ? capabilityKeysToDescriptors(form.capabilities)
      : undefined,
    pricing_override: Object.keys(form.pricingOverride).length ? form.pricingOverride : undefined,
  };
}

function splitLines(value: string): string[] {
  return value
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean);
}

function optionalInt(value: string, fieldName: string): number | undefined {
  const trimmed = value.trim();
  if (!trimmed) return undefined;
  const parsed = Number(trimmed);
  if (!Number.isInteger(parsed) || parsed < 0) {
    throw new Error(`${fieldName} must be a non-negative whole number.`);
  }
  return parsed;
}

function optionalText(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed || undefined;
}

function requiredText(value: string, fieldName: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    throw new Error(`${fieldName} is required.`);
  }
  return trimmed;
}
