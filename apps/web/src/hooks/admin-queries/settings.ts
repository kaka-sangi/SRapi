"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { adminApi } from "@/lib/admin-api";
import { queryKeys } from "@/lib/query-keys";
import { type B, type P, useAdminMutation } from "./_shared";

// ---- Announcements ----
export function useAdminAnnouncements(params?: P<typeof adminApi.listAnnouncements>) {
  return useQuery({
    queryKey: queryKeys.admin.announcements(params),
    queryFn: () => adminApi.listAnnouncements(params),
  });
}
export function useAnnouncementReadStatus(id: string | null, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.admin.announcementReads(id ?? ""),
    queryFn: () => adminApi.getAnnouncementReadStatus(id as string),
    enabled: enabled && Boolean(id),
  });
}

// ---- Error passthrough rules ----
export function useErrorPassthroughRules() {
  return useQuery({
    queryKey: queryKeys.admin.errorPassthroughRules(),
    queryFn: () => adminApi.listErrorPassthroughRules(),
  });
}

// ---- Payload transform rules ----
export function usePayloadRules() {
  return useQuery({
    queryKey: queryKeys.admin.payloadRules(),
    queryFn: () => adminApi.listPayloadRules(),
  });
}

// Custom RBAC roles (read + create; no PATCH/DELETE endpoint yet)
export function useAdminRoles() {
  return useQuery({
    queryKey: queryKeys.admin.roles(),
    queryFn: () => adminApi.listRoles(),
  });
}
export function useAdminPermissionCatalog() {
  return useQuery({
    queryKey: queryKeys.admin.permissionCatalog(),
    queryFn: () => adminApi.listPermissionCatalog(),
  });
}
export function useCreateAdminRole() {
  return useAdminMutation(
    (body: P<typeof adminApi.createRole>) => adminApi.createRole(body),
    ["admin", "roles"],
  );
}
export function useUpdateAdminRole() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateRole> }) =>
      adminApi.updateRole(vars.id, vars.body),
    ["admin", "roles"],
  );
}
export function useDeleteAdminRole() {
  return useAdminMutation((id: string) => adminApi.deleteRole(id), ["admin", "roles"]);
}
export function useAdminApiKeys(params?: P<typeof adminApi.listAdminApiKeys>) {
  return useQuery({
    queryKey: queryKeys.admin.apiKeys(params),
    queryFn: () => adminApi.listAdminApiKeys(params),
  });
}
export function useUpdateAdminApiKey() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateAdminApiKey> }) =>
      adminApi.updateAdminApiKey(vars.id, vars.body),
    ["admin", "api-keys"],
    queryKeys.admin.gatewayResources(),
  );
}
export function useResetAdminApiKeyUsage() {
  return useAdminMutation(
    (id: string) => adminApi.resetAdminApiKeyUsage(id),
    ["admin", "api-keys"],
    queryKeys.admin.gatewayResources(),
  );
}
export function useAdminApiKeyUsage(id: string | null, days: number, enabled: boolean) {
  return useQuery({
    queryKey: queryKeys.admin.apiKeyUsage(id ?? "", days),
    queryFn: () => adminApi.getAdminApiKeyUsage(id as string, days),
    enabled: enabled && Boolean(id),
  });
}

// ---- Notification email templates ----
export function useNotificationEmailTemplates() {
  return useQuery({
    queryKey: queryKeys.admin.notificationEmailTemplates(),
    queryFn: () => adminApi.listNotificationEmailTemplates(),
  });
}

// ---- Settings ----
export function useAdminSettings() {
  return useQuery({
    queryKey: queryKeys.admin.settings(),
    queryFn: () => adminApi.getSettings(),
  });
}

// ---- Admin AI copilot ----
export function useAdminCopilotConfig() {
  return useQuery({
    queryKey: queryKeys.admin.copilotConfig(),
    queryFn: () => adminApi.getCopilotConfig(),
  });
}

// ---- Risk control config (read) ----
export function useRiskConfig() {
  return useQuery({
    queryKey: queryKeys.admin.riskConfig(),
    queryFn: () => adminApi.getRiskConfig(),
  });
}

export function useContentSafetyConfig() {
  return useQuery({
    queryKey: queryKeys.admin.contentSafetyConfig(),
    queryFn: () => adminApi.getContentSafetyConfig(),
  });
}

export function useUpdateContentSafetyConfig() {
  return useAdminMutation(
    (body: P<typeof adminApi.updateContentSafetyConfig>) =>
      adminApi.updateContentSafetyConfig(body),
    ["admin", "content-safety-config"],
  );
}

