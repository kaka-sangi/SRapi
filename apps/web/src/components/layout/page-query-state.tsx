"use client";

import { AlertTriangle, RefreshCw } from "lucide-react";
import { Button, Card, Spinner } from "@/components/ui";
import { adminErrorMessage } from "@/lib/admin-api";

export function PageQueryError({
  error,
  onRetry,
  title = "API request failed",
}: {
  error: unknown;
  onRetry?: () => void;
  title?: string;
}) {
  return (
    <Card className="space-y-4 border-srapi-error/30 bg-srapi-error/5 p-6" role="alert">
      <div className="flex items-start gap-3">
        <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-srapi-error" />
        <div className="space-y-1">
          <div className="font-mono text-xs font-bold uppercase tracking-wider text-srapi-error">
            {title}
          </div>
          <p className="text-xs leading-relaxed text-srapi-text-secondary">
            {adminErrorMessage(error)}
          </p>
        </div>
      </div>
      {onRetry ? (
        <Button type="button" variant="outline" size="sm" onClick={onRetry}>
          <RefreshCw size={12} />
          Retry
        </Button>
      ) : null}
    </Card>
  );
}

export function PageQueryLoading({ label }: { label: string }) {
  return (
    <div className="py-12 text-center font-mono">
      <Spinner size={24} label={label} />
    </div>
  );
}
