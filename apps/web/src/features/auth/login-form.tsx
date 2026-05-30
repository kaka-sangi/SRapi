"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { ArrowRight } from "lucide-react";
import { apiService } from "@/lib/api";
import { homeRouteForRole } from "@/lib/routes";
import { useLanguage } from "@/context/LanguageContext";
import { Button, Input, Label } from "@/components/ui";
import { cn } from "@/lib/cn";

const loginSchema = z.object({
  email: z.string().email(),
  password: z.string().min(1),
});

type LoginValues = z.infer<typeof loginSchema>;

/**
 * SRapi sign-in form. Self-contained: validates with zod, authenticates through
 * `apiService.login`, then routes to the role's home. Reused by the marketing
 * landing (`/`) and the focused `/login` page.
 *
 * Stable hooks the e2e/unit suites depend on: email placeholder
 * `operator@srapi.local`, password placeholder `••••••••••••`, submit id
 * `login-submit`, and the `t("authenticate")` ("Sign in") label.
 */
export function LoginForm({ className }: { className?: string }) {
  const router = useRouter();
  const { t } = useLanguage();
  const [serverError, setServerError] = React.useState("");

  const {
    register,
    handleSubmit,
    formState: { isSubmitting, errors },
  } = useForm<LoginValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: { email: "", password: "" },
  });

  const onSubmit = async (values: LoginValues) => {
    setServerError("");
    try {
      const user = await apiService.login(values.email, values.password);
      router.push(homeRouteForRole(user.role));
    } catch (err) {
      setServerError(err instanceof Error ? err.message : t("authRejected"));
    }
  };

  const fieldError = errors.email || errors.password ? t("loginError") : "";

  return (
    <form
      method="post"
      onSubmit={handleSubmit(onSubmit)}
      className={cn("space-y-6", className)}
      noValidate
    >
      {(serverError || fieldError) && (
        <div
          role="alert"
          className="animate-bloom-soft rounded-xl border border-srapi-error/25 bg-srapi-error/5 p-3.5 text-xs leading-relaxed text-srapi-error"
        >
          {serverError || fieldError}
        </div>
      )}

      <div className="space-y-2">
        <Label htmlFor="email">{t("operatorIdentity")}</Label>
        <Input
          id="email"
          type="email"
          autoComplete="username"
          placeholder="operator@srapi.local"
          aria-invalid={errors.email ? true : undefined}
          {...register("email")}
        />
      </div>

      <div className="space-y-2">
        <Label htmlFor="password">{t("consolePassphrase")}</Label>
        <Input
          id="password"
          type="password"
          autoComplete="current-password"
          placeholder="••••••••••••"
          aria-invalid={errors.password ? true : undefined}
          {...register("password")}
        />
      </div>

      <Button
        id="login-submit"
        type="submit"
        size="xl"
        disabled={isSubmitting}
        className="w-full justify-center"
      >
        {isSubmitting ? t("decrypting") : t("authenticate")}
        <ArrowRight size={14} aria-hidden="true" />
      </Button>
    </form>
  );
}
