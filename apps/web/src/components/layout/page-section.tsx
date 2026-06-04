import * as React from "react";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";

/**
 * A titled content section on the warm-paper surface. Use for dashboard/detail
 * blocks (charts, summaries, key-value panels) so section headers stop being
 * hand-rolled with ad-hoc eyebrow spans and instead share the
 * Card / CardHeader / CardTitle rhythm used everywhere else. For data tables
 * prefer `AdminListView`; for plain forms use `Card` directly.
 */
export function PageSection({
  title,
  action,
  description,
  className,
  bodyClassName,
  children,
}: {
  title: React.ReactNode;
  action?: React.ReactNode;
  description?: React.ReactNode;
  className?: string;
  bodyClassName?: string;
  children: React.ReactNode;
}) {
  return (
    <Card className={className}>
      <CardHeader>
        <div className="min-w-0">
          <CardTitle>{title}</CardTitle>
          {description && (
            <p className="mt-0.5 text-2xs text-srapi-text-tertiary">{description}</p>
          )}
        </div>
        {action}
      </CardHeader>
      <CardContent className={bodyClassName}>{children}</CardContent>
    </Card>
  );
}
