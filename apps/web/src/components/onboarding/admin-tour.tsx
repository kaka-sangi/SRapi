"use client";

import { useEffect } from "react";
import { usePathname } from "next/navigation";
import { useTour, type TourStep } from "./tour-provider";
import { useLanguage } from "@/context/LanguageContext";

const TOUR_STORAGE_KEY = "srapi_admin_tour_done";

export function AdminTourTrigger() {
  const { start, isActive } = useTour();
  const { t } = useLanguage();
  const pathname = usePathname();

  useEffect(() => {
    if (isActive) return;
    if (pathname !== "/admin/dashboard" && pathname !== "/admin") return;
    if (typeof window === "undefined") return;
    if (localStorage.getItem(TOUR_STORAGE_KEY) === "1") return;

    const timer = setTimeout(() => {
      const steps: TourStep[] = [
        {
          target: "[data-tour='nav-quick-setup']",
          title: t("tour.quickSetupTitle"),
          content: t("tour.quickSetupContent"),
          placement: "right",
        },
        {
          target: "[data-tour='nav-accounts']",
          title: t("tour.accountsTitle"),
          content: t("tour.accountsContent"),
          placement: "right",
        },
        {
          target: "[data-tour='nav-models']",
          title: t("tour.modelsTitle"),
          content: t("tour.modelsContent"),
          placement: "right",
        },
        {
          target: "[data-tour='search-bar']",
          title: t("tour.searchTitle"),
          content: t("tour.searchContent"),
          placement: "bottom",
        },
      ];

      const allExist = steps.every((s) => document.querySelector(s.target));
      if (allExist) {
        start(steps);
        localStorage.setItem(TOUR_STORAGE_KEY, "1");
      }
    }, 2000);

    return () => clearTimeout(timer);
  }, [pathname, isActive, start, t]);

  return null;
}
