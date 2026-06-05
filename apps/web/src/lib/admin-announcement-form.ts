import type {
  Announcement,
  AnnouncementAudience,
  AnnouncementSegment,
  AnnouncementSeverity,
  AnnouncementStatus,
  CreateAnnouncementRequest,
  Id,
} from "../../../../packages/sdk/typescript/src/types.gen";

export const ANNOUNCEMENT_STATUSES: AnnouncementStatus[] = ["draft", "published", "archived"];
export const ANNOUNCEMENT_SEVERITIES: AnnouncementSeverity[] = ["info", "warning", "critical"];
export const ANNOUNCEMENT_AUDIENCES: AnnouncementAudience[] = ["all", "users", "admins"];

export const ANNOUNCEMENT_SEGMENT_ROLES = ["owner", "admin", "user"];

export interface AnnouncementFormState {
  title: string;
  content: string;
  status: AnnouncementStatus;
  severity: AnnouncementSeverity;
  audience: AnnouncementAudience;
  // A single AND-group of optional targeting conditions that refines the
  // audience. (The API stores an array of OR-groups; the form edits the first.)
  segmentRoles: string[];
  segmentUserIds: string[];
  segmentEmailDomains: string[];
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
    segmentRoles: [],
    segmentUserIds: [],
    segmentEmailDomains: [],
  };
}

export function announcementFormFromAnnouncement(announcement: Announcement): AnnouncementFormState {
  const segment = announcement.segments?.[0];
  return {
    title: announcement.title,
    content: announcement.content,
    status: announcement.status,
    severity: announcement.severity,
    audience: announcement.audience,
    segmentRoles: segment?.roles ?? [],
    segmentUserIds: (segment?.user_ids ?? []).map((id) => String(id)),
    segmentEmailDomains: segment?.email_domains ?? [],
  };
}

export function buildAnnouncementBody(form: AnnouncementFormState): CreateAnnouncementRequest {
  const segment: AnnouncementSegment = {};
  if (form.segmentRoles.length) segment.roles = form.segmentRoles;
  if (form.segmentUserIds.length) segment.user_ids = form.segmentUserIds as Id[];
  if (form.segmentEmailDomains.length) segment.email_domains = form.segmentEmailDomains;
  const hasSegment = Boolean(segment.roles || segment.user_ids || segment.email_domains);
  return {
    title: requiredText(form.title, "Title"),
    content: requiredText(form.content, "Content"),
    status: form.status,
    severity: form.severity,
    audience: form.audience,
    segments: hasSegment ? [segment] : undefined,
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