// Announcements
export function useCreateAnnouncement() {
  return useAdminMutation(
    (body: P<typeof adminApi.createAnnouncement>) => adminApi.createAnnouncement(body),
    ["admin", "announcements"],
  );
}
export function useUpdateAnnouncement() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateAnnouncement> }) =>
      adminApi.updateAnnouncement(vars.id, vars.body),
    ["admin", "announcements"],
  );
}
export function useDeleteAnnouncement() {
  return useAdminMutation(
    (id: string) => adminApi.deleteAnnouncement(id),
    ["admin", "announcements"],
  );
}

// Error passthrough rules
export function useCreateErrorPassthroughRule() {
  return useAdminMutation(
    (body: P<typeof adminApi.createErrorPassthroughRule>) =>
      adminApi.createErrorPassthroughRule(body),
    ["admin", "error-passthrough-rules"],
  );
}
export function useUpdateErrorPassthroughRule() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updateErrorPassthroughRule> }) =>
      adminApi.updateErrorPassthroughRule(vars.id, vars.body),
    ["admin", "error-passthrough-rules"],
  );
}
export function useDeleteErrorPassthroughRule() {
  return useAdminMutation(
    (id: string) => adminApi.deleteErrorPassthroughRule(id),
    ["admin", "error-passthrough-rules"],
  );
}

// Payload transform rules
export function useCreatePayloadRule() {
  return useAdminMutation(
    (body: P<typeof adminApi.createPayloadRule>) => adminApi.createPayloadRule(body),
    ["admin", "payload-rules"],
  );
}
export function useUpdatePayloadRule() {
  return useAdminMutation(
    (vars: { id: string; body: B<typeof adminApi.updatePayloadRule> }) =>
      adminApi.updatePayloadRule(vars.id, vars.body),
    ["admin", "payload-rules"],
  );
}
export function useDeletePayloadRule() {
  return useAdminMutation(
    (id: string) => adminApi.deletePayloadRule(id),
    ["admin", "payload-rules"],
  );
}

// Notification email templates
export function useUpdateNotificationEmailTemplate() {
  return useAdminMutation(
    (vars: {
      event: P<typeof adminApi.updateNotificationEmailTemplate>;
      body: B<typeof adminApi.updateNotificationEmailTemplate>;
    }) => adminApi.updateNotificationEmailTemplate(vars.event, vars.body),
    ["admin", "notification-email-templates"],
  );
}
export function useRestoreNotificationEmailTemplate() {
  return useAdminMutation(
    (event: P<typeof adminApi.restoreNotificationEmailTemplate>) =>
      adminApi.restoreNotificationEmailTemplate(event),
    ["admin", "notification-email-templates"],
  );
}

// Settings
export function useUpdateSettings() {
  return useAdminMutation(
    (body: P<typeof adminApi.updateSettings>) => adminApi.updateSettings(body),
    ["admin", "settings"],
    queryKeys.admin.gatewayResources(),
  );
}

// Send a probe email to verify SMTP credentials. Returns an AdminTestResult; it
// changes no server state, so there is nothing to invalidate.
export function useSendTestEmail() {
  return useMutation({
    mutationFn: (body?: P<typeof adminApi.sendTestEmail>) => adminApi.sendTestEmail(body),
  });
}

// Config snapshot (backup / restore)
export function useConfigSnapshot() {
  return useQuery({
    queryKey: queryKeys.admin.configSnapshot(),
    queryFn: () => adminApi.getConfigSnapshot(),
    enabled: false, // fetched on demand from the Backup tab
  });
}
export function useImportConfigSnapshot() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { body: P<typeof adminApi.importConfigSnapshot>; dryRun?: boolean }) =>
      adminApi.importConfigSnapshot(vars.body, vars.dryRun),
    onSuccess: (_data, vars) => {
      // A real (non-dry-run) restore can replace providers, models, accounts,
      // settings, etc. Refresh the whole admin cache instead of leaving every
      // admin view showing pre-restore data.
      if (!vars.dryRun) {
        qc.invalidateQueries({ queryKey: ["admin"] });
      }
    },
  });
}

// Database backup history (list, trigger, delete). The backup tab calls
// useAdminBackupSnapshots to render the table, useTriggerAdminBackup for
// the "Snapshot now" button, and useDeleteAdminBackup for the per-row
// delete action. Download streams through admin-api directly — it's not a
// react-query call because the browser handles the file save.
export function useAdminBackupSnapshots(params?: P<typeof adminApi.listBackupSnapshots>) {
  return useQuery({
    queryKey: queryKeys.admin.backupSnapshots(params),
    queryFn: () => adminApi.listBackupSnapshots(params),
  });
}

export function useTriggerAdminBackup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => adminApi.triggerBackupSnapshot(),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "backup-snapshots"] });
    },
  });
}

export function useDeleteAdminBackup() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: Parameters<typeof adminApi.deleteBackupSnapshot>[0]) =>
      adminApi.deleteBackupSnapshot(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "backup-snapshots"] });
    },
  });
}
