"use client";

import { useEffect } from "react";
import { reportWebVitals } from "@/lib/telemetry";

export function WebVitalsReporter() {
  useEffect(() => {
    reportWebVitals();
  }, []);
  return null;
}
