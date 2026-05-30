"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { apiService } from "@/lib/api";
import { homeRouteForRole } from "@/lib/routes";

/**
 * Redirect already-authenticated visitors away from public surfaces (`/`,
 * `/login`) to their role's home. Uses the locally cached session so it runs
 * instantly on the client; the edge proxy handles the server-side case.
 */
export function useAuthRedirect() {
  const router = useRouter();
  React.useEffect(() => {
    const user = apiService.getCurrentUser();
    if (user) {
      router.replace(homeRouteForRole(user.role));
    }
  }, [router]);
}
