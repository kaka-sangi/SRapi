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
  // Scheduling window. Both are optional — leave blank for "active now" /
  // "no end". Stored as the local-form `yyyy-MM-ddTHH:mm` shape used by the
  // datetime-local input; converted to ISO on save.
  startsAt: string;
  endsAt: string;
}

interface AnnouncementDeleteState {
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
    startsAt: "",
    endsAt: "",
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
    startsAt: isoToLocalInput(announcement.starts_at),
    endsAt: isoToLocalInput(announcement.ends_at),
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
    starts_at: localInputToIso(form.startsAt) ?? undefined,
    ends_at: localInputToIso(form.endsAt) ?? undefined,
  };
}

// datetime-local <input> values are `yyyy-MM-ddTHH:mm` in the user's local
// time. Convert each direction via the JS Date so the backend sees a real
// ISO timestamp and the form sees a value the input control will accept.
function isoToLocalInput(iso: string | null | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function localInputToIso(value: string): string | null {
  const trimmed = value.trim();
  if (!trimmed) return null;
  const d = new Date(trimmed);
  if (Number.isNaN(d.getTime())) return null;
  return d.toISOString();
}

function deleteStateFromAnnouncement(announcement: Announcement): AnnouncementDeleteState {
  return {
    id: announcement.id,
    title: announcement.title,
    confirmation: "",
  };
}

function canDeleteAnnouncement(state: AnnouncementDeleteState | null): boolean {
  return Boolean(state && state.confirmation.trim() === state.title);
}

function requiredText(value: string, fieldName: string): string {
  const trimmed = value.trim();
  if (!trimmed) {
    throw new Error(`${fieldName} is required.`);
  }
  return trimmed;
}
