"use client";

import { useState } from "react";
import { Mail } from "lucide-react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import { Card } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableScroll,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui/table";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog";
import {
  useNotificationEmailTemplates,
  useUpdateNotificationEmailTemplate,
  useRestoreNotificationEmailTemplate,
} from "@/hooks/admin-queries";
import { adminApi, adminErrorMessage } from "@/lib/admin-api";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import type {
  NotificationEmailTemplate,
  NotificationEmailTemplateEvent,
} from "@/lib/sdk-types";

export default function AdminNotificationTemplatesPage() {
  return (
    <AdminShell>
      <NotificationTemplatesContent />
    </AdminShell>
  );
}

interface EditTarget {
  event: NotificationEmailTemplateEvent;
  template?: NotificationEmailTemplate;
  placeholders: string[];
}

function NotificationTemplatesContent() {
  const { t } = useLanguage();
  const query = useNotificationEmailTemplates();
  const [editing, setEditing] = useState<EditTarget | null>(null);

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminNotificationTemplates.title")}
        description={t("adminNotificationTemplates.subtitle")}
      />
      <PageQueryState query={query} skeleton={<TemplatesSkeleton />}>
        {(list) => {
          const templateByEvent = new Map(list.templates.map((tpl) => [tpl.event, tpl]));
          if (list.events.length === 0) {
            return (
              <EmptyState
                icon={Mail}
                title={t("adminNotificationTemplates.emptyTitle")}
                description={t("adminNotificationTemplates.emptyBody")}
              />
            );
          }
          return (
            <Card className="overflow-hidden">
              <TableScroll minWidth={640}>
                <Table>
                  <TableHeader>
                    <tr>
                      <TableHead>{t("adminNotificationTemplates.event")}</TableHead>
                      <TableHead className="hidden sm:table-cell">
                        {t("adminNotificationTemplates.category")}
                      </TableHead>
                      <TableHead>{t("adminNotificationTemplates.status")}</TableHead>
                      <TableHead aria-label="actions" className="w-px" />
                    </tr>
                  </TableHeader>
                  <TableBody>
                    {list.events.map((event) => {
                      const template = templateByEvent.get(event.event);
                      return (
                        <TableRow key={event.event}>
                          <TableCell>
                            <div className="text-srapi-text-primary">{event.label}</div>
                            {event.description ? (
                              <div className="mt-0.5 text-2xs text-srapi-text-tertiary">
                                {event.description}
                              </div>
                            ) : null}
                          </TableCell>
                          <TableCell className="hidden sm:table-cell">
                            <span className="font-mono text-2xs text-srapi-text-tertiary">
                              {event.category || "—"}
                            </span>
                          </TableCell>
                          <TableCell>
                            <QuietBadge
                              status={template?.is_custom ? "active" : "disabled"}
                              label={
                                template?.is_custom
                                  ? t("adminNotificationTemplates.custom")
                                  : t("adminNotificationTemplates.default")
                              }
                            />
                          </TableCell>
                          <TableCell className="w-px whitespace-nowrap text-right">
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() =>
                                setEditing({ event, template, placeholders: list.placeholders })
                              }
                            >
                              {t("common.edit")}
                            </Button>
                          </TableCell>
                        </TableRow>
                      );
                    })}
                  </TableBody>
                </Table>
              </TableScroll>
            </Card>
          );
        }}
      </PageQueryState>

      {editing ? <TemplateEditor target={editing} onClose={() => setEditing(null)} /> : null}
    </>
  );
}

