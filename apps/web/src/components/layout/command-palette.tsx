"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { useRouter } from "next/navigation";
import { useTheme } from "next-themes";
import { Search, CornerDownLeft, Moon, Sun, Languages, LogOut, type LucideIcon } from "lucide-react";
import { navSectionsForRole } from "./nav-items";
import { useLanguage } from "@/context/LanguageContext";
import { apiService } from "@/lib/api";
import { SIGN_IN_ROUTE } from "@/lib/routes";
import { cn } from "@/lib/cn";

interface CommandItem {
  id: string;
  group: string;
  label: string;
  icon: LucideIcon;
  /** extra text matched by the filter but not displayed (e.g. the route path). */
  keywords?: string;
  run: () => void;
}

const CommandPaletteContext = createContext<{ open: () => void } | null>(null);

/** Open the ⌘K command palette from anywhere inside the shell. */
export function useCommandPalette() {
  const ctx = useContext(CommandPaletteContext);
  if (!ctx) throw new Error("useCommandPalette must be used within CommandPaletteProvider");
  return ctx;
}

/**
 * Hosts the global ⌘K / Ctrl+K command palette: a keyboard-first launcher that
 * jumps to any page the current role can reach and runs a few power actions.
 * Mounted inside the authenticated shell so it knows the role and can navigate.
 */
export function CommandPaletteProvider({
  role,
  children,
}: {
  role: "admin" | "user";
  children: React.ReactNode;
}) {
  const [open, setOpen] = useState(false);

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setOpen((v) => !v);
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const value = useMemo(() => ({ open: () => setOpen(true) }), []);

  return (
    <CommandPaletteContext.Provider value={value}>
      {children}
      <CommandPalette role={role} open={open} onOpenChange={setOpen} />
    </CommandPaletteContext.Provider>
  );
}

