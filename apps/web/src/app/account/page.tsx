"use client";

import { useEffect, useRef, useState } from "react";
import { AppShell } from "@/components/layout/app-shell";
import { PageHeader } from "@/components/layout/page-header";
import { PageQueryState } from "@/components/layout/page-query-state";
import {
  useProfile,
  useUpdateProfile,
  useChangePassword,
  useTotpStatus,
  useSetupTotp,
  useEnableTotp,
  useDisableTotp,
  useUploadAvatar,
  useDeleteAvatar,
  useRevokeAllSessions,
} from "@/hooks/queries";
import { NotificationsTab } from "@/components/features/account-notifications";
import { LinkedSignInsCard } from "@/components/features/account-linked-signins";
import { useLanguage } from "@/context/LanguageContext";
import { useToast } from "@/context/ToastContext";
import { Card, CardContent } from "@/components/ui/card";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/ui/copy-button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { FormSkeleton } from "@/components/charts/chart-skeleton";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { apiService, type CurrentUserAttribute } from "@/lib/api";
import { meErrorMessage } from "@/lib/me-api";
import type { User } from "@/lib/sdk-types";

export default function AccountPage() {
  return (
    <AppShell allowedRole="user">
      <AccountContent />
    </AppShell>
  );
}

function AccountContent() {
  const { t } = useLanguage();
  const profile = useProfile();

  return (
    <>
      <PageHeader
        eyebrow={t("nav.sectionAccount")}
        title={t("account.title")}
        description={t("account.subtitle")}
      />
      <Tabs defaultValue="profile">
        <TabsList>
          <TabsTrigger value="profile">{t("account.tabProfile")}</TabsTrigger>
          <TabsTrigger value="security">{t("account.tabSecurity")}</TabsTrigger>
          <TabsTrigger value="notifications">{t("account.tabNotifications")}</TabsTrigger>
        </TabsList>
        <TabsContent value="profile">
          <PageQueryState query={profile} skeleton={<FormSkeleton rows={3} className="p-5" />}>
            {(user) => (
              <div className="grid gap-4 lg:grid-cols-2">
                <ProfileForm user={user} />
                <UserAttributesCard />
              </div>
            )}
          </PageQueryState>
        </TabsContent>
        <TabsContent value="security">
          <div className="grid gap-4 lg:grid-cols-2">
            <ChangePasswordCard />
            <TwoFactorCard />
            <LinkedSignInsCard />
            <SignOutEverywhereCard />
          </div>
        </TabsContent>
        <TabsContent value="notifications">
          <NotificationsTab />
        </TabsContent>
      </Tabs>
    </>
  );
}

