"use client";

import { useCallback, useRef, useState } from "react";
import { Upload } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useLanguage } from "@/context/LanguageContext";

const VERTEX_LOCATIONS = [
  "global", "us-central1", "us-east1", "us-east4", "us-west1", "us-west4",
  "europe-west1", "europe-west4", "asia-east1", "asia-northeast1", "asia-southeast1",
  "australia-southeast1", "northamerica-northeast1",
];

interface VertexInputProps {
  disabled?: boolean;
  onCredential: (cred: Record<string, unknown>) => void;
}

export function VertexInput({ disabled, onCredential }: VertexInputProps) {
  const { t } = useLanguage();
  const [saJson, setSaJson] = useState("");
  const [projectId, setProjectId] = useState("");
  const [clientEmail, setClientEmail] = useState("");
  const [location, setLocation] = useState("global");
  const [dragActive, setDragActive] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);

  const applySaJson = useCallback((raw: string) => {
    setSaJson(raw);
    try {
      const parsed = JSON.parse(raw.trim());
      if (parsed.project_id) setProjectId(parsed.project_id);
      if (parsed.client_email) setClientEmail(parsed.client_email);
      onCredential({
        service_account_json: raw.trim(),
        project_id: parsed.project_id ?? projectId,
        client_email: parsed.client_email ?? clientEmail,
        location,
      });
    } catch {
      // invalid JSON, let user fix it
    }
  }, [clientEmail, location, onCredential, projectId]);

  function handleFile(file: File) {
    const reader = new FileReader();
    reader.onload = (e) => {
      const text = e.target?.result;
      if (typeof text === "string") applySaJson(text);
    };
    reader.readAsText(file);
  }

  function handleDrop(e: React.DragEvent) {
    e.preventDefault();
    setDragActive(false);
    const file = e.dataTransfer.files[0];
    if (file) handleFile(file);
  }

  return (
    <div className="space-y-4">
      <div>
        <Label>{t("adminAccounts.vertex.saJson")}</Label>
        <div
          className={`mt-1.5 rounded-lg border-2 border-dashed p-4 text-center transition-colors ${dragActive ? "border-srapi-primary bg-srapi-primary/5" : "border-srapi-border"}`}
          onDragOver={(e) => { e.preventDefault(); setDragActive(true); }}
          onDragLeave={() => setDragActive(false)}
          onDrop={handleDrop}
        >
          <Upload className="mx-auto size-8 text-srapi-text-tertiary" />
          <p className="mt-2 text-sm text-srapi-text-secondary">{t("adminAccounts.vertex.dropHint")}</p>
          <button type="button" className="mt-1 text-xs text-srapi-primary hover:underline" onClick={() => fileRef.current?.click()}>
            {t("adminAccounts.vertex.browse")}
          </button>
          <input ref={fileRef} type="file" accept=".json" className="hidden" onChange={(e) => { const f = e.target.files?.[0]; if (f) handleFile(f); }} />
        </div>
        <Textarea
          className="mt-2 min-h-20 font-mono text-xs"
          placeholder='{"type": "service_account", "project_id": "...", ...}'
          value={saJson}
          disabled={disabled}
          spellCheck={false}
          onChange={(e) => applySaJson(e.target.value)}
        />
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div>
          <Label htmlFor="vertex-project-id">Project ID</Label>
          <Input id="vertex-project-id" className="mt-1.5 font-mono" value={projectId} disabled={disabled} onChange={(e) => setProjectId(e.target.value)} />
        </div>
        <div>
          <Label htmlFor="vertex-client-email">Client Email</Label>
          <Input id="vertex-client-email" className="mt-1.5 font-mono text-xs" value={clientEmail} disabled={disabled} onChange={(e) => setClientEmail(e.target.value)} />
        </div>
      </div>

      <div>
        <Label>{t("adminAccounts.vertex.location")}</Label>
        <Select value={location} onValueChange={setLocation} disabled={disabled}>
          <SelectTrigger><SelectValue /></SelectTrigger>
          <SelectContent>
            {VERTEX_LOCATIONS.map((loc) => (
              <SelectItem key={loc} value={loc}>{loc}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}
