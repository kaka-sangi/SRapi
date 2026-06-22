"use client";

import { useState } from "react";
import {
  useNotificationPreferences,
  useUpdateNotificationPreferences,
  useNotificationContacts,
  useRequestNotificationContact,
  useConfirmNotificationContact,
  useUpdateNotificationContact,
  useDeleteNotificationContact,
} from "@/hooks/queries";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { meErrorMessage } from "@/lib/me-api";
import { Card, CardContent } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { DialogListSkeleton } from "@/components/charts/chart-skeleton";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { PageQueryState } from "@/components/layout/page-query-state";
import type { NotificationContact, NotificationPreference } from "@/lib/sdk-types";

// Maps the fixed per-event enum to localized copy. Falls back to the
// server-provided label/description for any event we don't recognize yet.
const EVENT_I18N: Record<string, { label: string; desc: string }> = {
  "balance.low": {
    label: "account.notifyEventBalanceLowLabel",
    desc: "account.notifyEventBalanceLowDesc",
  },
  "subscription.expiry_reminder": {
    label: "account.notifyEventSubscriptionExpiryLabel",
    desc: "account.notifyEventSubscriptionExpiryDesc",
  },
  "account.quota_alert": {
    label: "account.notifyEventQuotaAlertLabel",
    desc: "account.notifyEventQuotaAlertDesc",
  },
};

export function NotificationsTab() {
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <NotificationPreferencesCard />
      <NotificationContactsCard />
    </div>
  );
}

function NotificationPreferencesCard() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const prefs = useNotificationPreferences();
  const updateMut = useUpdateNotificationPreferences();
  const [pendingEvent, setPendingEvent] = useState<string | null>(null);

  async function toggle(event: NotificationPreference["event"], subscribed: boolean) {
    setPendingEvent(event);
    try {
      await updateMut.mutateAsync({ preferences: [{ event, subscribed }] });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: meErrorMessage(err), tone: "error" });
    } finally {
      setPendingEvent(null);
    }
  }

  return (
    <Card>
      <CardContent className="space-y-4">
        <div>
          <h3 className="text-lg font-semibold tracking-tight text-srapi-text-primary">
            {t("account.notifyPrefsTitle")}
          </h3>
          <p className="mt-1 text-xs text-srapi-text-tertiary">{t("account.notifyPrefsHint")}</p>
        </div>
        <PageQueryState
          query={prefs}
          skeleton={<DialogListSkeleton rows={3} />}
          isEmpty={(d) => d.data.length === 0}
          emptyTitle={t("account.notifyPrefsEmpty")}
        >
          {(list) => (
            <ul className="divide-y divide-srapi-border/70">
              {list.data.map((pref) => {
                const copy = EVENT_I18N[pref.event];
                const label = copy ? t(copy.label) : pref.label;
                const desc = copy ? t(copy.desc) : pref.description;
                return (
                  <li key={pref.event} className="flex items-start justify-between gap-4 py-3">
                    <div>
                      <p className="text-sm font-medium text-srapi-text-primary">{label}</p>
                      {desc ? (
                        <p className="text-xs text-srapi-text-tertiary">{desc}</p>
                      ) : null}
                    </div>
                    <Switch
                      checked={pref.subscribed}
                      disabled={pendingEvent === pref.event}
                      onCheckedChange={(next) => void toggle(pref.event, next)}
                      aria-label={label}
                    />
                  </li>
                );
              })}
            </ul>
          )}
        </PageQueryState>
      </CardContent>
    </Card>
  );
}

