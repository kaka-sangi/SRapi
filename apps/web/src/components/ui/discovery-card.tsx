import * as React from "react";
import Link from "next/link";
import { ArrowUpRight } from "lucide-react";
import { cn } from "@/lib/cn";
import { Card } from "./card";

export interface DiscoveryCardProps {
  icon: React.ReactNode;
  title: string;
  description?: string;
  footer?: React.ReactNode;
  href?: string;
  tone?: "default" | "accent";
  className?: string;
}

export const DiscoveryCard = React.forwardRef<HTMLDivElement, DiscoveryCardProps>(
  ({ icon, title, description, footer, href, tone = "default", className }, ref) => {
    const content = (
      <Card
        ref={ref}
        className={cn(
          "card-interactive group h-full",
          tone === "accent" && "border-srapi-primary/30 bg-srapi-accent-soft/30",
          className,
        )}
      >
        <div className="flex flex-col gap-4 p-5">
          <div className="flex items-start justify-between gap-3">
            <span className="grid size-11 place-items-center rounded-xl bg-srapi-accent-soft text-srapi-primary [&>svg]:size-5 transition-transform duration-200 group-hover:scale-105">
              {icon}
            </span>
            {href ? (
              <ArrowUpRight
                aria-hidden
                className="size-4 text-srapi-text-tertiary transition-transform duration-200 group-hover:-translate-y-0.5 group-hover:translate-x-0.5 group-hover:text-srapi-primary"
              />
            ) : null}
          </div>
          <div className="flex flex-col gap-1.5">
            <h3 className="text-base font-semibold tracking-tight text-srapi-text-primary">
              {title}
            </h3>
            {description ? (
              <p className="text-sm leading-relaxed text-srapi-text-secondary">{description}</p>
            ) : null}
          </div>
          {footer ? (
            <div className="mt-auto border-t border-srapi-border/70 pt-3">{footer}</div>
          ) : null}
        </div>
      </Card>
    );

    if (href) {
      return (
        <Link href={href} className="block h-full focus:outline-none">
          {content}
        </Link>
      );
    }

    return content;
  },
);
DiscoveryCard.displayName = "DiscoveryCard";