function UserAttributesCard() {
  const { toast } = useToast();
  const [items, setItems] = useState<CurrentUserAttribute[]>([]);
  const [values, setValues] = useState<Record<number, string>>({});
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let active = true;
    apiService.listCurrentUserAttributes()
      .then((next) => {
        if (!active) return;
        setItems(next);
        setValues(Object.fromEntries(next.map((item) => [item.definition_id, item.value || ""])));
      })
      .catch((err) => {
        if (active) setError(meErrorMessage(err));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

  async function save() {
    setSaving(true);
    setError(null);
    try {
      const updated = await apiService.updateCurrentUserAttributes(
        items.map((item) => ({ definition_id: item.definition_id, value: values[item.definition_id] || "" })),
      );
      setItems(updated);
      setValues(Object.fromEntries(updated.map((item) => [item.definition_id, item.value || ""])));
      toast({ title: "Saved", tone: "success" });
    } catch (err) {
      setError(meErrorMessage(err));
    } finally {
      setSaving(false);
    }
  }

  if (loading) {
    return (
      <Card>
        <CardContent className="space-y-3">
          <Skeleton className="h-5 w-32" />
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </CardContent>
      </Card>
    );
  }

  if (items.length === 0) {
    return null;
  }

  return (
    <Card>
      <CardContent className="space-y-4">
        <h3 className="font-serif text-lg text-srapi-text-primary">Profile attributes</h3>
        {items.map((item) => (
          <div key={item.definition_id}>
            <Label htmlFor={`attr-${item.definition_id}`}>{item.name}</Label>
            {item.data_type === "boolean" ? (
              <select
                id={`attr-${item.definition_id}`}
                className="mt-1 h-10 w-full rounded-lg border border-srapi-border bg-srapi-card px-3 text-sm"
                value={values[item.definition_id] || ""}
                onChange={(event) => setValues((prev) => ({ ...prev, [item.definition_id]: event.target.value }))}
              >
                <option value="">Unset</option>
                <option value="true">True</option>
                <option value="false">False</option>
              </select>
            ) : item.data_type === "select" ? (
              <select
                id={`attr-${item.definition_id}`}
                className="mt-1 h-10 w-full rounded-lg border border-srapi-border bg-srapi-card px-3 text-sm"
                value={values[item.definition_id] || ""}
                onChange={(event) => setValues((prev) => ({ ...prev, [item.definition_id]: event.target.value }))}
              >
                <option value="">Unset</option>
                {(item.options || []).map((option) => (
                  <option key={option} value={option}>{option}</option>
                ))}
              </select>
            ) : (
              <Input
                id={`attr-${item.definition_id}`}
                type={item.data_type === "number" ? "number" : "text"}
                value={values[item.definition_id] || ""}
                onChange={(event) => setValues((prev) => ({ ...prev, [item.definition_id]: event.target.value }))}
              />
            )}
          </div>
        ))}
        {error ? <p className="text-sm text-srapi-error">{error}</p> : null}
        <div className="flex justify-end">
          <Button variant="primary" loading={saving} onClick={save}>
            Save attributes
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

const MAX_AVATAR_BYTES = 1024 * 1024;

function ProfileForm({ user }: { user: User }) {
  const { t } = useLanguage();
  const { toast } = useToast();
  const updateMut = useUpdateProfile();
  const uploadMut = useUploadAvatar();
  const deleteMut = useDeleteAvatar();
  const fileInput = useRef<HTMLInputElement>(null);
  const [name, setName] = useState(user.name);

  const initials = (user.name || user.email).trim().charAt(0).toUpperCase();
  // Cache-bust against avatar_updated_at so a fresh upload renders immediately.
  const avatarSrc = user.avatar_url
    ? `${user.avatar_url}${user.avatar_updated_at ? `?v=${encodeURIComponent(user.avatar_updated_at)}` : ""}`
    : null;

  async function save() {
    try {
      await updateMut.mutateAsync({ name: name.trim() });
      toast({ title: t("feedback.saved"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: meErrorMessage(err), tone: "error" });
    }
  }

  async function onPickFile(event: React.ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    if (file.size > MAX_AVATAR_BYTES) {
      toast({ title: t("feedback.failed"), description: t("account.avatarTooLarge"), tone: "error" });
      return;
    }
    try {
      await uploadMut.mutateAsync(file);
      toast({ title: t("feedback.saved"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: meErrorMessage(err), tone: "error" });
    }
  }

  async function removeAvatar() {
    try {
      await deleteMut.mutateAsync();
      toast({ title: t("feedback.saved"), tone: "success" });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: meErrorMessage(err), tone: "error" });
    }
  }

  return (
    <Card>
      <CardContent className="max-w-md space-y-4">
        <div>
          <Label>{t("account.avatar")}</Label>
          <div className="mt-1 flex items-center gap-4">
            <span className="flex size-16 items-center justify-center overflow-hidden rounded-full border border-srapi-border bg-srapi-card-muted text-xl font-serif text-srapi-text-secondary">
              {avatarSrc ? (
                // eslint-disable-next-line @next/next/no-img-element
                <img src={avatarSrc} alt="" className="size-full object-cover" />
              ) : (
                initials
              )}
            </span>
            <div className="space-y-2">
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  loading={uploadMut.isPending}
                  onClick={() => fileInput.current?.click()}
                >
                  {t("account.avatarUpload")}
                </Button>
                {avatarSrc ? (
                  <Button
                    variant="ghost"
                    size="sm"
                    loading={deleteMut.isPending}
                    onClick={removeAvatar}
                  >
                    {t("account.avatarRemove")}
                  </Button>
                ) : null}
              </div>
              <p className="text-2xs text-srapi-text-tertiary">{t("account.avatarHint")}</p>
            </div>
            <input
              ref={fileInput}
              type="file"
              accept="image/png,image/jpeg"
              className="hidden"
              onChange={onPickFile}
            />
          </div>
        </div>
        <div>
          <Label htmlFor="email">{t("account.email")}</Label>
          <Input id="email" value={user.email} disabled />
        </div>
        <div>
          <Label htmlFor="name">{t("account.name")}</Label>
          <Input id="name" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <div className="flex justify-end">
          <Button
            variant="primary"
            loading={updateMut.isPending}
            disabled={!name.trim()}
            onClick={save}
          >
            {t("account.saveProfile")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function ChangePasswordCard() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const changeMut = useChangePassword();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    if (next !== confirm) {
      setError(t("account.passwordMismatch"));
      return;
    }
    try {
      await changeMut.mutateAsync({ current_password: current, new_password: next });
      toast({ title: t("feedback.saved"), tone: "success" });
      setCurrent("");
      setNext("");
      setConfirm("");
    } catch (err) {
      setError(meErrorMessage(err));
    }
  }

  return (
    <Card>
      <CardContent>
        <form onSubmit={submit} className="space-y-4">
          <h3 className="font-serif text-lg text-srapi-text-primary">{t("account.changePassword")}</h3>
          <div>
            <Label htmlFor="cur">{t("account.currentPassword")}</Label>
            <Input id="cur" type="password" value={current} onChange={(e) => setCurrent(e.target.value)} />
          </div>
          <div>
            <Label htmlFor="new">{t("account.newPassword")}</Label>
            <Input id="new" type="password" value={next} onChange={(e) => setNext(e.target.value)} />
          </div>
          <div>
            <Label htmlFor="cfm">{t("account.confirmPassword")}</Label>
            <Input id="cfm" type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
          </div>
          {error ? (
            <p role="alert" className="text-sm text-srapi-error">
              {error}
            </p>
          ) : null}
          <div className="flex justify-end">
            <Button
              type="submit"
              variant="primary"
              loading={changeMut.isPending}
              disabled={!current || !next}
            >
              {t("account.changePassword")}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

function TwoFactorCard() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const status = useTotpStatus();
  const setupMut = useSetupTotp();
  const enableMut = useEnableTotp();
  const disableMut = useDisableTotp();
  const [setup, setSetup] = useState<{ secret: string; otp_auth_url: string } | null>(null);
  const [code, setCode] = useState("");

  const enabled = status.data?.enabled ?? false;

  async function beginSetup() {
    try {
      const res = await setupMut.mutateAsync();
      setSetup({ secret: res.secret, otp_auth_url: res.otp_auth_url });
    } catch (err) {
      toast({ title: t("feedback.failed"), description: meErrorMessage(err), tone: "error" });
    }
  }

  async function enable() {
    try {
      await enableMut.mutateAsync({ code: code.trim() });
      toast({ title: t("feedback.saved"), tone: "success" });
      setSetup(null);
      setCode("");
    } catch (err) {
      toast({ title: t("feedback.failed"), description: meErrorMessage(err), tone: "error" });
    }
  }

  async function disable() {
    try {
      await disableMut.mutateAsync({ code: code.trim() });
      toast({ title: t("feedback.saved"), tone: "success" });
      setCode("");
    } catch (err) {
      toast({ title: t("feedback.failed"), description: meErrorMessage(err), tone: "error" });
    }
  }

  return (
    <Card>
      <CardContent className="space-y-4">
        <div className="flex items-center justify-between">
          <h3 className="font-serif text-lg text-srapi-text-primary">{t("account.twoFactor")}</h3>
          <QuietBadge
            status={enabled ? "active" : "disabled"}
            label={enabled ? t("account.twoFactorOn") : t("account.twoFactorOff")}
          />
        </div>

        {enabled ? (
          <div className="space-y-3">
            <div>
              <Label htmlFor="dis-code">{t("account.totpCode")}</Label>
              <Input id="dis-code" inputMode="numeric" value={code} onChange={(e) => setCode(e.target.value)} />
            </div>
            <div className="flex justify-end">
              <Button
                variant="danger"
                loading={disableMut.isPending}
                disabled={!code.trim()}
                onClick={disable}
              >
                {t("account.disable")}
              </Button>
            </div>
          </div>
        ) : setup ? (
          <div className="space-y-3">
            <p className="text-2xs text-srapi-text-tertiary">{t("account.totpHint")}</p>
            <div>
              <Label>{t("account.totpSecret")}</Label>
              <div className="flex items-center gap-2">
                <code className="flex-1 truncate rounded-lg border border-srapi-border bg-srapi-card-muted px-3 py-2 font-mono text-xs">
                  {setup.secret}
                </code>
                <CopyButton value={setup.secret} label={t("account.totpSecret")} />
              </div>
            </div>
            <div>
              <Label htmlFor="en-code">{t("account.totpCode")}</Label>
              <Input id="en-code" inputMode="numeric" value={code} onChange={(e) => setCode(e.target.value)} />
            </div>
            <div className="flex justify-end">
              <Button
                variant="primary"
                loading={enableMut.isPending}
                disabled={!code.trim()}
                onClick={enable}
              >
                {t("account.enable")}
              </Button>
            </div>
          </div>
        ) : (
          <div className="flex justify-end">
            <Button variant="outline" loading={setupMut.isPending} onClick={beginSetup}>
              {t("account.setup")}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function SignOutEverywhereCard() {
  const { t } = useLanguage();
  const { toast } = useToast();
  const revokeMut = useRevokeAllSessions();

  async function revokeAll() {
    if (!window.confirm(t("account.signOutAllConfirm"))) return;
    try {
      await revokeMut.mutateAsync();
      // The current session is revoked too, so leave for the sign-in screen.
      window.location.assign("/login");
    } catch (err) {
      toast({ title: t("feedback.failed"), description: meErrorMessage(err), tone: "error" });
    }
  }

  return (
    <Card>
      <CardContent className="space-y-4">
        <div>
          <h3 className="font-serif text-lg text-srapi-text-primary">{t("account.signOutAll")}</h3>
          <p className="mt-1 text-2xs text-srapi-text-tertiary">{t("account.signOutAllHint")}</p>
        </div>
        <div className="flex justify-end">
          <Button variant="danger" loading={revokeMut.isPending} onClick={revokeAll}>
            {t("account.signOutAllButton")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
