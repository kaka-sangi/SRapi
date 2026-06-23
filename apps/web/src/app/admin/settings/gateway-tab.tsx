import { Server, RotateCcw, ArrowLeftRight, FlaskConical } from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
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

function NumField({ id, label, hint, value, onChange }: {
  id: string; label: string; hint?: string; value: number; onChange: (v: number) => void;
}) {
  return (
    <div>
      <Label htmlFor={id}>{label}</Label>
      {hint && <p className="mb-1 text-xs text-srapi-text-tertiary">{hint}</p>}
      <Input id={id} type="number" min={0} value={String(value)}
        onChange={(e) => onChange(e.target.value === "" ? 0 : Math.max(0, Math.trunc(Number(e.target.value))))} />
    </div>
  );
}

export function GatewayTab({ value, draft, onField, onSpecial, onSave, pending, modelOptions }: Props) {
  const { t } = useLanguage();
  const num = (key: string) => (typeof value[key] === "number" ? value[key] as number : 0);
  const rolloutEnabled = value.scheduler_strategy_rollout_enabled === true;

  return (
    <div className="space-y-6">
      {/* Cooldown & Timeouts */}
      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <Server className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">{t("adminSettings.gateway.cooldowns")}</h3>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.gateway.cooldownsHint")}</p>
            </div>
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            <NumField id="gw-overload" label={t("adminSettings.fields.overload_cooldown_seconds")}
              hint={t("adminSettings.gateway.overloadHint")} value={num("overload_cooldown_seconds")}
              onChange={(v) => onField("overload_cooldown_seconds", v)} />
            <NumField id="gw-ratelimit" label={t("adminSettings.fields.rate_limit_cooldown_seconds")}
              hint={t("adminSettings.gateway.rateLimitCooldownHint")} value={num("rate_limit_cooldown_seconds")}
              onChange={(v) => onField("rate_limit_cooldown_seconds", v)} />
            <NumField id="gw-stream" label={t("adminSettings.fields.stream_timeout_seconds")}
              hint={t("adminSettings.gateway.streamTimeoutHint")} value={num("stream_timeout_seconds")}
              onChange={(v) => onField("stream_timeout_seconds", v)} />
          </div>
        </CardContent>
      </Card>

      {/* Failover & Retry */}
      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <RotateCcw className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">{t("adminSettings.gateway.failover")}</h3>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.gateway.failoverHint")}</p>
            </div>
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            <NumField id="gw-retry" label={t("adminSettings.fields.retry_count")}
              hint={t("adminSettings.gateway.retryCountHint")} value={num("retry_count")}
              onChange={(v) => onField("retry_count", v)} />
            <NumField id="gw-maxcred" label={t("adminSettings.fields.max_retry_credentials")}
              hint={t("adminSettings.gateway.maxRetryCredentialsHint")} value={num("max_retry_credentials")}
              onChange={(v) => onField("max_retry_credentials", v)} />
            <NumField id="gw-maxinterval" label={t("adminSettings.fields.max_retry_interval_ms")}
              hint={t("adminSettings.gateway.maxRetryIntervalHint")} value={num("max_retry_interval_ms")}
              onChange={(v) => onField("max_retry_interval_ms", v)} />
          </div>
          <div className="flex items-center justify-between gap-4">
            <div>
              <Label htmlFor="gw-shaper" className="mb-0">{t("adminSettings.fields.request_shaper_enabled")}</Label>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.gateway.shaperHint")}</p>
            </div>
            <Switch id="gw-shaper" checked={value.request_shaper_enabled === true}
              onCheckedChange={(v) => onField("request_shaper_enabled", v)} />
          </div>
        </CardContent>
      </Card>

      {/* Protocol Conversion & Passthrough */}
      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <ArrowLeftRight className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">{t("adminSettings.gateway.protocol")}</h3>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.gateway.protocolHint")}</p>
            </div>
          </div>
          {(SPECIAL_FIELDS.gateway ?? [])
            .filter((f) => f.skip === "protocol_conversion_routes" || f.skip === "passthrough_header_allowlist")
            .map((field) => (
              <SpecialFieldRow key={String(field.key)} field={field} draft={draft} onChange={onSpecial} modelOptions={modelOptions} />
            ))}
          <div className="flex items-center justify-between gap-4">
            <div>
              <Label htmlFor="gw-passthrough" className="mb-0">{t("adminSettings.gateway.passthroughHeaders")}</Label>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.gateway.passthroughHeadersHint")}</p>
            </div>
            <Switch id="gw-passthrough" checked={value.passthrough_upstream_headers === true}
              onCheckedChange={(v) => onField("passthrough_upstream_headers", v)} />
          </div>
        </CardContent>
      </Card>

      {/* Scheduler Rollout */}
      <Card>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <FlaskConical className="size-5 text-srapi-text-tertiary" />
            <div>
              <h3 className="text-sm font-semibold text-srapi-text-primary">{t("adminSettings.gateway.rollout")}</h3>
              <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.gateway.rolloutHint")}</p>
            </div>
          </div>
          <div className="flex items-center justify-between gap-4">
            <Label htmlFor="gw-rollout" className="mb-0">{t("adminSettings.fields.scheduler_strategy_rollout_enabled")}</Label>
            <Switch id="gw-rollout" checked={rolloutEnabled}
              onCheckedChange={(v) => onField("scheduler_strategy_rollout_enabled", v)} />
          </div>
          {rolloutEnabled && (
            <div className="space-y-4 rounded-xl border border-srapi-border/70 bg-srapi-card-muted/30 p-4">
              <div className="grid gap-4 sm:grid-cols-2">
                <div>
                  <Label htmlFor="gw-shadow">{t("adminSettings.fields.scheduler_strategy_shadow_strategy")}</Label>
                  <Input id="gw-shadow" value={value.scheduler_strategy_shadow_strategy == null ? "" : String(value.scheduler_strategy_shadow_strategy)}
                    placeholder="balanced" onChange={(e) => onField("scheduler_strategy_shadow_strategy", e.target.value)} />
                </div>
                <NumField id="gw-percent" label={t("adminSettings.fields.scheduler_strategy_rollout_percent")}
                  value={num("scheduler_strategy_rollout_percent")} onChange={(v) => onField("scheduler_strategy_rollout_percent", v)} />
              </div>
              {(SPECIAL_FIELDS.gateway ?? [])
                .filter((f) => f.skip === "scheduler_strategy_rollout_models" || f.skip === "scheduler_strategy_rollout_api_key_hashes")
                .map((field) => (
                  <SpecialFieldRow key={String(field.key)} field={field} draft={draft} onChange={onSpecial} modelOptions={modelOptions} />
                ))}
            </div>
          )}
        </CardContent>
      </Card>

      <div className="flex justify-end">
        <Button variant="primary" loading={pending} onClick={onSave}>{t("adminSettings.saveSection")}</Button>
      </div>
    </div>
  );
}
