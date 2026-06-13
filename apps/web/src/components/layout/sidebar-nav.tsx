"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { ExternalLink, Link2 } from "lucide-react";
import { cn } from "@/lib/cn";
import { useLanguage } from "@/context/LanguageContext";
import { useSiteConfig } from "@/hooks/queries";
import { navSectionsForRole } from "./nav-items";

interface CustomMenu {
  label: string;
  url: string;
  external: boolean;
}

/** Normalizes the operator-authored `custom_menus` JSON (freeform objects) into
 * renderable links. Tolerates label/name/title and url/href/link key spellings;
 * drops entries without both a label and a usable URL. */
function parseCustomMenus(raw: unknown): CustomMenu[] {
  if (!Array.isArray(raw)) return [];
  const menus: CustomMenu[] = [];
  for (const entry of raw) {
    if (typeof entry !== "object" || entry === null) continue;
    const obj = entry as Record<string, unknown>;
    const label = firstString(obj.label, obj.name, obj.title);
    const url = firstString(obj.url, obj.href, obj.link);
    if (!label || !url) continue;
    const external = /^https?:\/\//i.test(url);
    if (!external && !url.startsWith("/")) continue;
    menus.push({ label, url, external });
  }
  return menus;
}

function firstString(...values: unknown[]): string {
  for (const value of values) {
    if (typeof value === "string" && value.trim() !== "") return value.trim();
  }
  return "";
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
  const customMenus = parseCustomMenus(siteConfig.data?.custom_menus);

  function isActive(href: string): boolean {
    if (href === "/dashboard" || href === "/admin") return pathname === href;
    return pathname === href || pathname.startsWith(`${href}/`);
  }

  return (
    <nav className="space-y-0.5 text-sm">
      {sections.map((section) => (
        <div key={section.titleKey}>
          <div className="px-2 pb-1 pt-4 font-mono text-2xs uppercase tracking-widest text-srapi-text-secondary">
            {t(section.titleKey)}
          </div>
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
                  "flex items-center gap-2.5 rounded-lg px-3 py-2 transition-colors",
                  active
                    ? "tactile-card border border-srapi-border bg-srapi-card text-srapi-text-primary"
                    : "text-srapi-text-secondary hover:bg-srapi-card/60 hover:text-srapi-text-primary",
                )}
              >
                <Icon className="size-4 shrink-0" aria-hidden />
                <span className="truncate">{t(item.labelKey)}</span>
              </Link>
            );
          })}
        </div>
      ))}
      {customMenus.length > 0 ? (
        <div>
          <div className="px-2 pb-1 pt-4 font-mono text-2xs uppercase tracking-widest text-srapi-text-secondary">
            {t("nav.sectionLinks")}
          </div>
          {customMenus.map((menu) =>
            menu.external ? (
              <a
                key={`${menu.label}-${menu.url}`}
                href={menu.url}
                target="_blank"
                rel="noopener noreferrer"
                onClick={onNavigate}
                className="flex items-center gap-2.5 rounded-lg px-3 py-2 text-srapi-text-secondary transition-colors hover:bg-srapi-card/60 hover:text-srapi-text-primary"
              >
                <ExternalLink className="size-4 shrink-0" aria-hidden />
                <span className="truncate">{menu.label}</span>
              </a>
            ) : (
              <Link
                key={`${menu.label}-${menu.url}`}
                href={menu.url}
                onClick={onNavigate}
                className="flex items-center gap-2.5 rounded-lg px-3 py-2 text-srapi-text-secondary transition-colors hover:bg-srapi-card/60 hover:text-srapi-text-primary"
              >
                <Link2 className="size-4 shrink-0" aria-hidden />
                <span className="truncate">{menu.label}</span>
              </Link>
            ),
          )}
        </div>
      ) : null}
    </nav>
  );
}

export function SidebarBrand() {
  const { t } = useLanguage();
  return (
    <div className="flex items-center gap-2.5 px-2 py-3">
      <div className="grid size-8 place-items-center rounded-lg bg-srapi-invert font-serif text-lg text-srapi-invert-fg">
        S
      </div>
      <div className="font-serif text-xl tracking-tight text-srapi-text-primary">
        {t("common.appName")}
      </div>
      <span className="ml-auto font-mono text-2xs text-srapi-text-secondary">
        {t("common.version")}
      </span>
    </div>
  );
}
