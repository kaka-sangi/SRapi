"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { ExternalLink, Link2 } from "lucide-react";
import { cn } from "@/lib/cn";
import { useLanguage } from "@/context/LanguageContext";
import { useAdminSettings } from "@/hooks/admin-queries";
import { useSiteConfig } from "@/hooks/queries";
import { navSectionsForRole } from "./nav-items";
import type { CustomMenuItem } from "../../../../../packages/sdk/typescript/src/types.gen";

interface CustomMenu {
  label: string;
  url: string;
  external: boolean;
}

function parseCustomMenus(raw: unknown, role: "admin" | "user"): CustomMenu[] {
  if (!Array.isArray(raw)) return [];
  const menus: Array<CustomMenu & { sortOrder: number }> = [];
  for (const entry of raw) {
    if (typeof entry !== "object" || entry === null) continue;
    const obj = entry as Record<string, unknown>;
    const label = firstString(obj.label);
    const url = firstString(obj.url);
    const visibility = obj.visibility === "admin" ? "admin" : "user";
    if (visibility !== role) continue;
    if (!label || !url) continue;
    const external = /^https?:\/\//i.test(url);
    if (!external && !url.startsWith("/")) continue;
    menus.push({ label, url, external, sortOrder: numberValue(obj.sort_order) });
  }
  return menus.sort((a, b) => a.sortOrder - b.sortOrder).map(({ sortOrder: _sortOrder, ...menu }) => menu);
}

function firstString(...values: unknown[]): string {
  for (const value of values) {
    if (typeof value === "string" && value.trim() !== "") return value.trim();
  }
  return "";
}

function numberValue(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : Number.MAX_SAFE_INTEGER;
}

const TOUR_TAGS: Record<string, string> = {
  "/admin/quick-setup": "nav-quick-setup",
  "/admin/accounts": "nav-accounts",
  "/admin/models": "nav-models",
  "/admin/providers": "nav-providers",
};

export function SidebarNav({
  role,
  onNavigate,
}: {
  role: "admin" | "user";
  onNavigate?: () => void;
}) {
  const pathname = usePathname();
  const { t } = useLanguage();
  const sections = navSectionsForRole(role);
  const siteConfig = useSiteConfig();
  const adminSettings = useAdminSettings({ enabled: role === "admin" });
  const rawCustomMenus: CustomMenuItem[] | undefined =
    role === "admin"
      ? adminSettings.data?.general.custom_menus
      : siteConfig.data?.custom_menus;
  const customMenus = parseCustomMenus(rawCustomMenus, role);

  function isActive(href: string): boolean {
    if (href === "/dashboard" || href === "/admin") return pathname === href;
    return pathname === href || pathname.startsWith(`${href}/`);
  }

  return (
    <nav className="space-y-1.5 text-sm">
      {sections.map((section, idx) => (
        <div key={section.titleKey} className={idx > 0 ? "pt-4" : undefined}>
          <div className="px-3 pb-2 pt-2 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            {t(section.titleKey)}
          </div>
          <div className="space-y-0.5">
            {section.items.map((item) => {
              const active = isActive(item.href);
              const Icon = item.icon;
              const tourTag = TOUR_TAGS[item.href];
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  onClick={onNavigate}
                  aria-current={active ? "page" : undefined}
                  data-tour={tourTag}
                  className={cn(
                    // Modern card-style nav: full-width rounded fill, soft accent
                    // wash when active, gentle hover wash otherwise.
                    "group relative flex items-center gap-3 rounded-xl px-3 py-2.5 transition-[background-color,color,transform] duration-150 ease-[var(--ease-out-quint)]",
                    active
                      ? "bg-srapi-accent-soft font-medium text-srapi-primary"
                      : "text-srapi-text-secondary hover:bg-srapi-card/80 hover:text-srapi-text-primary",
                  )}
                >
                  <Icon
                    className={cn(
                      "size-[18px] shrink-0 transition-colors",
                      active
                        ? "text-srapi-primary"
                        : "text-srapi-text-tertiary group-hover:text-srapi-text-secondary",
                    )}
                    aria-hidden
                  />
                  <span className="truncate">{t(item.labelKey)}</span>
                </Link>
              );
            })}
          </div>
        </div>
      ))}
      {customMenus.length > 0 ? (
        <div className="pt-4">
          <div className="px-3 pb-2 pt-2 text-[11px] font-semibold uppercase tracking-[0.12em] text-srapi-text-tertiary">
            {t("nav.sectionLinks")}
          </div>
          <div className="space-y-0.5">
            {customMenus.map((menu) =>
              menu.external ? (
                <a
                  key={`${menu.label}-${menu.url}`}
                  href={menu.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  onClick={onNavigate}
                  className="flex items-center gap-3 rounded-xl px-3 py-2.5 text-srapi-text-secondary transition-colors hover:bg-srapi-card/80 hover:text-srapi-text-primary"
                >
                  <ExternalLink className="size-[18px] shrink-0 text-srapi-text-tertiary" aria-hidden />
                  <span className="truncate">{menu.label}</span>
                </a>
              ) : (
                <Link
                  key={`${menu.label}-${menu.url}`}
                  href={menu.url}
                  onClick={onNavigate}
                  className="flex items-center gap-3 rounded-xl px-3 py-2.5 text-srapi-text-secondary transition-colors hover:bg-srapi-card/80 hover:text-srapi-text-primary"
                >
                  <Link2 className="size-[18px] shrink-0 text-srapi-text-tertiary" aria-hidden />
                  <span className="truncate">{menu.label}</span>
                </Link>
              ),
            )}
          </div>
        </div>
      ) : null}
    </nav>
  );
}

export function SidebarBrand() {
  const { t } = useLanguage();
  return (
    <div className="flex items-center gap-3 px-2 pb-1 pt-1">
      <div className="relative grid size-10 place-items-center overflow-hidden rounded-xl bg-gradient-to-br from-srapi-primary to-srapi-primary-hover text-base font-semibold text-white shadow-[0_4px_10px_-4px_rgba(194,85,59,0.45)]">
        <span className="relative z-[1]">S</span>
        <span
          className="pointer-events-none absolute inset-0 opacity-60 mix-blend-overlay"
          style={{
            backgroundImage:
              "radial-gradient(circle at 30% 25%, rgba(255,255,255,0.6), transparent 55%)",
          }}
          aria-hidden
        />
      </div>
      <div className="min-w-0 leading-tight">
        <div className="text-base font-semibold tracking-tight text-srapi-text-primary">
          {t("common.appName")}
        </div>
        <div className="mt-0.5 text-[11px] font-medium text-srapi-text-tertiary">
          {t("common.version")}
        </div>
      </div>
    </div>
  );
}
