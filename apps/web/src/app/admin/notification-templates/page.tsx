"use client";

import { Fragment, useMemo, useState } from "react";
import { AdminShell } from "@/components/layout/admin-shell";
import { PageQueryState } from "@/components/layout/page-query-state";
import { SectionHero } from "@/components/visual/section-hero";
import { Card } from "@/components/ui/card";
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
import { DataPill } from "@/components/ui/data-pill";
import { DataTooltip } from "@/components/ui/data-tooltip";
import { SegmentedControl } from "@/components/ui/segmented-control";
import { SearchInput, ListToolbar } from "@/components/admin/list-toolbar";
import { ExpandableRow } from "@/components/ui/expandable-row";
import { IllustratedEmptyState } from "@/components/ui/illustrated-empty-state";
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
import { cn } from "@/lib/cn";
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

type CustomFilter = "__all__" | "custom" | "default";

function NotificationTemplatesContent() {
  const { t } = useLanguage();
  const query = useNotificationEmailTemplates();
  const [editing, setEditing] = useState<EditTarget | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState<CustomFilter>("__all__");

  // Pre-compute the filtered + counted view so the SectionHero metrics and
  // the table draw from the same source — no risk of drifting numbers.
  const view = useMemo(() => {
    if (!query.data) return null;
    const templateByEvent = new Map(query.data.templates.map((tpl) => [tpl.event, tpl]));
    const customCount = query.data.templates.filter((tpl) => tpl.is_custom).length;
    const defaultCount = query.data.events.length - customCount;
    const term = search.trim().toLowerCase();
    const rows = query.data.events
      .map((event) => ({ event, template: templateByEvent.get(event.event) }))
      .filter(({ event, template }) => {
        if (filter === "custom" && !template?.is_custom) return false;
        if (filter === "default" && template?.is_custom) return false;
        if (!term) return true;
        return [event.event, event.label, event.description, event.category]
          .filter(Boolean)
          .join(" ")
          .toLowerCase()
          .includes(term);
      });
    return { rows, customCount, defaultCount, placeholders: query.data.placeholders };
  }, [query.data, search, filter]);

  return (
    <>
      <SectionHero
        eyebrow={t("nav.sectionAdmin")}
        title={t("adminNotificationTemplates.title")}
        description={t("adminNotificationTemplates.subtitle")}
        metrics={
          query.data
            ? [
                { label: t("adminNotificationTemplates.event"), value: String(query.data.events.length) },
                {
                  label: t("adminNotificationTemplates.customizedCount"),
                  value: String(view?.customCount ?? 0),
                },
                {
                  label: t("adminNotificationTemplates.defaultCount"),
                  value: String(view?.defaultCount ?? 0),
                },
              ]
            : undefined
        }
      />
      <PageQueryState query={query} skeleton={<TemplatesSkeleton />}>
        {(list) => {
          if (list.events.length === 0) {
            return (
              <IllustratedEmptyState
                illust="bell"
                title={t("adminNotificationTemplates.emptyTitle")}
                description={t("adminNotificationTemplates.emptyBody")}
              />
            );
          }
          const rows = view?.rows ?? [];
          return (
            <Card className="overflow-hidden">
              <ListToolbar>
                <SearchInput
                  value={search}
                  onChange={setSearch}
                  placeholder={t("adminNotificationTemplates.searchPlaceholder")}
                />
                <SegmentedControl<CustomFilter>
                  value={filter}
                  onChange={setFilter}
                  ariaLabel={t("adminNotificationTemplates.status")}
                  size="sm"
                  options={[
                    { value: "__all__", label: t("adminNotificationTemplates.filter_all") },
                    { value: "custom", label: t("adminNotificationTemplates.filter_custom") },
                    { value: "default", label: t("adminNotificationTemplates.filter_default") },
                  ]}
                />
              </ListToolbar>
              {rows.length === 0 ? (
                <div className="p-8">
                  <IllustratedEmptyState
                    illust="search"
                    title={t("adminCommon.noResults")}
                    description={t("adminCommon.noResultsBody")}
                    action={
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => {
                          setSearch("");
                          setFilter("__all__");
                        }}
                      >
                        {t("adminCommon.clearFilters")}
                      </Button>
                    }
                  />
                </div>
              ) : (
                <TableScroll minWidth={720}>
                  <Table>
                    <TableHeader>
                      <tr>
                        <TableHead>{t("adminNotificationTemplates.event")}</TableHead>
                        <TableHead className="hidden sm:table-cell">
                          {t("adminNotificationTemplates.category")}
                        </TableHead>
                        <TableHead className="hidden md:table-cell">
                          {t("adminNotificationTemplates.placeholders")}
                        </TableHead>
                        <TableHead>{t("adminNotificationTemplates.status")}</TableHead>
                        <TableHead aria-label="actions" className="w-px" />
                      </tr>
                    </TableHeader>
                    <TableBody>
                      {rows.map(({ event, template }) => {
                        const isCustom = Boolean(template?.is_custom);
                        const isExpanded = expanded === event.event;
                        const isCustomized = template?.is_custom;
                        const placeholderCount = event.placeholders?.length ?? 0;
                        const htmlSize = template?.html?.length ?? 0;
                        return (
                          <Fragment key={event.event}>
                            <TableRow
                              aria-expanded={isExpanded}
                              data-sev={isCustom ? "success" : undefined}
                              className={cn(
                                "log-row cursor-pointer transition-colors",
                                isExpanded && "bg-srapi-card-muted/40",
                              )}
                              onClick={() =>
                                setExpanded((prev) => (prev === event.event ? null : event.event))
                              }
                            >
                              <TableCell>
                                <div className="text-srapi-text-primary">{event.label}</div>
                                {event.description ? (
                                  <div className="mt-0.5 text-xs text-srapi-text-tertiary">
                                    {event.description}
                                  </div>
                                ) : null}
                              </TableCell>
                              <TableCell className="hidden sm:table-cell">
                                {event.category ? (
                                  <DataPill tone="neutral" size="sm">
                                    {event.category}
                                  </DataPill>
                                ) : (
                                  <span className="text-xs text-srapi-text-tertiary">—</span>
                                )}
                              </TableCell>
                              <TableCell className="hidden md:table-cell">
                                <DataTooltip
                                  title={t("adminNotificationTemplates.placeholders")}
                                  primary={String(placeholderCount)}
                                  rows={
                                    placeholderCount > 0
                                      ? event.placeholders.slice(0, 6).map((name) => ({
                                          label: name,
                                          value: "",
                                          tone: "muted" as const,
                                        }))
                                      : undefined
                                  }
                                  footer={
                                    isCustomized
                                      ? t("adminNotificationTemplates.bytes", { count: htmlSize })
                                      : undefined
                                  }
                                >
                                  <span className="text-xs text-srapi-text-tertiary tabular">
                                    {placeholderCount}
                                  </span>
                                </DataTooltip>
                              </TableCell>
                              <TableCell>
                                <QuietBadge
                                  status={isCustom ? "active" : "disabled"}
                                  label={
                                    isCustom
                                      ? t("adminNotificationTemplates.custom")
                                      : t("adminNotificationTemplates.default")
                                  }
                                />
                              </TableCell>
                              <TableCell className="w-px whitespace-nowrap text-right">
                                <Button
                                  variant="outline"
                                  size="sm"
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    setEditing({
                                      event,
                                      template,
                                      placeholders: list.placeholders,
                                    });
                                  }}
                                >
                                  {t("common.edit")}
                                </Button>
                              </TableCell>
                            </TableRow>
                            {isExpanded ? (
                              <tr>
                                <td colSpan={5} className="p-0">
                                  <ExpandableRow expanded>
                                    <TemplatePreviewDetail
                                      event={event}
                                      template={template}
                                      globalPlaceholders={list.placeholders}
                                    />
                                  </ExpandableRow>
                                </td>
                              </tr>
                            ) : null}
                          </Fragment>
                        );
                      })}
                    </TableBody>
                  </Table>
                </TableScroll>
              )}
            </Card>
          );
        }}
      </PageQueryState>

      {editing ? <TemplateEditor target={editing} onClose={() => setEditing(null)} /> : null}
    </>
  );
}

