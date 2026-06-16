import type {
  CreateModelAliasRequest,
  CreateModelProviderMappingRequest,
  CreateModelRequest,
  Model,
  ModelAlias,
  ModelProviderMapping,
  ResourceStatus,
  UpdateModelAliasRequest,
  UpdateModelProviderMappingRequest,
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
    capabilities: [],
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

// Pre-populates the alias-edit form from a row so the dialog opens with the
// current values rather than blanks. The form shape is identical to create,
// so the same FieldConfig + dialog can drive both flows.
export function modelAliasFormFromRow(alias: ModelAlias): ModelAliasFormState {
  return {
    alias: alias.alias,
    strategyHint: alias.strategy_hint ?? "",
    fallbackModelsText: (alias.fallback_models ?? []).join("\n"),
    status: alias.status,
  };
}

// The update body sends ALL fields the form knows about, not a diff. The
// backend service treats nil-pointer fields as "no change", so we always
// pass the current form state — letting the user-visible form be the source
// of truth (a half-cleared field clears it). For fallback_models we pass [].
export function buildUpdateModelAliasBody(form: ModelAliasFormState): UpdateModelAliasRequest {
  return {
    alias: requiredText(form.alias, "Alias"),
    strategy_hint: optionalText(form.strategyHint),
    fallback_models: splitLines(form.fallbackModelsText),
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

export function modelMappingFormFromRow(mapping: ModelProviderMapping): ModelMappingFormState {
  return {
    providerId: mapping.provider_id,
    upstreamModelName: mapping.upstream_model_name,
    status: mapping.status,
    capabilities: descriptorsToCapabilityKeys(mapping.capability_override ?? []),
    pricingOverride: (mapping.pricing_override ?? {}) as Record<string, unknown>,
  };
}

// The update body omits provider_id — the backend explicitly does NOT support
// reassigning a mapping to a different provider via PATCH (that'd require a
// new mapping row to preserve audit history). The dialog disables the
// provider field in edit mode to match.
export function buildUpdateModelMappingBody(
  form: ModelMappingFormState,
): UpdateModelProviderMappingRequest {
  return {
    upstream_model_name: requiredText(form.upstreamModelName, "Upstream model name"),
    status: form.status,
    capability_override: capabilityKeysToDescriptors(form.capabilities),
    pricing_override: form.pricingOverride,
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