function CommandPalette({
  role,
  open,
  onOpenChange,
}: {
  role: "admin" | "user";
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const router = useRouter();
  const { t, toggleLanguage } = useLanguage();
  const { resolvedTheme, setTheme } = useTheme();
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const listRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const close = useCallback(() => onOpenChange(false), [onOpenChange]);

  const commands = useMemo<CommandItem[]>(() => {
    const pages: CommandItem[] = navSectionsForRole(role).flatMap((section) =>
      section.items.map((item) => ({
        id: `page:${item.href}`,
        group: t(section.titleKey),
        label: t(item.labelKey),
        icon: item.icon,
        keywords: item.href,
        run: () => {
          router.push(item.href);
          close();
        },
      })),
    );
    const actionsGroup = t("commandPalette.actions");
    const isDark = resolvedTheme === "dark";
    const actions: CommandItem[] = [
      {
        id: "action:theme",
        group: actionsGroup,
        label: t("commandPalette.toggleTheme"),
        icon: isDark ? Sun : Moon,
        run: () => {
          setTheme(isDark ? "light" : "dark");
          close();
        },
      },
      {
        id: "action:language",
        group: actionsGroup,
        label: t("commandPalette.toggleLanguage"),
        icon: Languages,
        run: () => {
          toggleLanguage();
          close();
        },
      },
      {
        id: "action:signout",
        group: actionsGroup,
        label: t("nav.signOut"),
        icon: LogOut,
        run: () => {
          close();
          void apiService.logout().then(() => router.replace(SIGN_IN_ROUTE));
        },
      },
    ];
    return [...pages, ...actions];
  }, [role, t, router, close, resolvedTheme, setTheme, toggleLanguage]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return commands;
    return commands.filter((c) => `${c.label} ${c.keywords ?? ""}`.toLowerCase().includes(q));
  }, [commands, query]);

  // Reset highlight whenever the result set changes or the palette (re)opens.
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setActive(0);
  }, [query, open]);

  // Group the filtered results, preserving order and the flat index used by the
  // keyboard highlight so arrows can cross group boundaries seamlessly.
  const groups = useMemo(() => {
    const order: string[] = [];
    const map = new Map<string, { item: CommandItem; index: number }[]>();
    filtered.forEach((item, index) => {
      if (!map.has(item.group)) {
        map.set(item.group, []);
        order.push(item.group);
      }
      map.get(item.group)!.push({ item, index });
    });
    return order.map((group) => ({ group, items: map.get(group)! }));
  }, [filtered]);

  useEffect(() => {
    listRef.current?.querySelector(`[data-index="${active}"]`)?.scrollIntoView({ block: "nearest" });
  }, [active]);

  function onKeyDown(event: React.KeyboardEvent) {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setActive((i) => (filtered.length ? (i + 1) % filtered.length : 0));
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setActive((i) => (filtered.length ? (i - 1 + filtered.length) % filtered.length : 0));
    } else if (event.key === "Enter") {
      event.preventDefault();
      filtered[active]?.run();
    }
  }

  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="srapi-anim-fade fixed inset-0 z-50 bg-srapi-text-primary/20 backdrop-blur-sm" />
        <Dialog.Content
          aria-label={t("commandPalette.placeholder")}
          onKeyDown={onKeyDown}
          onOpenAutoFocus={(event) => {
            event.preventDefault();
            inputRef.current?.focus();
          }}
          className="srapi-anim-pop tactile-card fixed inset-x-0 top-[14vh] z-50 mx-auto w-[calc(100%-2rem)] max-w-xl overflow-hidden rounded-2xl border border-srapi-border bg-srapi-card"
        >
          <Dialog.Title className="sr-only">{t("common.search")}</Dialog.Title>
          <div className="flex items-center gap-3 border-b border-srapi-border px-4">
            <Search className="size-4 shrink-0 text-srapi-text-tertiary" aria-hidden />
            <input
              ref={inputRef}
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder={t("commandPalette.placeholder")}
              className="h-12 w-full bg-transparent text-sm text-srapi-text-primary outline-none placeholder:text-srapi-text-tertiary"
              aria-label={t("commandPalette.placeholder")}
            />
            <kbd className="hidden shrink-0 rounded border border-srapi-border px-1.5 py-0.5 font-mono text-2xs text-srapi-text-tertiary sm:block">
              ESC
            </kbd>
          </div>

          <div ref={listRef} className="max-h-[52vh] overflow-y-auto p-2">
            {filtered.length === 0 ? (
              <div className="flex flex-col items-center justify-center gap-2 px-4 py-12 text-center text-srapi-text-tertiary">
                <Search className="size-5 opacity-50" aria-hidden />
                <span className="text-sm">{t("commandPalette.empty", { query })}</span>
              </div>
            ) : (
              groups.map(({ group, items }) => (
                <div key={group} className="mb-1 last:mb-0">
                  <div className="px-2 pb-1 pt-2 font-mono text-2xs uppercase tracking-wide text-srapi-text-tertiary">
                    {group}
                  </div>
                  {items.map(({ item, index }) => {
                    const Icon = item.icon;
                    const isActive = index === active;
                    return (
                      <button
                        key={item.id}
                        type="button"
                        data-index={index}
                        onMouseMove={() => setActive(index)}
                        onClick={() => item.run()}
                        className={cn(
                          "flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left text-sm outline-none transition-colors",
                          isActive
                            ? "bg-srapi-card-muted text-srapi-text-primary"
                            : "text-srapi-text-secondary",
                        )}
                      >
                        <Icon
                          className={cn(
                            "size-4 shrink-0",
                            isActive ? "text-srapi-primary" : "text-srapi-text-tertiary",
                          )}
                          aria-hidden
                        />
                        <span className="flex-1 truncate">{item.label}</span>
                        {isActive ? (
                          <CornerDownLeft className="size-3.5 shrink-0 text-srapi-text-tertiary" aria-hidden />
                        ) : null}
                      </button>
                    );
                  })}
                </div>
              ))
            )}
          </div>

          <div className="flex items-center gap-4 border-t border-srapi-border px-4 py-2 font-mono text-2xs text-srapi-text-tertiary">
            <span className="flex items-center gap-1">
              <Key>↑</Key>
              <Key>↓</Key>
              {t("commandPalette.hintNavigate")}
            </span>
            <span className="flex items-center gap-1">
              <Key>↵</Key>
              {t("commandPalette.hintSelect")}
            </span>
            <span className="ml-auto flex items-center gap-1">
              <Key>esc</Key>
              {t("commandPalette.hintClose")}
            </span>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

function Key({ children }: { children: React.ReactNode }) {
  return (
    <kbd className="rounded border border-srapi-border px-1 py-px font-mono text-2xs text-srapi-text-secondary">
      {children}
    </kbd>
  );
}
