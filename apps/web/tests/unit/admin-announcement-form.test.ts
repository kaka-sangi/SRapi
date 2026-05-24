import { describe, expect, it } from "vitest";
import {
  announcementFormFromAnnouncement,
  buildAnnouncementBody,
  canDeleteAnnouncement,
  deleteStateFromAnnouncement,
  emptyAnnouncementForm,
} from "@/lib/admin-announcement-form";
import type { Announcement } from "../../../../packages/sdk/typescript/src/types.gen";

const announcement: Announcement = {
  id: "announcement-1",
  title: "Maintenance Window",
  content: "Gateway maintenance tonight.",
  status: "published",
  severity: "warning",
  audience: "users",
  created_at: "2026-05-24T00:00:00Z",
  updated_at: "2026-05-24T00:00:00Z",
};

describe("admin-announcement-form", () => {
  it("builds announcement payloads from trimmed required fields", () => {
    const body = buildAnnouncementBody({
      ...emptyAnnouncementForm(),
      title: "  Release  ",
      content: "  New gateway controls. ",
    });

    expect(body).toMatchObject({
      title: "Release",
      content: "New gateway controls.",
      status: "draft",
      severity: "info",
      audience: "all",
    });
  });

  it("round-trips existing announcements into form state", () => {
    expect(announcementFormFromAnnouncement(announcement)).toMatchObject({
      title: "Maintenance Window",
      status: "published",
      severity: "warning",
      audience: "users",
    });
  });

  it("rejects empty title or content", () => {
    expect(() => buildAnnouncementBody({ ...emptyAnnouncementForm(), content: "body" })).toThrow(
      "Title is required.",
    );
    expect(() => buildAnnouncementBody({ ...emptyAnnouncementForm(), title: "Title" })).toThrow(
      "Content is required.",
    );
  });

  it("requires exact title confirmation before delete", () => {
    const state = deleteStateFromAnnouncement(announcement);
    expect(canDeleteAnnouncement(state)).toBe(false);
    expect(canDeleteAnnouncement({ ...state, confirmation: "Maintenance Window" })).toBe(true);
    expect(canDeleteAnnouncement({ ...state, confirmation: "maintenance window" })).toBe(false);
  });
});
