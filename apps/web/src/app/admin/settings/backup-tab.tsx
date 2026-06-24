import { useState } from "react";
import {
  Copy,
  Check,
  Download,
  Trash2,
  RefreshCw,
  Database,
  Upload,
  ArchiveRestore,
} from "lucide-react";
import {
  useAdminBackupSnapshots,
  useConfigSnapshot,
  useDeleteAdminBackup,
  useImportConfigSnapshot,
  useTriggerAdminBackup,
} from "@/hooks/admin-queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { writeClipboard } from "@/components/ui/copy-button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { SectionTitle } from "@/components/ui/section-title";
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { DataPill } from "@/components/ui/data-pill";
import { Kbd } from "@/components/ui/kbd";
import { adminApi, adminErrorMessage } from "@/lib/admin-api";
import type { BackupSnapshot } from "../../../../../../packages/sdk/typescript/src/types.gen";

/** Backup tab: export the full config snapshot as JSON, or import one (dry-run first). */
export function BackupTab() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const snapshot = useConfigSnapshot();
  const importMut = useImportConfigSnapshot();
  const [importText, setImportText] = useState("");
  const [dryRun, setDryRun] = useState(true);
  const [result, setResult] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

  const snapshotJson = snapshot.data ? JSON.stringify(snapshot.data, null, 2) : "";

  async function copySnapshot() {
    if (!snapshotJson) return;
    const ok = await writeClipboard(snapshotJson);
    if (ok) {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    }
  }

  async function runImport() {
    setResult(null);
    let body;
    try {
      body = JSON.parse(importText || "{}");
    } catch {
      toast({ title: t("feedback.failed"), description: t("feedback.invalidJson"), tone: "error" });
      return;
    }
    try {
      const res = await importMut.mutateAsync({ body, dryRun });
      setResult(JSON.stringify(res, null, 2));
      toast({ title: dryRun ? t("adminSettings.dryRun") : t("feedback.saved"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  // Mark the import textarea as «dirty» whenever it diverges from empty —
  // mirrors the SectionFields convention so operators see at a glance which
  // panel has unsaved changes.
  const importDirty = importText.trim().length > 0;

  return (
    <div className="space-y-6">
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardContent className="space-y-3">
            <SectionTitle
              icon={<ArchiveRestore />}
              label={t("adminSettings.export")}
              action={
                <>
                  <Button variant="outline" size="sm" onClick={() => snapshot.refetch()} loading={snapshot.isFetching}>
                    {t("adminSettings.fetchSnapshot")}
                  </Button>
                  {snapshotJson ? (
                    <Button variant="outline" size="sm" onClick={copySnapshot}>
                      {copied ? <Check className="size-4 text-srapi-success" /> : <Copy className="size-4" />}
                      {t("common.copy")}
                    </Button>
                  ) : null}
                </>
              }
            />
            <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.exportHint")}</p>
            <Textarea
              readOnly
              className="min-h-64 font-mono text-xs"
              value={snapshotJson}
              placeholder={t("adminSettings.fetchSnapshot")}
            />
            {snapshotJson ? (
              <p className="flex items-center gap-1.5 text-[11px] text-srapi-text-tertiary">
                <Kbd>⌘</Kbd>
                <Kbd>C</Kbd>
                <span>to copy after selecting</span>
              </p>
            ) : null}
          </CardContent>
        </Card>

        <Card>
          <CardContent className="space-y-3">
            <SectionTitle
              icon={<Upload />}
              label={t("adminSettings.import")}
              action={
                importDirty ? (
                  // 2.5px-style severity hint reused as a tiny dirty pill on
                  // the import panel; subtle enough not to compete with the
                  // sticky top save bar on sibling tabs.
                  <DataPill tone="warning" size="sm">
                    ●&nbsp;dirty
                  </DataPill>
                ) : undefined
              }
            />
            <p className="text-xs text-srapi-text-tertiary">{t("adminSettings.importHint")}</p>
            <Textarea
              className="min-h-48 font-mono text-xs"
              spellCheck={false}
              value={importText}
              onChange={(e) => setImportText(e.target.value)}
              placeholder='{ "providers": [], "models": [] }'
            />
            <div className="flex items-center justify-between gap-4">
              <label className="flex items-center gap-2 text-sm text-srapi-text-secondary">
                <Switch checked={dryRun} onCheckedChange={setDryRun} />
                {t("adminSettings.dryRun")}
              </label>
              <Button
                variant={dryRun ? "outline" : "primary"}
                size="sm"
                loading={importMut.isPending}
                disabled={!importText.trim()}
                onClick={runImport}
              >
                {dryRun ? t("adminSettings.dryRun") : t("adminSettings.applyImport")}
              </Button>
            </div>
            {result ? (
              <div>
                <Label>{t("adminSettings.importResult")}</Label>
                <Textarea readOnly className="min-h-32 font-mono text-xs" value={result} />
              </div>
            ) : null}
          </CardContent>
        </Card>
      </div>

      <DatabaseBackupsSection />
    </div>
  );
}

// DatabaseBackupsSection is the operator-facing history table for the daily
// pg_dump worker. Distinct from the config-JSON snapshot cards above: those
// export provider/model configuration, this one lists actual database dumps.
function DatabaseBackupsSection() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const list = useAdminBackupSnapshots({ limit: 50 });
  const triggerMut = useTriggerAdminBackup();
  const deleteMut = useDeleteAdminBackup();

  async function onSnapshotNow() {
    try {
      await triggerMut.mutateAsync();
      toast({ title: t("feedback.saved"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  async function onDownload(row: BackupSnapshot) {
    try {
      await adminApi.downloadBackupSnapshot(row.id);
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  async function onDelete(row: BackupSnapshot) {
    if (typeof window !== "undefined") {
      const ok = window.confirm(t("adminSettings.snapshotDeleteConfirmBody"));
      if (!ok) return;
    }
    try {
      await deleteMut.mutateAsync(row.id);
      toast({ title: t("feedback.saved"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: adminErrorMessage(err), tone: "error" });
    }
  }

  const rows = list.data?.data ?? [];

  // Roll-up counts for the SectionTitle DataTooltip — at-a-glance breakdown
  // of snapshot health without forcing operators to scan the full table.
  const successCount = rows.filter((r) => r.status === "success").length;
  const failedCount = rows.filter((r) => r.status === "failed").length;
  const runningCount = rows.filter((r) => r.status === "running").length;
  const supersededCount = rows.filter((r) => r.status === "superseded").length;

  return (
    <Card>
      <CardContent className="space-y-3">
        <SectionTitle
          icon={<Database />}
          label={
            <DataTooltip
              title={t("adminSettings.databaseBackups")}
              primary={`${rows.length} snapshot${rows.length === 1 ? "" : "s"}`}
              rows={[
                { label: t("adminSettings.snapshotStatus") + " · success", value: successCount, tone: "success" },
                { label: t("adminSettings.snapshotStatus") + " · failed", value: failedCount, tone: "error" },
                { label: t("adminSettings.snapshotStatus") + " · running", value: runningCount, tone: "warning" },
                { label: t("adminSettings.snapshotStatus") + " · superseded", value: supersededCount, tone: "muted" },
              ]}
            >
              <span className="cursor-help underline decoration-srapi-border decoration-dotted underline-offset-4">
                {t("adminSettings.databaseBackups")}
              </span>
            </DataTooltip>
          }
          action={
            <>
              <Button
                variant="outline"
                size="sm"
                onClick={() => list.refetch()}
                loading={list.isFetching}
              >
                <RefreshCw className="size-4" />
              </Button>
              <Button
                variant="primary"
                size="sm"
                onClick={onSnapshotNow}
                loading={triggerMut.isPending}
              >
                {t("adminSettings.snapshotNow")}
              </Button>
            </>
          }
        />

        {rows.length === 0 ? (
          <IllustratedEmptyState
            illust="cog"
            title={t("adminSettings.snapshotEmpty")}
            description={t("adminSettings.snapshotEmptyBody")}
            action={
              <Button
                variant="primary"
                size="sm"
                onClick={onSnapshotNow}
                loading={triggerMut.isPending}
              >
                {t("adminSettings.snapshotNow")}
              </Button>
            }
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm">
              <thead>
                <tr className="border-b border-srapi-border/70 text-left text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                  <th className="py-2 pr-3">{t("adminSettings.snapshotStarted")}</th>
                  <th className="py-2 pr-3">{t("adminSettings.snapshotStatus")}</th>
                  <th className="py-2 pr-3">{t("adminSettings.snapshotKindScheduled")}/{t("adminSettings.snapshotKindManual")}</th>
                  <th className="py-2 pr-3">{t("adminSettings.snapshotSize")}</th>
                  <th className="py-2 pr-3">{t("adminSettings.snapshotChecksum")}</th>
                  <th className="py-2 pr-3 text-right">Actions</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <BackupRow
                    key={row.id}
                    row={row}
                    onDownload={() => onDownload(row)}
                    onDelete={() => onDelete(row)}
                    busy={deleteMut.isPending}
                  />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function BackupRow({
  row,
  onDownload,
  onDelete,
  busy,
}: {
  row: BackupSnapshot;
  onDownload: () => void;
  onDelete: () => void;
  busy: boolean;
}) {
  const { t } = useLanguage();
  const [copiedChecksum, setCopiedChecksum] = useState(false);
  const downloadable = row.status === "success";
  const startedAt = row.started_at ? new Date(row.started_at) : null;

  const kindLabel =
    row.kind === "manual"
      ? t("adminSettings.snapshotKindManual")
      : t("adminSettings.snapshotKindScheduled");

  async function copyChecksum() {
    if (!row.sha256) return;
    if (await writeClipboard(row.sha256)) {
      setCopiedChecksum(true);
      setTimeout(() => setCopiedChecksum(false), 1500);
    }
  }

  // Severity drives the 2.5px left stripe via .log-row — error/warn/info map
  // directly from status; «superseded» is treated as info (historical) so it
  // doesn't compete visually with active failures.
  const severity: "success" | "error" | "warning" | "info" =
    row.status === "success"
      ? "success"
      : row.status === "failed"
        ? "error"
        : row.status === "running"
          ? "warning"
          : "info";

  return (
    <tr
      className="log-row border-b border-srapi-border/70 transition-colors hover:bg-srapi-card-muted/50"
      data-sev={severity}
    >
      <td className="py-3 pr-3 text-[12px] tabular text-srapi-text-secondary">
        {startedAt ? (
          <DataTooltip
            title={t("adminSettings.snapshotStarted")}
            primary={startedAt.toLocaleString()}
            rows={[
              { label: t("adminBackup.iso"), value: startedAt.toISOString() },
              { label: t("adminBackup.kind"), value: kindLabel },
            ]}
          >
            <span className="cursor-help">{startedAt.toLocaleString()}</span>
          </DataTooltip>
        ) : (
          "—"
        )}
      </td>
      <td className="py-3 pr-3">
        <StatusBadge status={row.status} />
      </td>
      <td className="py-3 pr-3 text-xs text-srapi-text-secondary">{kindLabel}</td>
      <td className="py-3 pr-3 text-[12px] tabular text-srapi-text-secondary">
        <DataTooltip
          title={t("adminSettings.snapshotSize")}
          primary={formatBytes(row.size_bytes)}
          rows={[
            { label: t("adminBackup.bytes"), value: row.size_bytes ?? 0 },
            { label: t("adminBackup.kind"), value: kindLabel },
          ]}
        >
          <span className="cursor-help">{formatBytes(row.size_bytes)}</span>
        </DataTooltip>
      </td>
      <td className="py-3 pr-3">
        {row.sha256 ? (
          <button
            type="button"
            onClick={copyChecksum}
            className="inline-flex items-center gap-1 rounded-full bg-srapi-card-muted px-2 py-0.5 font-mono text-[11px] font-medium text-srapi-text-secondary transition-colors hover:text-srapi-text-primary"
            title={row.sha256}
          >
            <span>{row.sha256.slice(0, 12)}…</span>
            {copiedChecksum ? <Check className="size-3 text-srapi-success" /> : <Copy className="size-3" />}
          </button>
        ) : (
          <span className="text-xs text-srapi-text-tertiary">—</span>
        )}
      </td>
      <td className="py-3 pr-3">
        <div className="flex justify-end gap-2">
          <Button variant="outline" size="sm" disabled={!downloadable} onClick={onDownload}>
            <Download className="size-4" />
            {t("adminSettings.snapshotDownload")}
          </Button>
          <Button variant="outline" size="sm" onClick={onDelete} loading={busy}>
            <Trash2 className="size-4 text-srapi-error" />
            {t("adminSettings.snapshotDelete")}
          </Button>
        </div>
      </td>
    </tr>
  );
}

function StatusBadge({ status }: { status: BackupSnapshot["status"] }) {
  switch (status) {
    case "success":
      return <Badge variant="success">{status}</Badge>;
    case "failed":
      return <Badge variant="danger">{status}</Badge>;
    case "superseded":
      return <Badge variant="warning">{status}</Badge>;
    case "running":
    default:
      return <Badge variant="info">{status}</Badge>;
  }
}

function formatBytes(size: number): string {
  if (!size || size <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let n = size;
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n.toFixed(n >= 100 || i === 0 ? 0 : 1)} ${units[i]}`;
}
