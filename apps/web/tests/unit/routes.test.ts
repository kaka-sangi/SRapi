import { describe, expect, it } from "vitest";
import {
  ADMIN_HOME_ROUTE,
  ADMIN_ROUTES,
  SIGN_IN_ROUTE,
  USER_HOME_ROUTE,
  homeRouteForRole,
} from "@/lib/routes";

// The route table is consumed by the sidebar nav, the post-login redirect,
// and the legacy-URL redirect stubs. Several entries point at TAB PARAMS of
// the aggregated pages (`/admin/billing-admin?tab=...`, `/admin/affiliates?
// tab=...`, etc.) — a "tidy up" PR that strips the query string would
// silently land users on the wrong tab and break external bookmarks.

describe("homeRouteForRole", () => {
  it("routes admins to the admin home", () => {
    expect(homeRouteForRole("admin")).toBe(ADMIN_HOME_ROUTE);
    expect(homeRouteForRole("admin")).toBe("/admin/dashboard");
  });

  it("routes regular users to the user home", () => {
    expect(homeRouteForRole("user")).toBe(USER_HOME_ROUTE);
    expect(homeRouteForRole("user")).toBe("/dashboard");
  });
});

describe("ADMIN_ROUTES — aggregation pass tab contracts", () => {
  it("billing-admin tabs all share the /admin/billing-admin base path", () => {
    // The catalog + sales config aggregation collapsed plans /
    // subscriptions / payment-providers into one tabbed page. Pin that
    // none of these entries silently drift back to their old standalone
    // paths during a "cleanup" PR.
    for (const key of ["billingAdmin", "subscriptions", "ordersPlans", "paymentProviders"] as const) {
      expect(ADMIN_ROUTES[key].startsWith("/admin/billing-admin")).toBe(true);
    }
    // Non-base entries must carry ?tab=...
    expect(ADMIN_ROUTES.subscriptions).toBe("/admin/billing-admin?tab=subscriptions");
    expect(ADMIN_ROUTES.ordersPlans).toBe("/admin/billing-admin?tab=plans");
    expect(ADMIN_ROUTES.paymentProviders).toBe("/admin/billing-admin?tab=payment-providers");
  });

  it("orders remains its OWN standalone page, NOT a billing-admin tab", () => {
    // The transactional order log is a separate concern from catalog
    // config — pin so a future "merge orders into billing-admin" sweep
    // doesn't quietly happen without redirect housekeeping.
    expect(ADMIN_ROUTES.orders).toBe("/admin/orders");
  });

  it("affiliate tabs all share the /admin/affiliates base path", () => {
    for (const key of [
      "affiliates",
      "affiliatesInvites",
      "affiliatesRebates",
      "affiliatesTransfers",
      "affiliatesWithdrawals",
      "affiliatesManualAdjustments",
      "affiliatesRules",
    ] as const) {
      expect(ADMIN_ROUTES[key].startsWith("/admin/affiliates")).toBe(true);
    }
    expect(ADMIN_ROUTES.affiliatesWithdrawals).toBe("/admin/affiliates?tab=withdrawals");
  });

  it("logs tabs all share the /admin/logs base path", () => {
    for (const key of ["logs", "auditLogs", "errorLogs", "billingLedger"] as const) {
      expect(ADMIN_ROUTES[key].startsWith("/admin/logs")).toBe(true);
    }
    // auditLogs collapses onto the bare logs path (it's the default tab).
    expect(ADMIN_ROUTES.auditLogs).toBe("/admin/logs");
    expect(ADMIN_ROUTES.errorLogs).toBe("/admin/logs?tab=error");
    expect(ADMIN_ROUTES.billingLedger).toBe("/admin/logs?tab=billing-ledger");
  });

  it("gateway-policies tabs all share the /admin/gateway-policies base path", () => {
    for (const key of ["gatewayPolicies", "errorPassthrough", "tlsProfiles", "payloadRules"] as const) {
      expect(ADMIN_ROUTES[key].startsWith("/admin/gateway-policies")).toBe(true);
    }
  });

  it("identity tabs share /admin/identity but users CRUD stays standalone", () => {
    // Identity config (roles + user-attribute schemas) is aggregated,
    // but the user DATA CRUD remains its own page at /admin/users — pin
    // that split so a future "everything-identity" sweep doesn't fold
    // them together and break user-management bookmarks.
    expect(ADMIN_ROUTES.identity).toBe("/admin/identity");
    expect(ADMIN_ROUTES.roles).toBe("/admin/identity?tab=roles");
    expect(ADMIN_ROUTES.userAttributes).toBe("/admin/identity?tab=user-attributes");
    expect(ADMIN_ROUTES.users).toBe("/admin/users");
  });
});

describe("home-route constants", () => {
  it("admin home is the admin dashboard", () => {
    expect(ADMIN_HOME_ROUTE).toBe("/admin/dashboard");
  });
  it("user home is the user dashboard (NOT the admin one)", () => {
    expect(USER_HOME_ROUTE).toBe("/dashboard");
    expect(USER_HOME_ROUTE).not.toBe(ADMIN_HOME_ROUTE);
  });
  it("sign-in route is the bare root", () => {
    expect(SIGN_IN_ROUTE).toBe("/");
  });
});
