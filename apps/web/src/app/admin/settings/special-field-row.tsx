import { useLanguage } from "@/context/LanguageContext";
import type { ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { TagInput } from "@/components/ui/tag-input";
import { MultiSelect, type MultiSelectOption } from "@/components/ui/multi-select";
import { KeyValueEditor } from "@/components/ui/key-value-editor";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  PROTOCOL_CONVERSION_ROUTES,
  type AdminSettingsDraft,
  type ProtocolConversionRoute,
} from "@/lib/admin-settings-form";
import { type SpecialField, fieldLabel } from "./settings-fields";
import type {
  AuthIdentityProvider,
  OAuthProviderConfig,
} from "../../../../../../packages/sdk/typescript/src/types.gen";

const OAUTH_PROVIDERS = [
  "oidc",
  "github",
  "google",
  "linuxdo",
  "wechat",
  "dingtalk",
] as const satisfies readonly AuthIdentityProvider[];

/**
 * Render one "special" settings field with a graphical control: chips for plain
 * string lists, a searchable model picker for the scheduler rollout scope, a
 * key→value editor for the email-template map, and a JSON box only for the
 * freeform custom-menus array (which has no fixed shape).
 */
export function SpecialFieldRow({
  field,
  draft,
  onChange,
  modelOptions,
}: {
  field: SpecialField;
  draft: AdminSettingsDraft;
  onChange: (key: keyof AdminSettingsDraft, value: unknown) => void;
  modelOptions: MultiSelectOption[];
}) {
  const { t } = useLanguage();
  const id = `s-${String(field.key)}`;
  const label = fieldLabel(field.skip, t);
  const value = draft[field.key];

  if (field.kind === "tags") {
    const tags = Array.isArray(value) ? (value as string[]) : [];
    const hintId = `adminSettings.fields.${field.skip}_hint`;
    const hint = t(hintId);
    return (
      <div>
        <Label htmlFor={id}>{label}</Label>
        <div className="mt-1.5">
          <TagInput id={id} value={tags} onChange={(next) => onChange(field.key, next)} />
        </div>
        {hint !== hintId ? (
          <p className="mt-1 text-2xs text-srapi-text-tertiary">{hint}</p>
        ) : null}
      </div>
    );
  }

  if (field.kind === "conversion-routes") {
    const selected = new Set(Array.isArray(value) ? (value as ProtocolConversionRoute[]) : []);
    const routes = protocolConversionRouteOptions(t);
    return (
      <div>
        <Label>{label}</Label>
        <div className="mt-2 grid gap-2 sm:grid-cols-2">
          {routes.map((route) => {
            const checked = selected.has(route.value);
            return (
              <label
                key={route.value}
                className="flex min-h-10 cursor-pointer items-center gap-3 rounded-md border border-srapi-border bg-srapi-surface/40 px-3 py-2 text-sm"
              >
                <Checkbox
                  checked={checked}
                  onChange={(e) => {
                    const next = new Set(selected);
                    if (e.target.checked) next.add(route.value);
                    else next.delete(route.value);
                    onChange(field.key, routes.map((item) => item.value).filter((item) => next.has(item)));
                  }}
                />
                <span>{route.label}</span>
              </label>
            );
          })}
        </div>
        <p className="mt-1 text-2xs text-srapi-text-tertiary">{t("adminSettings.fields.protocol_conversion_routes_hint")}</p>
      </div>
    );
  }

  if (field.kind === "models") {
    const selected = Array.isArray(value) ? (value as string[]) : [];
    return (
      <div>
        <Label htmlFor={id}>{label}</Label>
        <div className="mt-1.5">
          <MultiSelect
            id={id}
            value={selected}
            onChange={(next) => onChange(field.key, next)}
            options={modelOptions}
            allowCustom
            placeholder={t("adminSettings.allModels")}
            searchPlaceholder={t("adminCommon.search")}
            emptyText={t("adminCommon.noResults")}
            addCustomLabel={(q) => t("adminCommon.addValue", { value: q })}
          />
        </div>
        <p className="mt-1 text-2xs text-srapi-text-tertiary">{t("adminSettings.rolloutModelsHint")}</p>
      </div>
    );
  }

  if (field.kind === "templates") {
    const map =
      value && typeof value === "object" && !Array.isArray(value)
        ? (value as Record<string, string>)
        : {};
    return (
      <div>
        <Label>{label}</Label>
        <div className="mt-1.5">
          <KeyValueEditor
            value={map}
            onChange={(next) => onChange(field.key, next)}
            addLabel={t("adminSettings.addTemplate")}
            keyPlaceholder={t("adminSettings.templateKeyPlaceholder")}
            valuePlaceholder={t("adminSettings.templateValuePlaceholder")}
          />
        </div>
      </div>
    );
  }

  if (field.kind === "oauth-provider-configs") {
    const configs = Array.isArray(value) ? (value as OAuthProviderConfig[]) : [];
    return (
      <OAuthProviderConfigsEditor
        id={id}
        label={label}
        value={configs}
        onChange={(next) => onChange(field.key, next)}
      />
    );
  }

  return (
    <div>
      <Label htmlFor={id}>{label}</Label>
      <Textarea
        id={id}
        className="min-h-28 font-mono text-xs"
        spellCheck={false}
        value={String(value ?? "")}
        onChange={(e) => onChange(field.key, e.target.value)}
      />
      <p className="mt-1 text-2xs text-srapi-text-tertiary">{t("adminSettings.customMenusHint")}</p>
    </div>
  );
}

