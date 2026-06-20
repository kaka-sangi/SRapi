import {
  LayoutDashboard,
  KeyRound,
  ChartLine,
  CloudCog,
  Radio,
  Users,
  Group,
  Globe,
  Cable,
  ReceiptText,
  HeartPulse,
  SlidersHorizontal,
  CircleUser,
  Wallet,
  Gem,
  Handshake,
  CircleDollarSign,
  Ticket,
  ShieldAlert,
  Megaphone,
  FileSearch,
  FileText,
  MailCheck,
  Compass,
  BrainCircuit,
  Send,
  Sparkles,
  Stethoscope,
  ShieldCheck,
  Fingerprint,
  Receipt,
  BellRing,
  Network,
  type LucideIcon,
} from "lucide-react";
import { ADMIN_ROUTES, USER_ROUTES, USER_HOME_ROUTE } from "@/lib/routes";

export interface NavItem {
  href: string;
  labelKey: string;
  icon: LucideIcon;
}

export interface NavSection {
  titleKey: string;
  items: NavItem[];
}

/** Workspace section — the signed-in user's own gateway usage. */
export const WORKSPACE_SECTION: NavSection = {
  titleKey: "nav.sectionWorkspace",
  items: [
    { href: USER_HOME_ROUTE, labelKey: "nav.dashboard", icon: LayoutDashboard },
    { href: USER_ROUTES.playground, labelKey: "nav.playground", icon: Compass },
    { href: USER_ROUTES.availableChannels, labelKey: "nav.availableChannels", icon: Radio },
    { href: "/api-keys", labelKey: "nav.apiKeys", icon: KeyRound },
    { href: "/usage", labelKey: "nav.usage", icon: ChartLine },
  ],
};

/** Account self-service — visible to regular users. */
export const ACCOUNT_SECTION: NavSection = {
  titleKey: "nav.sectionAccount",
  items: [
    { href: USER_ROUTES.account, labelKey: "nav.account", icon: CircleUser },
    { href: USER_ROUTES.billing, labelKey: "nav.billing", icon: Wallet },
    { href: USER_ROUTES.redeem, labelKey: "nav.redeem", icon: Gem },
    { href: USER_ROUTES.affiliate, labelKey: "nav.affiliate", icon: Handshake },
  ],
};

// Admin navigation is grouped by domain. The aggregated tabbed pages
// (/admin/billing-admin, /admin/affiliates, /admin/logs, /admin/gateway-
// policies, /admin/identity) each get a SINGLE sidebar entry — operators
// switch tabs in-page. Surfacing every tab here re-created the visual
// sprawl the aggregation was meant to fix and broke the active-state
// highlight (usePathname strips ?tab=... so /admin/billing-admin?tab=plans
// never matched a sidebar href with `?tab=plans` in it).
const ADMIN_OVERVIEW_SECTION: NavSection = {
  titleKey: "nav.sectionAdminOverview",
  items: [
    { href: ADMIN_ROUTES.dashboard, labelKey: "nav.dashboard", icon: LayoutDashboard },
    { href: ADMIN_ROUTES.users, labelKey: "nav.adminUsers", icon: Users },
    { href: ADMIN_ROUTES.usage, labelKey: "nav.adminUsage", icon: ChartLine },
  ],
};

const ADMIN_GATEWAY_SECTION: NavSection = {
  titleKey: "nav.sectionAdminGateway",
  items: [
    { href: ADMIN_ROUTES.gatewayResources, labelKey: "nav.adminGatewayResources", icon: Network },
    { href: ADMIN_ROUTES.quickSetup, labelKey: "nav.adminQuickSetup", icon: Sparkles },
    { href: ADMIN_ROUTES.providers, labelKey: "nav.adminProviders", icon: Cable },
    { href: ADMIN_ROUTES.models, labelKey: "nav.adminModels", icon: BrainCircuit },
    { href: ADMIN_ROUTES.channelsPricing, labelKey: "nav.adminChannelsPricing", icon: CircleDollarSign },
    { href: ADMIN_ROUTES.gatewayPolicies, labelKey: "nav.adminGatewayPolicies", icon: ShieldCheck },
    { href: ADMIN_ROUTES.accounts, labelKey: "nav.adminAccounts", icon: CloudCog },
    { href: ADMIN_ROUTES.groups, labelKey: "nav.adminGroups", icon: Group },
    { href: ADMIN_ROUTES.apiKeys, labelKey: "nav.adminApiKeys", icon: KeyRound },
  ],
};

const ADMIN_COMMERCE_SECTION: NavSection = {
  titleKey: "nav.sectionAdminCommerce",
  items: [
    { href: ADMIN_ROUTES.billingAdmin, labelKey: "nav.adminBillingAdmin", icon: Receipt },
    { href: ADMIN_ROUTES.orders, labelKey: "nav.adminOrders", icon: ReceiptText },
    { href: ADMIN_ROUTES.promoCodes, labelKey: "nav.adminPromoCodes", icon: Ticket },
    { href: ADMIN_ROUTES.redeem, labelKey: "nav.adminRedeem", icon: Gem },
    { href: ADMIN_ROUTES.affiliates, labelKey: "nav.adminAffiliates", icon: Handshake },
  ],
};

const ADMIN_OPS_SECTION: NavSection = {
  titleKey: "nav.sectionAdminOps",
  items: [
    { href: ADMIN_ROUTES.ops, labelKey: "nav.adminOps", icon: HeartPulse },
    { href: ADMIN_ROUTES.opsAlertEvents, labelKey: "nav.adminOpsAlertEvents", icon: BellRing },
    { href: ADMIN_ROUTES.riskControl, labelKey: "nav.adminRiskControl", icon: ShieldAlert },
    { href: ADMIN_ROUTES.announcements, labelKey: "nav.adminAnnouncements", icon: Megaphone },
    { href: ADMIN_ROUTES.eventsOutbox, labelKey: "nav.adminOutbox", icon: Send },
    { href: ADMIN_ROUTES.opsSystemLogs, labelKey: "nav.adminOpsSystemLogs", icon: FileText },
    { href: ADMIN_ROUTES.diagnostics, labelKey: "nav.adminDiagnostics", icon: Stethoscope },
  ],
};

const ADMIN_SYSTEM_SECTION: NavSection = {
  titleKey: "nav.sectionAdminSystem",
  items: [
    { href: ADMIN_ROUTES.proxies, labelKey: "nav.adminProxies", icon: Globe },
    { href: ADMIN_ROUTES.identity, labelKey: "nav.adminIdentity", icon: Fingerprint },
    { href: ADMIN_ROUTES.logs, labelKey: "nav.adminLogs", icon: FileSearch },
    {
      href: ADMIN_ROUTES.notificationTemplates,
      labelKey: "nav.adminNotificationTemplates",
      icon: MailCheck,
    },
    { href: ADMIN_ROUTES.settings, labelKey: "nav.adminSettings", icon: SlidersHorizontal },
  ],
};

export const ADMIN_SECTIONS: NavSection[] = [
  ADMIN_OVERVIEW_SECTION,
  ADMIN_GATEWAY_SECTION,
  ADMIN_COMMERCE_SECTION,
  ADMIN_OPS_SECTION,
  ADMIN_SYSTEM_SECTION,
];

export function navSectionsForRole(role: "admin" | "user"): NavSection[] {
  if (role === "admin") {
    return [...ADMIN_SECTIONS, WORKSPACE_SECTION, ACCOUNT_SECTION];
  }
  return [WORKSPACE_SECTION, ACCOUNT_SECTION];
}
