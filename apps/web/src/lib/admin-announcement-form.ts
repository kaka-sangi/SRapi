import type {
  Announcement,
  AnnouncementAudience,
  AnnouncementSeverity,
  AnnouncementStatus,
  CreateAnnouncementRequest,
} from "../../../../packages/sdk/typescript/src/types.gen";

export const ANNOUNCEMENT_STATUSES: AnnouncementStatus[] = ["draft", "published", "archived"];
export const ANNOUNCEMENT_SEVERITIES: AnnouncementSeverity[] = ["info", "warning", "critical"];
export const ANNOUNCEMENT_AUDIENCES: AnnouncementAudience[] = ["all", "users", "admins"];

export interface AnnouncementFormState {
  title: string;
  content: string;
  status: AnnouncementStatus;
  severity: AnnouncementSeverity;
  audience: AnnouncementAudience;
}

export interface AnnouncementDeleteState {
  id: string;
  title: string;
  confirmation: string;
}

export function emptyAnnouncementForm(): AnnouncementFormState {
  return {
    title: "",
    content: "",
    status: "draft",
    severity: "info",
    audience: "all",
  };
}

export function announcementFormFromAnnouncement(announcement: Announcement): AnnouncementFormState {
  return {
    title: announcement.title,
    content: announcement.content,
    status: announcement.status,
    severity: announcement.severity,
    audience: announcement.audience,
  };
}

export function buildAnnouncementBody(form: AnnouncementFormState): CreateAnnouncementRequest {
  return {
    title: requiredText(form.title, "Title"),
    content: requiredText(form.content, "Content"),
    status: form.status,
    severity: form.severity,
    audience: form.audience,
  };
}

export function deleteStateFromAnnouncement(announcement: Announcement): AnnouncementDeleteState {
  return {
    id: announcement.id,
    title: announcement.title,
    confirmation: "",
  };
}

export function canDeleteAnnouncement(state: AnnouncementDeleteState | null): boolean {
  return Boolean(state && state.confirmation.trim() === state.title);
}

function requiredText(value: string, fieldName: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    throw new Error(`${fieldName} is required.`);
  }
  return trimmed;
}
