import { describe, expect, it } from "vitest";
import { ADMIN_HOME_ROUTE, USER_HOME_ROUTE, homeRouteForRole } from "@/lib/routes";

describe("console routes", () => {
  it("uses the production dashboard as the admin home", () => {
    expect(ADMIN_HOME_ROUTE).toBe("/admin/dashboard");
    expect(homeRouteForRole("admin")).toBe("/admin/dashboard");
    expect(homeRouteForRole("user")).toBe(USER_HOME_ROUTE);
  });
});