function protocolConversionRouteOptions(t: (key: string) => string): Array<{ value: ProtocolConversionRoute; label: string }> {
  return PROTOCOL_CONVERSION_ROUTES.map((value) => ({
    value,
    label: t(`adminSettings.protocolConversionRoutes.${value}`),
  }));
}

function OAuthProviderConfigsEditor({
  id,
  label,
  value,
  onChange,
}: {
  id: string;
  label: string;
  value: OAuthProviderConfig[];
  onChange: (next: OAuthProviderConfig[]) => void;
}) {
  const { t } = useLanguage();

  function update(index: number, patch: Partial<OAuthProviderConfig>) {
    onChange(value.map((item, i) => (i === index ? { ...item, ...patch } : item)));
  }

  function remove(index: number) {
    onChange(value.filter((_, i) => i !== index));
  }

  function add() {
    onChange([
      ...value,
      {
        provider: "oidc",
        provider_key: "",
        display_name: "",
        client_id: "",
        authorize_url: "",
        token_auth_method: "none",
        redirect_uri: "",
        scopes: [],
      },
    ]);
  }

  return (
    <div id={id} className="space-y-2">
      <div className="flex items-center justify-between gap-3">
        <Label>{label}</Label>
        <Button type="button" variant="outline" size="sm" onClick={add}>
          {t("adminSettings.addOAuthProviderConfig")}
        </Button>
      </div>
      <p className="text-2xs text-srapi-text-tertiary">
        {t("adminSettings.oauthProviderConfigsHint")}
      </p>
      {value.length === 0 ? (
        <div className="rounded-lg border border-dashed border-srapi-border px-3 py-4 text-sm text-srapi-text-tertiary">
          {t("adminSettings.oauthProviderConfigsEmpty")}
        </div>
      ) : (
        <div className="space-y-3">
          {value.map((config, index) => (
            <div key={index} className="rounded-lg border border-srapi-border p-3">
              <div className="grid gap-3 lg:grid-cols-12">
                <Field className="lg:col-span-2" label={t("adminSettings.oauthFields.provider")}>
                  <Select
                    value={config.provider}
                    onValueChange={(provider) =>
                      update(index, { provider: provider as AuthIdentityProvider })
                    }
                  >
                    <SelectTrigger className="h-9 rounded-lg">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {OAUTH_PROVIDERS.map((provider) => (
                        <SelectItem key={provider} value={provider}>
                          {provider}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>
                <Field
                  className="lg:col-span-2"
                  htmlFor={`oauth-${index}-provider-key`}
                  label={t("adminSettings.oauthFields.providerKey")}
                >
                  <Input
                    id={`oauth-${index}-provider-key`}
                    className="h-9"
                    value={config.provider_key}
                    onChange={(event) => update(index, { provider_key: event.target.value })}
                  />
                </Field>
                <Field
                  className="lg:col-span-2"
                  htmlFor={`oauth-${index}-display-name`}
                  label={t("adminSettings.oauthFields.displayName")}
                >
                  <Input
                    id={`oauth-${index}-display-name`}
                    className="h-9"
                    value={config.display_name}
                    onChange={(event) => update(index, { display_name: event.target.value })}
                  />
                </Field>
                <Field
                  className="lg:col-span-3"
                  htmlFor={`oauth-${index}-client-id`}
                  label={t("adminSettings.oauthFields.clientId")}
                >
                  <Input
                    id={`oauth-${index}-client-id`}
                    className="h-9 font-mono text-xs"
                    value={config.client_id}
                    onChange={(event) => update(index, { client_id: event.target.value })}
                  />
                </Field>
                <div className="flex items-end justify-end lg:col-span-3">
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => remove(index)}
                  >
                    {t("common.delete")}
                  </Button>
                </div>
                <Field
                  className="lg:col-span-6"
                  htmlFor={`oauth-${index}-authorize-url`}
                  label={t("adminSettings.oauthFields.authorizeUrl")}
                >
                  <Input
                    id={`oauth-${index}-authorize-url`}
                    className="h-9 font-mono text-xs"
                    value={config.authorize_url}
                    onChange={(event) => update(index, { authorize_url: event.target.value })}
                  />
                </Field>
                <Field
                  className="lg:col-span-6"
                  htmlFor={`oauth-${index}-redirect-uri`}
                  label={t("adminSettings.oauthFields.redirectUri")}
                >
                  <Input
                    id={`oauth-${index}-redirect-uri`}
                    className="h-9 font-mono text-xs"
                    value={config.redirect_uri}
                    onChange={(event) => update(index, { redirect_uri: event.target.value })}
                  />
                </Field>
                <Field
                  className="lg:col-span-6"
                  htmlFor={`oauth-${index}-token-url`}
                  label={t("adminSettings.oauthFields.tokenUrl")}
                >
                  <Input
                    id={`oauth-${index}-token-url`}
                    className="h-9 font-mono text-xs"
                    value={config.token_url ?? ""}
                    onChange={(event) => update(index, { token_url: event.target.value })}
                  />
                </Field>
                <Field
                  className="lg:col-span-6"
                  htmlFor={`oauth-${index}-userinfo-url`}
                  label={t("adminSettings.oauthFields.userinfoUrl")}
                >
                  <Input
                    id={`oauth-${index}-userinfo-url`}
                    className="h-9 font-mono text-xs"
                    value={config.userinfo_url ?? ""}
                    onChange={(event) => update(index, { userinfo_url: event.target.value })}
                  />
                </Field>
                <Field
                  className="lg:col-span-12"
                  htmlFor={`oauth-${index}-scopes`}
                  label={t("adminSettings.oauthFields.scopes")}
                >
                  <TagInput
                    id={`oauth-${index}-scopes`}
                    value={config.scopes}
                    onChange={(scopes) => update(index, { scopes })}
                    placeholder="openid, email, profile"
                  />
                </Field>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function Field({
  label,
  className,
  htmlFor,
  children,
}: {
  label: string;
  className?: string;
  htmlFor?: string;
  children: ReactNode;
}) {
  return (
    <div className={className}>
      <label
        htmlFor={htmlFor}
        className="mb-1.5 block text-2xs font-medium uppercase tracking-wide text-srapi-text-tertiary"
      >
        {label}
      </label>
      {children}
    </div>
  );
}
