"use client";

import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useLanguage } from "@/context/LanguageContext";

const CLIENT_PRESETS = [
  { key: "codex_cli", label: "Codex CLI" },
  { key: "claude_code", label: "Claude Code" },
  { key: "gemini_cli", label: "Gemini CLI" },
] as const;

interface AllowedClientsProps {
  metadata: Record<string, unknown>;
  onMetadataChange: (metadata: Record<string, unknown>) => void;
  disabled?: boolean;
}

export function AllowedClients({ metadata, onMetadataChange, disabled }: AllowedClientsProps) {
  const { t } = useLanguage();
  const current = Array.isArray(metadata.allowed_clients)
    ? (metadata.allowed_clients as string[])
    : [];
  const enabled = current.length > 0;

  function toggle(key: string, on: boolean) {
    const next = on ? [...new Set([...current, key])] : current.filter((c) => c !== key);
    const updated = { ...metadata };
    if (next.length === 0) {
      delete updated.allowed_clients;
    } else {
      updated.allowed_clients = next;
    }
    onMetadataChange(updated);
  }

  function toggleAll(on: boolean) {
    const updated = { ...metadata };
    if (!on) {
      delete updated.allowed_clients;
    } else {
      updated.allowed_clients = CLIENT_PRESETS.map((p) => p.key);
    }
    onMetadataChange(updated);
  }

  return (
    <div className="rounded-lg border border-srapi-border p-3.5 space-y-3">
      <div className="flex items-center justify-between">
        <div>
          <span className="text-sm font-medium text-srapi-text-primary">{t("adminAccounts.allowedClients.title")}</span>
          <p className="text-[11px] text-srapi-text-tertiary">{t("adminAccounts.allowedClients.hint")}</p>
        </div>
        <Switch checked={enabled} onCheckedChange={toggleAll} disabled={disabled} />
      </div>
      {enabled ? (
        <div className="space-y-2">
          {CLIENT_PRESETS.map((preset) => (
            <label key={preset.key} className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={current.includes(preset.key)}
                onChange={(e) => toggle(preset.key, e.target.checked)}
                disabled={disabled}
                className="rounded text-srapi-primary"
              />
              <span className="text-sm">{preset.label}</span>
            </label>
          ))}
        </div>
      ) : null}
    </div>
  );
}