function TemplateEditor({ target, onClose }: { target: EditTarget; onClose: () => void }) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const updateMut = useUpdateNotificationEmailTemplate();
  const restoreMut = useRestoreNotificationEmailTemplate();

  const { event, template } = target;
  const [subject, setSubject] = useState(template?.subject ?? "");
  const [html, setHtml] = useState(template?.html ?? "");
  const [preview, setPreview] = useState<{ subject: string; html: string } | null>(null);
  const [previewing, setPreviewing] = useState(false);
  const [confirmRestore, setConfirmRestore] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const placeholders = [...new Set([...(event.placeholders ?? []), ...target.placeholders])];
  const busy = updateMut.isPending || restoreMut.isPending;

  async function handlePreview() {
    setError(null);
    setPreviewing(true);
    try {
      setPreview(
        await adminApi.previewNotificationEmailTemplate({ event: event.event, subject, html }),
      );
    } catch (err) {
      setError(adminErrorMessage(err));
    } finally {
      setPreviewing(false);
    }
  }

  async function handleSave() {
    setError(null);
    try {
      await updateMut.mutateAsync({ event: event.event, body: { subject: subject.trim(), html } });
      toast({ title: t("feedback.updated"), tone: "success" });
      onClose();
    } catch (err) {
      setError(adminErrorMessage(err));
    }
  }

  async function handleRestore() {
    if (!confirmRestore) {
      setConfirmRestore(true);
      return;
    }
    setError(null);
    try {
      const restored = await restoreMut.mutateAsync(event.event);
      setSubject(restored.subject);
      setHtml(restored.html);
      setPreview(null);
      toast({ title: t("feedback.updated"), tone: "success" });
    } catch (err) {
      setError(adminErrorMessage(err));
    } finally {
      setConfirmRestore(false);
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{event.label}</DialogTitle>
          <DialogDescription>
            {event.description || t("adminNotificationTemplates.edit")}
          </DialogDescription>
        </DialogHeader>
        <div className="mt-2 max-h-[64vh] space-y-4 overflow-y-auto pr-1">
          <div>
            <Label htmlFor="tmpl-subject">{t("adminNotificationTemplates.subject")}</Label>
            <Input
              id="tmpl-subject"
              value={subject}
              disabled={busy}
              onChange={(e) => setSubject(e.target.value)}
            />
          </div>
          <div>
            <Label htmlFor="tmpl-html">{t("adminNotificationTemplates.html")}</Label>
            <Textarea
              id="tmpl-html"
              value={html}
              disabled={busy}
              spellCheck={false}
              className="min-h-48 font-mono text-xs"
              onChange={(e) => setHtml(e.target.value)}
            />
          </div>
          {placeholders.length > 0 ? (
            <div>
              <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">
                {t("adminNotificationTemplates.placeholders")}
              </span>
              <div className="mt-1.5 flex flex-wrap gap-1">
                {placeholders.map((name) => (
                  <code
                    key={name}
                    className="rounded border border-srapi-border px-1.5 py-0.5 font-mono text-2xs text-srapi-text-secondary"
                  >
                    {name}
                  </code>
                ))}
              </div>
            </div>
          ) : null}
          {preview ? (
            <div>
              <span className="font-mono text-2xs uppercase text-srapi-text-tertiary">
                {t("adminNotificationTemplates.previewTitle")}
              </span>
              <p className="mt-1.5 text-sm font-medium text-srapi-text-primary">{preview.subject}</p>
              <iframe
                title={t("adminNotificationTemplates.previewTitle")}
                sandbox=""
                srcDoc={preview.html}
                className="mt-1.5 h-64 w-full rounded-lg border border-srapi-border bg-white"
              />
              <p className="mt-1 text-2xs text-srapi-text-tertiary">
                {t("adminNotificationTemplates.previewHint")}
              </p>
            </div>
          ) : null}
          {error ? (
            <p role="alert" className="text-sm text-srapi-error">
              {error}
            </p>
          ) : null}
        </div>
        <DialogFooter className="mt-6 sm:justify-between">
          <Button
            type="button"
            variant={confirmRestore ? "danger" : "ghost"}
            disabled={busy}
            loading={restoreMut.isPending}
            title={t("adminNotificationTemplates.restoreHint")}
            onClick={handleRestore}
          >
            {confirmRestore
              ? t("adminNotificationTemplates.restoreConfirm")
              : t("adminNotificationTemplates.restore")}
          </Button>
          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={busy}
              loading={previewing}
              onClick={handlePreview}
            >
              {t("adminNotificationTemplates.preview")}
            </Button>
            <Button
              type="button"
              variant="primary"
              loading={updateMut.isPending}
              disabled={busy}
              onClick={handleSave}
            >
              {t("common.save")}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function TemplatesSkeleton() {
  return (
    <Card className="space-y-2 p-5">
      {Array.from({ length: 6 }).map((_, i) => (
        <Skeleton key={i} className="h-10" />
      ))}
    </Card>
  );
}
