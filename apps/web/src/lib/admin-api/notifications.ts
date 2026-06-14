"use client";

import {
  createAdminAnnouncement,
  deleteAdminAnnouncement,
  getAdminAnnouncementReadStatus,
  listAdminAnnouncements,
  updateAdminAnnouncement,
  listAdminNotificationEmailTemplates,
  updateAdminNotificationEmailTemplate,
  previewAdminNotificationEmailTemplate,
  restoreAdminNotificationEmailTemplate,
} from "../../../../../packages/sdk/typescript/src/index";
import type {
  Announcement,
  AnnouncementReadStatus,
  CreateAnnouncementRequest,
  Id,
  ListAdminAnnouncementsData,
  UpdateAnnouncementRequest,
  NotificationEmailTemplate,
  NotificationEmailTemplateList,
  NotificationEmailTemplateEventName,
  NotificationEmailTemplatePreview,
  PreviewNotificationEmailTemplateRequest,
  UpdateNotificationEmailTemplateRequest,
} from "../../../../../packages/sdk/typescript/src/types.gen";
import { unwrapData, unwrapList } from "./_shared";
import type { AdminListResult } from "./types";

export const notificationsApi = {
  listAnnouncements(
    query?: ListAdminAnnouncementsData["query"],
  ): Promise<AdminListResult<Announcement>> {
    return unwrapList(() => listAdminAnnouncements({ query, throwOnError: true }));
  },

  createAnnouncement(body: CreateAnnouncementRequest): Promise<Announcement> {
    return unwrapData(() => createAdminAnnouncement({ body, throwOnError: true }));
  },

  updateAnnouncement(id: Id, body: UpdateAnnouncementRequest): Promise<Announcement> {
    return unwrapData(() => updateAdminAnnouncement({ path: { id }, body, throwOnError: true }));
  },

  deleteAnnouncement(id: Id): Promise<{ deleted: boolean }> {
    return unwrapData(() => deleteAdminAnnouncement({ path: { id }, throwOnError: true }));
  },

  getAnnouncementReadStatus(id: Id): Promise<AnnouncementReadStatus> {
    return unwrapData(() => getAdminAnnouncementReadStatus({ path: { id }, throwOnError: true }));
  },

  listNotificationEmailTemplates(): Promise<NotificationEmailTemplateList> {
    return unwrapData(() => listAdminNotificationEmailTemplates({ throwOnError: true }));
  },

  updateNotificationEmailTemplate(
    event: NotificationEmailTemplateEventName,
    body: UpdateNotificationEmailTemplateRequest,
  ): Promise<NotificationEmailTemplate> {
    return unwrapData(() =>
      updateAdminNotificationEmailTemplate({ path: { event }, body, throwOnError: true }),
    );
  },

  restoreNotificationEmailTemplate(
    event: NotificationEmailTemplateEventName,
  ): Promise<NotificationEmailTemplate> {
    return unwrapData(() =>
      restoreAdminNotificationEmailTemplate({ path: { event }, throwOnError: true }),
    );
  },

  previewNotificationEmailTemplate(
    body: PreviewNotificationEmailTemplateRequest,
  ): Promise<NotificationEmailTemplatePreview> {
    return unwrapData(() => previewAdminNotificationEmailTemplate({ body, throwOnError: true }));
  },
};
