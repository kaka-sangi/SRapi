"use client";

import { AlertTriangle, CheckCircle, X } from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";
import type { SmokeChecklist } from "@/lib/srapi-types";
import { Badge } from "@/components/ui";
import { cn } from "@/lib/cn";

interface SmokeDrawerProps {
  open: boolean;
  onClose: () => void;
  status: SmokeChecklist;
}

const REQUIRED_ENDPOINTS = ["/v1/chat/completions", "/v1/responses", "/v1/messages"];

export function SmokeDrawer({ open, onClose, status }: SmokeDrawerProps) {
  const { t } = useLanguage();

  if (!open) {
    return null;
  }

  return (
    <>
      <div
        onClick={onClose}
        className="fixed inset-0 z-50 bg-black/40"
        aria-hidden="true"
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-label={t("smokeDrawerTitle")}
        className="paper-grain fixed bottom-0 right-0 top-0 z-50 w-full max-w-lg space-y-6 overflow-y-auto border-l border-srapi-border bg-srapi-card p-6 shadow-2xl md:p-8"
      >
        <div className="flex items-center justify-between border-b border-srapi-border pb-4">
          <div>
            <h3 className="font-serif text-lg font-bold tracking-tight">
              {t("smokeDrawerTitle")}
            </h3>
            <p className="mt-0.5 font-mono text-xs text-srapi-text-secondary">
              {status.base_url}
            </p>
          </div>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close"
            className="rounded-full border border-srapi-border p-1.5 text-srapi-text-secondary transition-colors hover:bg-srapi-card-muted hover:text-srapi-text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-srapi-primary"
          >
            <X size={16} aria-hidden="true" />
          </button>
        </div>

        <div
          className={cn(
            "flex items-start gap-3.5 rounded-2xl border p-5",
            status.v0_1_smoke_evidence_complete
              ? "border-srapi-success/30 bg-srapi-success/5 text-srapi-success"
              : "border-srapi-error/30 bg-srapi-error/5 text-srapi-error",
          )}
        >
          {status.v0_1_smoke_evidence_complete ? (
            <CheckCircle size={20} className="mt-0.5 shrink-0" aria-hidden="true" />
          ) : (
            <AlertTriangle size={20} className="mt-0.5 shrink-0" aria-hidden="true" />
          )}
          <div className="space-y-1">
            <h4 className="text-sm font-semibold">
              {t("statusTitle")}:{" "}
              {status.v0_1_smoke_evidence_complete ? t("completeState") : t("notComplete")}
            </h4>
            <p className="text-xs leading-relaxed text-srapi-text-secondary">
              {status.v0_1_smoke_evidence_complete ? t("smokeDesc") : t("smokeIncomplete")}
            </p>
          </div>
        </div>

        <div className="space-y-5">
          <span className="block border-b border-srapi-border pb-2 font-mono text-[11px] font-bold uppercase tracking-wider text-srapi-text-secondary">
            {t("constraintsMatrix")}
          </span>

          <div className="space-y-4">
            <CheckRow
              title={t("modelEntryVerif")}
              detail={t("modelRegistered", { model: status.model })}
              ok={status.model_exists}
              badge={status.model_exists ? t("exists") : t("missing")}
            />

            <div className="flex items-start justify-between">
              <div className="text-xs">
                <p className="font-semibold">{t("publicUpstreamAcc")}</p>
                <p className="mt-0.5 text-[11px] text-srapi-text-secondary">
                  {t("requiresPublic")}
                </p>
              </div>
              <div className="flex flex-col items-end gap-1 font-mono text-[11px]">
                <Badge
                  variant={status.public_https_upstream_account_count > 0 ? "success" : "danger"}
                >
                  {t("activeAccounts", { count: status.public_https_upstream_account_count })}
                </Badge>
                <span className="text-[11px] text-srapi-text-secondary">
                  ({status.active_account_count} active accounts)
                </span>
              </div>
            </div>

            <EndpointMatrix
              title={t("healthyTrafficRegistry")}
              description={t("loggedHealthy")}
              endpoints={REQUIRED_ENDPOINTS}
              recordedEndpoints={status.usage_endpoints}
              okLabel={t("completeState")}
              pendingLabel={t("incompleteState")}
              isComplete={status.missing_usage_endpoints.length === 0}
            />

            <EndpointMatrix
              title={t("schedulerRouting")}
              description={t("decisionsRouting")}
              endpoints={REQUIRED_ENDPOINTS}
              recordedEndpoints={status.real_upstream_scheduler_decision_endpoints}
              okLabel={t("completeState")}
              pendingLabel={t("incompleteState")}
              isComplete={status.missing_real_upstream_scheduler_decision_endpoints.length === 0}
            />
          </div>

          <div className="space-y-2 rounded-2xl border border-srapi-border bg-srapi-card-muted p-4 font-mono text-[11px] text-srapi-text-secondary">
            <p className="text-xs font-bold text-srapi-text-primary">
              {t("diagnosticInstructions")}
            </p>
            <p>{t("instr1")}</p>
            <p>{t("instr2")}</p>
            <p>{t("instr3")}</p>
            <pre className="mt-2 overflow-x-auto rounded-lg border border-srapi-border bg-srapi-card p-2 text-[11px] text-srapi-text-primary">
              node tools/srapi-admin.mjs smoke-status
            </pre>
          </div>
        </div>
      </div>
    </>
  );
}

function CheckRow({
  title,
  detail,
  ok,
  badge,
}: {
  title: string;
  detail: string;
  ok: boolean;
  badge: string;
}) {
  return (
    <div className="flex items-start justify-between">
      <div className="text-xs">
        <p className="font-semibold">{title}</p>
        <p className="mt-0.5 text-[11px] text-srapi-text-secondary">{detail}</p>
      </div>
      <Badge variant={ok ? "success" : "danger"}>{badge}</Badge>
    </div>
  );
}

function EndpointMatrix({
  title,
  description,
  endpoints,
  recordedEndpoints,
  okLabel,
  pendingLabel,
  isComplete,
}: {
  title: string;
  description: string;
  endpoints: string[];
  recordedEndpoints: string[];
  okLabel: string;
  pendingLabel: string;
  isComplete: boolean;
}) {
  return (
    <div className="space-y-2 border-t border-srapi-border/40 pt-3">
      <div className="flex items-center justify-between text-xs">
        <span className="font-semibold">{title}</span>
        <Badge variant={isComplete ? "success" : "danger"}>
          {isComplete ? okLabel : pendingLabel}
        </Badge>
      </div>
      <p className="text-[11px] text-srapi-text-secondary">{description}</p>
      <div className="mt-1 grid grid-cols-3 gap-2">
        {endpoints.map((ep) => {
          const has = recordedEndpoints.includes(ep);
          return (
            <div
              key={ep}
              className={cn(
                "rounded-lg border p-2 text-center font-mono text-[11px]",
                has
                  ? "border-srapi-success/30 bg-srapi-success/5 text-srapi-success"
                  : "border-srapi-border bg-srapi-card-muted/40 text-srapi-text-secondary",
              )}
            >
              {ep.replace("/v1", "")}
            </div>
          );
        })}
      </div>
    </div>
  );
}
