import { Users, Wallet, Gauge, Trash2 } from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";

interface Props {
  value: Record<string, unknown>;
  onField: (key: string, v: unknown) => void;
  onSave: () => void;
  pending: boolean;
}

export function UsersTab({ value, onField, onSave, pending }: Props) {
  const { t } = useLanguage();
  const str = (key: string) => (value[key] == null ? "" : String(value[key]));
  const num = (key: string) => (typeof value[key] === "number" ? value[key] as number : 0);

  return (
    <div className="space-y-6">
      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <Wallet className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">{t("adminSettings.users.defaults")}</h3>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.users.defaultsHint")}</p>
            </div>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <Label htmlFor="u-balance">{t("adminSettings.fields.default_balance")}</Label>
              <p className="mb-1 text-xs text-srapi-text-tertiary">{t("adminSettings.users.defaultBalanceHint")}</p>
              <Input id="u-balance" value={str("default_balance")} placeholder="0.00"
                onChange={(e) => onField("default_balance", e.target.value)} />
            </div>
            <div>
              <Label htmlFor="u-group">{t("adminSettings.fields.default_group")}</Label>
              <p className="mb-1 text-xs text-srapi-text-tertiary">{t("adminSettings.users.defaultGroupHint")}</p>
              <Input id="u-group" value={str("default_group")} placeholder="default"
                onChange={(e) => onField("default_group", e.target.value)} />
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <Gauge className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">{t("adminSettings.users.rateLimit")}</h3>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.users.rateLimitHint")}</p>
            </div>
          </div>
          <div>
            <Label htmlFor="u-rpm">{t("adminSettings.fields.rpm_limit_default")}</Label>
            <p className="mb-1 text-xs text-srapi-text-tertiary">{t("adminSettings.users.rpmHint")}</p>
            <Input id="u-rpm" type="number" min={0} value={String(num("rpm_limit_default"))}
              onChange={(e) => onField("rpm_limit_default", e.target.value === "" ? 0 : Math.max(0, Math.trunc(Number(e.target.value))))} />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <Trash2 className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">{t("adminSettings.users.selfDelete")}</h3>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.users.selfDeleteHint")}</p>
            </div>
          </div>
          <div className="flex items-center justify-between gap-4">
            <Label htmlFor="u-selfdelete" className="mb-0">{t("adminSettings.fields.user_self_delete_enabled")}</Label>
            <Switch id="u-selfdelete" checked={value.user_self_delete_enabled === true}
              onCheckedChange={(v) => onField("user_self_delete_enabled", v)} />
          </div>
        </CardContent>
      </Card>

      <div className="flex justify-end">
        <Button variant="primary" loading={pending} onClick={onSave}>{t("adminSettings.saveSection")}</Button>
      </div>
    </div>
  );
}
