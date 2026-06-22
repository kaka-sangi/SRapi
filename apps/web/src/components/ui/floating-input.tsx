import * as React from "react";
import { cn } from "@/lib/cn";

export interface FloatingInputProps {
  label: string;
  value: string;
  onChange: (v: string) => void;
  type?: string;
  error?: string;
  hint?: string;
  id?: string;
  required?: boolean;
  autoComplete?: string;
  disabled?: boolean;
  className?: string;
  placeholder?: string;
}

/**
 * Floating-label input — modern auth/settings feel.
 *
 * The label rests inside the 56px input when empty and chip-floats to the
 * top-left when focused or filled. Error state adds a red border, a shake
 * animation, and an a11y-labelled error message below.
 */
export function FloatingInput({
  label,
  value,
  onChange,
  type = "text",
  error,
  hint,
  id,
  required,
  autoComplete,
  disabled,
  className,
  placeholder,
}: FloatingInputProps) {
  const reactId = React.useId();
  const inputId = id ?? `floating-input-${reactId}`;
  const errorId = `${inputId}-error`;
  const hintId = `${inputId}-hint`;

  // Track previous error text to re-trigger shake animation on each new error.
  const [shakeKey, setShakeKey] = React.useState(0);
  const prevErrorRef = React.useRef<string | undefined>(error);
  React.useEffect(() => {
    if (error && error !== prevErrorRef.current) {
      setShakeKey((k) => k + 1);
    }
    prevErrorRef.current = error;
  }, [error]);

  return (
    <div className={cn("relative", className)}>
      <div className="relative">
        <input
          id={inputId}
          type={type}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder={placeholder ?? " "}
          required={required}
          autoComplete={autoComplete}
          disabled={disabled}
          aria-invalid={error ? true : undefined}
          aria-describedby={error ? errorId : hint ? hintId : undefined}
          className={cn(
            "peer h-12 w-full rounded-lg border border-srapi-border bg-transparent px-3 pt-5 pb-1 text-base text-srapi-text-primary outline-none transition-colors",
            "placeholder:text-transparent",
            "focus:border-srapi-primary focus:ring-2 focus:ring-srapi-primary/15",
            "disabled:cursor-not-allowed disabled:opacity-50",
            error
              ? "border-srapi-error focus:border-srapi-error focus:ring-srapi-error/15"
              : "border-srapi-border hover:border-srapi-border-strong",
            error && "anim-shake",
          )}
          key={error ? `err-${shakeKey}` : "ok"}
        />
        <label
          htmlFor={inputId}
          className={cn(
            "pointer-events-none absolute left-4 top-1/2 -translate-y-1/2 text-sm text-srapi-text-tertiary transition-all duration-150",
            "peer-focus:top-3 peer-focus:translate-y-0 peer-focus:text-[11px] peer-focus:font-medium peer-focus:text-srapi-primary",
            "peer-[:not(:placeholder-shown)]:top-3 peer-[:not(:placeholder-shown)]:translate-y-0 peer-[:not(:placeholder-shown)]:text-[11px] peer-[:not(:placeholder-shown)]:font-medium",
            error && "text-srapi-error peer-focus:text-srapi-error peer-[:not(:placeholder-shown)]:text-srapi-error",
          )}
        >
          {label}
          {required ? " *" : null}
        </label>
      </div>
      {error ? (
        <p
          id={errorId}
          role="alert"
          className="mt-1 text-xs text-srapi-error"
        >
          {error}
        </p>
      ) : hint ? (
        <p id={hintId} className="mt-1 text-xs text-srapi-text-tertiary">
          {hint}
        </p>
      ) : null}
    </div>
  );
}
FloatingInput.displayName = "FloatingInput";
