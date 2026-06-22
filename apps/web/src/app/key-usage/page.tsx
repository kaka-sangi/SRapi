"use client";

import { useState } from "react";
import Link from "next/link";
import { Search, KeyRound } from "lucide-react";
import { useLanguage } from "@/context/LanguageContext";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import { QuietBadge } from "@/components/ui/quiet-badge";
import { StatCard } from "@/components/ui/stat-card";
import {
  Table,
  TableScroll,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from "@/components/ui/table";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { ThemeToggle } from "@/components/layout/theme-toggle";
import { LanguageToggle } from "@/components/layout/language-toggle";
import { formatMoney } from "@/lib/admin-format";
import type { GatewayUsageResponse } from "@/lib/sdk-types";

/**
 * Public, login-free key usage self-check (公开 key 用量自查). The pasted key
 * authenticates the gateway's own `GET /v1/usage` — it never touches a session
 * and is kept in memory only. Lets end users of resold/distributed keys check
 * remaining balance and spend without an account on this console.
 */
export default function KeyUsagePage() {
  const { t } = useLanguage();
  const [key, setKey] = useState("");
  const [days, setDays] = useState("30");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [report, setReport] = useState<GatewayUsageResponse | null>(null);

  // Changing the window drops the previous report/error so a different-period
  // result isn't shown under the new filter until the user re-runs the lookup.
  function changeDays(next: string) {
    setDays(next);
    setReport(null);
    setError(null);
  }

  async function lookup(event: React.FormEvent) {
    event.preventDefault();
    const trimmed = key.trim();
    if (!trimmed || loading) return;
    setLoading(true);
    setError(null);
    try {
      const res = await fetch(`/v1/usage?days=${encodeURIComponent(days)}`, {
        headers: { Authorization: `Bearer ${trimmed}` },
      });
      if (!res.ok) {
        setReport(null);
        setError(res.status === 401 ? t("keyUsage.invalidKey") : `HTTP ${res.status}`);
        return;
      }
      setReport((await res.json()) as GatewayUsageResponse);
    } catch (err) {
      setReport(null);
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="relative flex min-h-dvh flex-col">
      <header className="mx-auto flex w-full max-w-5xl items-center justify-between px-6 py-6">
        <Link
          href="/"
          className="text-2xl font-semibold tracking-tight leading-none text-srapi-text-primary"
        >
          SRapi
        </Link>
        <div className="flex items-center gap-2">
          <LanguageToggle />
          <ThemeToggle />
        </div>
      </header>
      <main className="mx-auto w-full max-w-5xl flex-1 px-6 pb-16">
        <div className="animate-bloom">
          <h1 className="text-3xl font-semibold tracking-tight text-srapi-text-primary">
            {t("keyUsage.title")}
          </h1>
          <p className="mt-2 max-w-xl text-sm leading-relaxed text-srapi-text-secondary">
            {t("keyUsage.subtitle")}
          </p>

          <Card className="mt-6">
            <CardContent>
              <form onSubmit={lookup} className="flex flex-col gap-3 sm:flex-row sm:items-end">
                <div className="min-w-0 flex-1">
                  <Label htmlFor="lookup-key">{t("keyUsage.keyLabel")}</Label>
                  <Input
                    id="lookup-key"
                    value={key}
                    onChange={(e) => setKey(e.target.value)}
                    placeholder="sk-…"
                    autoComplete="off"
                    spellCheck={false}
                    className="font-mono"
                  />
                </div>
                <div>
                  <Label htmlFor="lookup-days">{t("keyUsage.window")}</Label>
                  <Select value={days} onValueChange={changeDays}>
                    <SelectTrigger id="lookup-days" className="w-28">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {["7", "30", "90"].map((d) => (
                        <SelectItem key={d} value={d}>
                          {t("keyUsage.days", { days: d })}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <Button type="submit" variant="primary" loading={loading} disabled={!key.trim()}>
                  <Search className="size-4" />
                  {t("keyUsage.lookup")}
                </Button>
              </form>
              <p className="mt-2 text-[11px] text-srapi-text-tertiary">{t("keyUsage.privacyHint")}</p>
              {error ? (
                <p role="alert" className="mt-2 text-sm text-srapi-error">
                  {error}
                </p>
              ) : null}
            </CardContent>
          </Card>

          {report ? <UsageReport report={report} /> : null}
        </div>
      </main>
    </div>
  );
}

function UsageReport({ report }: { report: GatewayUsageResponse }) {
  const { t } = useLanguage();
  const currency = report.usage.currency || report.unit;
  return (
    <div className="mt-6 space-y-4" style={{ "--stagger-index": 0 } as React.CSSProperties}>
      <div className="flex flex-wrap items-center gap-2.5">
        <KeyRound className="size-4 text-srapi-text-tertiary" aria-hidden />
        <span className="font-medium text-srapi-text-primary">{report.api_key_name}</span>
        <QuietBadge
          status={report.isValid ? "active" : "disabled"}
          label={report.isValid ? t("keyUsage.valid") : t("keyUsage.invalid")}
        />
        {typeof report.days_until_expiry === "number" ? (
          <span className="text-[12px] text-srapi-text-tertiary tabular">
            {t("keyUsage.expiresIn", { days: report.days_until_expiry })}
          </span>
        ) : null}
      </div>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <StatCard label={t("keyUsage.requests")} value={report.usage.requests} />
        <StatCard label={t("keyUsage.totalTokens")} value={report.usage.total_tokens} />
        <StatCard label={t("keyUsage.cost")} value={formatMoney(report.usage.cost, currency)} />
        <StatCard label={t("keyUsage.balance")} value={formatMoney(report.balance, report.unit)} />
      </div>

      {report.daily_usage.length > 0 ? (
        <Card>
          <CardContent>
            <h2 className="text-lg font-semibold tracking-tight text-srapi-text-primary">
              {t("adminUsage.byDay")}
            </h2>
            <TableScroll minWidth={480}>
              <Table className="mt-3">
                <TableHeader>
                  <tr>
                    <TableHead>{t("adminAffiliates.date")}</TableHead>
                    <TableHead className="text-right">{t("keyUsage.requests")}</TableHead>
                    <TableHead className="text-right">{t("dashboard.inputTokens")}</TableHead>
                    <TableHead className="text-right">{t("dashboard.outputTokens")}</TableHead>
                    <TableHead className="text-right">{t("keyUsage.cost")}</TableHead>
                  </tr>
                </TableHeader>
                <TableBody>
                  {report.daily_usage.map((d) => (
                    <TableRow key={d.date}>
                      <TableCell className="text-[12px] text-srapi-text-secondary tabular">{d.date}</TableCell>
                      <TableCell className="text-right text-sm tabular">{d.requests}</TableCell>
                      <TableCell className="text-right text-sm tabular">{d.input_tokens}</TableCell>
                      <TableCell className="text-right text-sm tabular">{d.output_tokens}</TableCell>
                      <TableCell className="text-right text-sm font-medium tabular">
                        {formatMoney(d.cost, currency)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableScroll>
          </CardContent>
        </Card>
      ) : null}

      {report.model_stats.length > 0 ? (
        <Card>
          <CardContent>
            <h2 className="text-lg font-semibold tracking-tight text-srapi-text-primary">
              {t("keyUsage.byModel")}
            </h2>
            <TableScroll minWidth={480}>
              <Table className="mt-3">
                <TableHeader>
                  <tr>
                    <TableHead>{t("keyUsage.model")}</TableHead>
                    <TableHead className="text-right">{t("keyUsage.requests")}</TableHead>
                    <TableHead className="text-right">{t("keyUsage.totalTokens")}</TableHead>
                    <TableHead className="text-right">{t("keyUsage.cost")}</TableHead>
                  </tr>
                </TableHeader>
                <TableBody>
                  {report.model_stats.map((m) => (
                    <TableRow key={m.model}>
                      <TableCell className="text-[12px] text-srapi-text-secondary tabular">{m.model}</TableCell>
                      <TableCell className="text-right text-sm tabular">{m.requests}</TableCell>
                      <TableCell className="text-right text-sm tabular">{m.total_tokens}</TableCell>
                      <TableCell className="text-right text-sm font-medium tabular">
                        {formatMoney(m.cost, currency)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableScroll>
          </CardContent>
        </Card>
      ) : null}

      {report.recent_requests.length > 0 ? (
        <Card>
          <CardContent>
            <h2 className="text-lg font-semibold tracking-tight text-srapi-text-primary">
              {t("keyUsage.recent")}
            </h2>
            <TableScroll minWidth={480}>
              <Table className="mt-3">
                <TableHeader>
                  <tr>
                    <TableHead>{t("keyUsage.model")}</TableHead>
                    <TableHead>{t("keyUsage.status")}</TableHead>
                    <TableHead className="text-right">{t("keyUsage.totalTokens")}</TableHead>
                    <TableHead className="text-right">{t("keyUsage.time")}</TableHead>
                  </tr>
                </TableHeader>
                <TableBody>
                  {report.recent_requests.map((r, i) => (
                    <TableRow key={i}>
                      <TableCell className="text-[12px] text-srapi-text-secondary tabular">{r.model}</TableCell>
                      <TableCell>
                        <QuietBadge
                          status={r.success ? "active" : "disabled"}
                          label={r.success ? t("keyUsage.ok") : t("keyUsage.failed")}
                        />
                      </TableCell>
                      <TableCell className="text-right text-sm tabular">{r.total_tokens}</TableCell>
                      <TableCell className="text-right text-[12px] text-srapi-text-tertiary tabular">
                        {r.created_at.replace("T", " ").slice(0, 19)}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableScroll>
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}
