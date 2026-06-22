import { en } from "./en";
import { zh } from "./zh";

export type Locale = "en" | "zh";

export const DEFAULT_LOCALE: Locale = "zh";
const LOCALES: Locale[] = ["zh", "en"];

type Dict = Record<string, string>;

// Flatten nested message objects into dot-keyed string maps.
function flatten(obj: Record<string, unknown>, prefix = "", out: Dict = {}): Dict {
  for (const [key, value] of Object.entries(obj)) {
    const full = prefix ? `${prefix}.${key}` : key;
    if (value && typeof value === "object") {
      flatten(value as Record<string, unknown>, full, out);
    } else {
      out[full] = String(value);
    }
  }
  return out;
}

const registry: Record<Locale, Dict> = {
  en: flatten(en),
  zh: flatten(zh),
};

// Interpolate {var} placeholders.
export function applyVariables(template: string, vars?: Record<string, string | number>): string {
  if (!vars) return template;
  return template.replace(/\{(\w+)\}/g, (match, name: string) =>
    name in vars ? String(vars[name]) : match,
  );
}

export function translate(
  locale: Locale,
  key: string,
  vars?: Record<string, string | number>,
): string {
  const value = registry[locale][key] ?? registry[DEFAULT_LOCALE][key] ?? key;
  return applyVariables(value, vars);
}

// registry is internal — not re-exported
