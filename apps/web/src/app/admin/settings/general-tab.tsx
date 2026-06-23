import { Globe, Palette, Link2 } from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { type MultiSelectOption } from "@/components/ui/multi-select";
import { type AdminSettingsDraft } from "@/lib/admin-settings-form";
import { SPECIAL_FIELDS } from "./settings-fields";
import { SpecialFieldRow } from "./special-field-row";

interface Props {
  value: Record<string, unknown>;
  draft: AdminSettingsDraft;
  onField: (key: string, v: unknown) => void;
  onSpecial: (key: keyof AdminSettingsDraft, v: unknown) => void;
  onSave: () => void;
  pending: boolean;
  modelOptions: MultiSelectOption[];
}

function TextField({ id, label, hint, value, placeholder, onChange }: {
  id: string; label: string; hint?: string; value: string;
  placeholder?: string; onChange: (v: string) => void;
}) {
  return (
    <div>
      <Label htmlFor={id}>{label}</Label>
      {hint && <p className="mb-1 text-xs text-srapi-text-tertiary">{hint}</p>}
      <Input id={id} value={value} placeholder={placeholder} onChange={(e) => onChange(e.target.value)} />
    </div>
  );
}

export function GeneralTab({ value, draft, onField, onSpecial, onSave, pending, modelOptions }: Props) {
  const { t } = useLanguage();
  const str = (key: string) => (value[key] == null ? "" : String(value[key]));

  return (
    <div className="space-y-6">
      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <Palette className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">{t("adminSettings.general.branding")}</h3>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.general.brandingHint")}</p>
            </div>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <TextField id="g-name" label={t("adminSettings.fields.site_name")} value={str("site_name")}
              placeholder="SRapi" onChange={(v) => onField("site_name", v)} />
            <TextField id="g-subtitle" label={t("adminSettings.fields.site_subtitle")} value={str("site_subtitle")}
              placeholder="AI Gateway" onChange={(v) => onField("site_subtitle", v)} />
            <TextField id="g-logo" label={t("adminSettings.fields.logo_url")} value={str("logo_url")}
              hint={t("adminSettings.general.logoHint")} placeholder="https://..." onChange={(v) => onField("logo_url", v)} />
            <TextField id="g-version" label={t("adminSettings.fields.version_label")} value={str("version_label")}
              placeholder="v1.0" onChange={(v) => onField("version_label", v)} />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <Link2 className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">{t("adminSettings.general.links")}</h3>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.general.linksHint")}</p>
            </div>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <TextField id="g-contact" label={t("adminSettings.fields.contact_info")} value={str("contact_info")}
              placeholder="support@example.com" onChange={(v) => onField("contact_info", v)} />
            <TextField id="g-doc" label={t("adminSettings.fields.doc_url")} value={str("doc_url")}
              placeholder="https://docs.example.com" onChange={(v) => onField("doc_url", v)} />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <Globe className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">{t("adminSettings.general.customMenus")}</h3>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.customMenusHint")}</p>
            </div>
          </div>
          {(SPECIAL_FIELDS.general ?? []).map((field) => (
            <SpecialFieldRow key={String(field.key)} field={field} draft={draft} onChange={onSpecial} modelOptions={modelOptions} />
          ))}
        </CardContent>
      </Card>

      <div className="flex justify-end">
        <Button variant="primary" loading={pending} onClick={onSave}>{t("adminSettings.saveSection")}</Button>
      </div>
    </div>
  );
}
