"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/cn";
import { useLanguage } from "@/context/LanguageContext";
import { navSectionsForRole } from "./nav-items";

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
            return (
              <Link
                key={item.href}
                href={item.href}
                onClick={onNavigate}
                aria-current={active ? "page" : undefined}
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