function TemplatePreviewDetail({
  event,
  template,
  globalPlaceholders,
}: {
  event: NotificationEmailTemplateEvent;
  template?: NotificationEmailTemplate;
  globalPlaceholders: string[];
}) {
  const { t } = useLanguage();
  const placeholders = [...new Set([...(event.placeholders ?? []), ...globalPlaceholders])];
  const htmlSize = template?.html?.length ?? 0;
  return (
    <div className="border-t border-srapi-border/60 bg-srapi-card-muted/30 px-6 py-4 space-y-4">
      <div className="grid gap-x-8 gap-y-4 sm:grid-cols-2 lg:grid-cols-3">
        <div>
          <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            {t("adminNotificationTemplates.subject")}
          </div>
          <p className="text-sm text-srapi-text-primary">
            {template?.subject || <span className="text-srapi-text-tertiary">—</span>}
          </p>
        </div>
        <div>
          <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            {t("adminNotificationTemplates.category")}
          </div>
          <p className="text-sm text-srapi-text-primary">
            {event.category || <span className="text-srapi-text-tertiary">—</span>}
          </p>
        </div>
        <div>
          <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            {t("adminNotificationTemplates.htmlSize")}
          </div>
          <p className="metric-secondary tabular">
            {t("adminNotificationTemplates.bytes", { count: htmlSize })}
          </p>
        </div>
      </div>
      {placeholders.length > 0 ? (
        <div>
          <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            {t("adminNotificationTemplates.placeholders")}
          </div>
          <div className="flex flex-wrap gap-1.5">
            {placeholders.map((name) => (
              <DataPill key={name} tone="neutral" size="sm">
                {name}
              </DataPill>
            ))}
          </div>
        </div>
      ) : null}
      {template?.html ? (
        <div>
          <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            {t("adminNotificationTemplates.previewTitle")}
          </div>
          <iframe
            title={t("adminNotificationTemplates.previewTitle")}
            sandbox=""
            srcDoc={template.html}
            className="h-56 w-full rounded-xl border border-srapi-border/70 bg-white"
          />
          <p className="mt-1 text-xs text-srapi-text-tertiary">
            {t("adminNotificationTemplates.previewHint")}
          </p>
        </div>
      ) : null}
    </div>
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
    if (!subject.trim() || !html.trim()) {
      setError(t("adminCommon.required"));
      return;
    }
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
              <span className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                {t("adminNotificationTemplates.placeholders")}
              </span>
              <div className="mt-1.5 flex flex-wrap gap-1">
                {placeholders.map((name) => (
                  <code
                    key={name}
                    className="rounded-full bg-srapi-card-muted px-2 py-0.5 font-mono text-[11px] font-medium text-srapi-text-secondary"
                  >
                    {name}
                  </code>
                ))}
              </div>
            </div>
          ) : null}
          {preview ? (
            <div>
              <span className="text-xs font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
                {t("adminNotificationTemplates.previewTitle")}
              </span>
              <p className="mt-1.5 text-sm font-medium text-srapi-text-primary">{preview.subject}</p>
              <iframe
                title={t("adminNotificationTemplates.previewTitle")}
                sandbox=""
                srcDoc={preview.html}
                className="mt-1.5 h-64 w-full rounded-xl border border-srapi-border/70 bg-white"
              />
              <p className="mt-1 text-xs text-srapi-text-tertiary">
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
