"use client";

import * as React from "react";
import { reportWebVitals } from "@/lib/telemetry";

export function WebVitalsReporter() {
  React.useEffect(() => {
    reportWebVitals();
  }, []);
  return null;
}
