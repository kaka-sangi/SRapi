import { useLanguage } from "@/context/LanguageContext";
import { Checkbox } from "@/components/ui/checkbox";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { TagInput } from "@/components/ui/tag-input";
import { MultiSelect, type MultiSelectOption } from "@/components/ui/multi-select";
import { KeyValueEditor } from "@/components/ui/key-value-editor";
import {
  PROTOCOL_CONVERSION_ROUTES,
  type AdminSettingsDraft,
  type ProtocolConversionRoute,
} from "@/lib/admin-settings-form";
import { type SpecialField, fieldLabel } from "./settings-fields";

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
