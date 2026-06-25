"use client";

import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useLanguage } from "@/context/LanguageContext";

interface OpenAICapabilitiesProps {
  extra: Record<string, unknown>;
  onExtraChange: (extra: Record<string, unknown>) => void;
  disabled?: boolean;
}

export function OpenAICapabilities({ extra, onExtraChange, disabled }: OpenAICapabilitiesProps) {
  const { t } = useLanguage();

  const capabilities = Array.isArray(extra.openai_capabilities)
    ? (extra.openai_capabilities as string[])
    : ["chat_completions", "embeddings"];

  const responsesMode = (extra.openai_responses_mode as string) ?? "auto";
  const wsMode = (extra.openai_ws_mode as string) ?? "off";

  function toggleCapability(cap: string, on: boolean) {
    const next = on ? [...new Set([...capabilities, cap])] : capabilities.filter((c) => c !== cap);
    const updated = { ...extra };
    if (next.length === 0 || (next.length === 2 && next.includes("chat_completions") && next.includes("embeddings"))) {
      delete updated.openai_capabilities;
    } else {
      updated.openai_capabilities = next;
    }
    onExtraChange(updated);
  }

  function setField(key: string, value: string) {
    const updated = { ...extra };
    if (!value || value === "auto" || value === "off") {
      delete updated[key];
    } else {
      updated[key] = value;
    }
    onExtraChange(updated);
  }

  return (
    <div className="rounded-lg border border-srapi-border p-3.5 space-y-3">
      <span className="text-sm font-medium text-srapi-text-primary">{t("adminAccounts.openaiCaps.title")}</span>

      <div className="space-y-2">
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={capabilities.includes("chat_completions")}
            onChange={(e) => toggleCapability("chat_completions", e.target.checked)}
            disabled={disabled || (capabilities.length <= 1 && capabilities.includes("chat_completions"))}
            className="rounded text-srapi-primary"
          />
          <span className="text-sm">{t("adminAccounts.openaiCaps.chatCompletions")}</span>
        </label>
        <label className="flex items-center gap-2 cursor-pointer">
          <input
            type="checkbox"
            checked={capabilities.includes("embeddings")}
            onChange={(e) => toggleCapability("embeddings", e.target.checked)}
            disabled={disabled || (capabilities.length <= 1 && capabilities.includes("embeddings"))}
            className="rounded text-srapi-primary"
          />
          <span className="text-sm">{t("adminAccounts.openaiCaps.embeddings")}</span>
        </label>
      </div>

      <div>
        <Label className="text-xs">{t("adminAccounts.openaiCaps.responsesMode")}</Label>
        <Select value={responsesMode} onValueChange={(v) => setField("openai_responses_mode", v)} disabled={disabled}>
          <SelectTrigger><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="auto">{t("adminAccounts.openaiCaps.auto")}</SelectItem>
            <SelectItem value="force_responses">Responses API</SelectItem>
            <SelectItem value="force_chat_completions">Chat Completions</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <div>
        <Label className="text-xs">{t("adminAccounts.openaiCaps.wsMode")}</Label>
        <Select value={wsMode} onValueChange={(v) => setField("openai_ws_mode", v)} disabled={disabled}>
          <SelectTrigger><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="off">{t("adminAccounts.openaiCaps.wsOff")}</SelectItem>
            <SelectItem value="ctx_pool">{t("adminAccounts.openaiCaps.wsCtxPool")}</SelectItem>
            <SelectItem value="passthrough">{t("adminAccounts.openaiCaps.wsPassthrough")}</SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}
