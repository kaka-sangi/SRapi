import { Zap, Monitor, Gift } from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { MultiSelect, type MultiSelectOption } from "@/components/ui/multi-select";
import { useAdminGroups } from "@/hooks/admin-queries";
import { type AdminSettingsDraft } from "@/lib/admin-settings-form";

interface Props {
  value: Record<string, unknown>;
  draft: AdminSettingsDraft;
  onField: (key: string, v: unknown) => void;
  onSpecial: (key: keyof AdminSettingsDraft, v: unknown) => void;
  onSave: () => void;
  pending: boolean;
}

export function FeaturesTab({ value, draft, onField, onSpecial, onSave, pending }: Props) {
  const { t } = useLanguage();
  const groups = useAdminGroups();
  const channelOptions: MultiSelectOption[] = (groups.data?.data ?? []).map((g) => ({
    value: g.name,
    label: g.name,
  }));

  return (
    <div className="space-y-6">
      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <Zap className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">{t("adminSettings.features.channels")}</h3>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.features.channelsHint")}</p>
            </div>
          </div>
          <div>
            <Label>{t("adminSettings.features.channelsLabel")}</Label>
            <MultiSelect
              options={channelOptions}
              value={Array.isArray(draft.enabledChannels) ? draft.enabledChannels : []}
              onChange={(next) => onSpecial("enabledChannels", next)}
              placeholder={t("adminSettings.features.channelsPlaceholder")}
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <Monitor className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">{t("adminSettings.features.monitoring")}</h3>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.features.monitoringHint")}</p>
            </div>
          </div>
          <div className="flex items-center justify-between gap-4">
            <div>
              <Label htmlFor="ft-chanmon" className="mb-0">{t("adminSettings.fields.channel_monitoring_enabled")}</Label>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.features.channelMonitoringHint")}</p>
            </div>
            <Switch id="ft-chanmon" checked={value.channel_monitoring_enabled === true}
              onCheckedChange={(v) => onField("channel_monitoring_enabled", v)} />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <Gift className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">{t("adminSettings.features.affiliate")}</h3>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.features.affiliateHint")}</p>
            </div>
          </div>
          <div className="flex items-center justify-between gap-4">
            <div>
              <Label htmlFor="ft-rebate" className="mb-0">{t("adminSettings.fields.invitation_rebate_enabled")}</Label>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.features.invitationRebateHint")}</p>
            </div>
            <Switch id="ft-rebate" checked={value.invitation_rebate_enabled === true}
              onCheckedChange={(v) => onField("invitation_rebate_enabled", v)} />
          </div>
        </CardContent>
      </Card>

      <div className="flex justify-end">
        <Button variant="primary" loading={pending} onClick={onSave}>{t("adminSettings.saveSection")}</Button>
      </div>
    </div>
  );
}
