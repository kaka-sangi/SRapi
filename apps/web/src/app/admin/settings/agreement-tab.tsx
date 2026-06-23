import { FileText } from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";

interface Props {
  value: Record<string, unknown>;
  onField: (key: string, v: unknown) => void;
  onSave: () => void;
  pending: boolean;
}

export function AgreementTab({ value, onField, onSave, pending }: Props) {
  const { t } = useLanguage();
  const str = (key: string) => (value[key] == null ? "" : String(value[key]));

  return (
    <div className="space-y-6">
      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <FileText className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">{t("adminSettings.agreement.title")}</h3>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.agreement.hint")}</p>
            </div>
          </div>

          <div>
            <Label htmlFor="ag-terms">{t("adminSettings.fields.user_agreement")}</Label>
            <p className="mb-1 text-xs text-srapi-text-tertiary">{t("adminSettings.agreement.userAgreementHint")}</p>
            <Textarea id="ag-terms" rows={8} value={str("user_agreement")}
              placeholder={t("adminSettings.agreement.userAgreementPlaceholder")}
              onChange={(e) => onField("user_agreement", e.target.value)} />
          </div>

          <div>
            <Label htmlFor="ag-privacy">{t("adminSettings.fields.privacy_policy")}</Label>
            <p className="mb-1 text-xs text-srapi-text-tertiary">{t("adminSettings.agreement.privacyPolicyHint")}</p>
            <Textarea id="ag-privacy" rows={8} value={str("privacy_policy")}
              placeholder={t("adminSettings.agreement.privacyPolicyPlaceholder")}
              onChange={(e) => onField("privacy_policy", e.target.value)} />
          </div>
        </CardContent>
      </Card>

      <div className="flex justify-end">
        <Button variant="primary" loading={pending} onClick={onSave}>{t("adminSettings.saveSection")}</Button>
      </div>
    </div>
  );
}