function NotificationContactsCard() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const contacts = useNotificationContacts();
  const requestMut = useRequestNotificationContact();
  const confirmMut = useConfirmNotificationContact();
  const updateMut = useUpdateNotificationContact();
  const deleteMut = useDeleteNotificationContact();

  const [email, setEmail] = useState("");
  const [codeSent, setCodeSent] = useState(false);
  const [token, setToken] = useState("");

  async function sendCode() {
    try {
      await requestMut.mutateAsync({ email: email.trim() });
      setCodeSent(true);
      toast({ title: t("account.notifyContactCodeSent"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: meErrorMessage(err), tone: "error" });
    }
  }

  async function confirm() {
    try {
      await confirmMut.mutateAsync({ token: token.trim() });
      toast({ title: t("feedback.saved"), tone: "success" });
      setEmail("");
      setToken("");
      setCodeSent(false);
    } catch (err) {
      toast({ title: t("feedback.failed"), description: meErrorMessage(err), tone: "error" });
    }
  }

  async function toggleMute(contact: NotificationContact) {
    try {
      await updateMut.mutateAsync({ id: contact.id, disabled: !contact.disabled });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: meErrorMessage(err), tone: "error" });
    }
  }

  async function remove(contact: NotificationContact) {
    if (!window.confirm(t("account.notifyContactDeleteConfirm"))) return;
    try {
      await deleteMut.mutateAsync(contact.id);
      toast({ title: t("feedback.saved"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: meErrorMessage(err), tone: "error" });
    }
  }

  return (
    <Card>
      <CardContent className="space-y-4">
        <div>
          <h3 className="text-lg font-semibold tracking-tight text-srapi-text-primary">
            {t("account.notifyContactsTitle")}
          </h3>
          <p className="mt-1 text-xs text-srapi-text-tertiary">
            {t("account.notifyContactsHint")}
          </p>
        </div>

        <PageQueryState
          query={contacts}
          skeleton={<DialogListSkeleton rows={2} />}
          isEmpty={(d) => d.data.length === 0}
          emptyTitle={t("account.notifyContactEmpty")}
        >
          {(list) => (
            <ul className="divide-y divide-srapi-border/70">
              {list.data.map((contact) => (
                <li key={contact.id} className="flex items-center justify-between gap-3 py-3">
                  <div className="min-w-0">
                    <p className="truncate text-sm text-srapi-text-primary">{contact.email}</p>
                    <div className="mt-1 flex gap-1.5">
                      <QuietBadge
                        status={contact.verified ? "active" : "limited"}
                        label={
                          contact.verified
                            ? t("account.notifyContactVerified")
                            : t("account.notifyContactPending")
                        }
                      />
                      {contact.disabled ? (
                        <QuietBadge status="disabled" label={t("account.notifyContactMuted")} />
                      ) : null}
                    </div>
                  </div>
                  <div className="flex shrink-0 gap-2">
                    {contact.verified ? (
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={updateMut.isPending}
                        onClick={() => void toggleMute(contact)}
                      >
                        {contact.disabled
                          ? t("account.notifyContactUnmute")
                          : t("account.notifyContactMute")}
                      </Button>
                    ) : null}
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={deleteMut.isPending}
                      onClick={() => void remove(contact)}
                    >
                      {t("account.notifyContactDelete")}
                    </Button>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </PageQueryState>

        <div className="space-y-3 border-t border-srapi-border/70 pt-4">
          <div>
            <Label htmlFor="contact-email">{t("account.notifyContactEmail")}</Label>
            <div className="mt-1 flex gap-2">
              <Input
                id="contact-email"
                type="email"
                value={email}
                onChange={(e) => {
                  setEmail(e.target.value);
                  setCodeSent(false);
                }}
                placeholder="alerts@example.com"
              />
              <Button
                variant="outline"
                loading={requestMut.isPending}
                disabled={!email.trim()}
                onClick={sendCode}
              >
                {t("account.notifyContactSendCode")}
              </Button>
            </div>
          </div>
          {codeSent ? (
            <div>
              <Label htmlFor="contact-code">{t("account.notifyContactCode")}</Label>
              <div className="mt-1 flex gap-2">
                <Input
                  id="contact-code"
                  value={token}
                  onChange={(e) => setToken(e.target.value)}
                  inputMode="numeric"
                />
                <Button
                  variant="primary"
                  loading={confirmMut.isPending}
                  disabled={!token.trim()}
                  onClick={confirm}
                >
                  {t("account.notifyContactConfirm")}
                </Button>
              </div>
            </div>
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}
