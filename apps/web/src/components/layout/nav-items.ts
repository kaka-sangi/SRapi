import {
  LayoutGrid,
  KeyRound,
  CalendarClock,
  BarChart3,
  Server,
  GitBranch,
  Users,
  Boxes,
  Network,
  Plug,
  Cpu,
  CreditCard,
  ShoppingCart,
  Activity,
  Settings,
  UserRound,
  Wallet,
  Gift,
  Share2,
  Layers,
  Tag,
  Landmark,
  Ticket,
  UserPlus,
  Coins,
  ArrowLeftRight,
  Shield,
  ShieldAlert,
  ShieldCheck,
  Fingerprint,
  SlidersHorizontal,
  Megaphone,
  ScrollText,
  Receipt,
  Tags,
  Mail,
  Gauge,
  Bot,
  Webhook,
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
    { href: USER_HOME_ROUTE, labelKey: "nav.dashboard", icon: LayoutGrid },
    { href: USER_ROUTES.playground, labelKey: "nav.playground", icon: Bot },
    { href: "/api-keys", labelKey: "nav.apiKeys", icon: KeyRound },
    { href: "/usage", labelKey: "nav.usage", icon: BarChart3 },
  ],
};

/** Account self-service — visible to regular users. */
export const ACCOUNT_SECTION: NavSection = {
  titleKey: "nav.sectionAccount",
  items: [
    { href: USER_ROUTES.account, labelKey: "nav.account", icon: UserRound },
    { href: USER_ROUTES.billing, labelKey: "nav.billing", icon: Wallet },
    { href: USER_ROUTES.redeem, labelKey: "nav.redeem", icon: Gift },
    { href: USER_ROUTES.affiliate, labelKey: "nav.affiliate", icon: Share2 },
  ],
};

// Operator view of gateway internals (account health, scheduler decisions).
// Both pages are admin-gated (no `/me` endpoint exists for this data), so this
// section is shown to admins only — never in the regular user workspace.
export const GATEWAY_SECTION: NavSection = {
  titleKey: "nav.sectionGateway",
  items: [
    { href: "/provider-accounts", labelKey: "nav.providerAccounts", icon: Server },
    { href: "/scheduler-decisions", labelKey: "nav.schedulerDecisions", icon: GitBranch },
  ],
};

// Admin navigation is grouped by domain so every admin surface is reachable from
// the sidebar (previously only ~11 of the ~22 admin pages had a nav entry, which
// left commerce / affiliate / ops sub-pages reachable only by typing the URL).
const ADMIN_OVERVIEW_SECTION: NavSection = {
  titleKey: "nav.sectionAdminOverview",
  items: [
    { href: ADMIN_ROUTES.dashboard, labelKey: "nav.dashboard", icon: LayoutGrid },
    { href: ADMIN_ROUTES.users, labelKey: "nav.adminUsers", icon: Users },
    { href: ADMIN_ROUTES.usage, labelKey: "nav.adminUsage", icon: BarChart3 },
  ],
};

const ADMIN_GATEWAY_SECTION: NavSection = {
  titleKey: "nav.sectionAdminGateway",
  items: [
    { href: ADMIN_ROUTES.providers, labelKey: "nav.adminProviders", icon: Plug },
    { href: ADMIN_ROUTES.models, labelKey: "nav.adminModels", icon: Cpu },
    { href: ADMIN_ROUTES.accounts, labelKey: "nav.adminAccounts", icon: Server },
    { href: ADMIN_ROUTES.groups, labelKey: "nav.adminGroups", icon: Boxes },
    { href: ADMIN_ROUTES.proxies, labelKey: "nav.adminProxies", icon: Network },
    { href: ADMIN_ROUTES.errorPassthrough, labelKey: "nav.adminErrorPassthrough", icon: ShieldAlert },
    { href: ADMIN_ROUTES.tlsProfiles, labelKey: "nav.adminTlsProfiles", icon: Fingerprint },
    { href: ADMIN_ROUTES.payloadRules, labelKey: "nav.adminPayloadRules", icon: SlidersHorizontal },
  ],
};

const ADMIN_COMMERCE_SECTION: NavSection = {
  titleKey: "nav.sectionAdminCommerce",
  items: [
    { href: ADMIN_ROUTES.subscriptions, labelKey: "nav.adminSubscriptions", icon: CreditCard },
    { href: ADMIN_ROUTES.ordersPlans, labelKey: "nav.adminOrdersPlans", icon: Layers },
    { href: ADMIN_ROUTES.orders, labelKey: "nav.adminOrders", icon: ShoppingCart },
    { href: ADMIN_ROUTES.channelsPricing, labelKey: "nav.adminChannelsPricing", icon: Tag },
    { href: ADMIN_ROUTES.paymentProviders, labelKey: "nav.adminPaymentProviders", icon: Landmark },
    { href: ADMIN_ROUTES.promoCodes, labelKey: "nav.adminPromoCodes", icon: Ticket },
    { href: ADMIN_ROUTES.redeem, labelKey: "nav.adminRedeem", icon: Gift },
    { href: ADMIN_ROUTES.billingLedger, labelKey: "nav.adminBillingLedger", icon: Receipt },
  ],
};

const ADMIN_AFFILIATE_SECTION: NavSection = {
  titleKey: "nav.sectionAdminAffiliate",
  items: [
    { href: ADMIN_ROUTES.affiliatesInvites, labelKey: "nav.adminAffiliatesInvites", icon: UserPlus },
    { href: ADMIN_ROUTES.affiliatesRebates, labelKey: "nav.adminAffiliatesRebates", icon: Coins },
    {
      href: ADMIN_ROUTES.affiliatesTransfers,
      labelKey: "nav.adminAffiliatesTransfers",
      icon: ArrowLeftRight,
    },
  ],
};

const ADMIN_OPS_SECTION: NavSection = {
  titleKey: "nav.sectionAdminOps",
  items: [
    { href: ADMIN_ROUTES.ops, labelKey: "nav.adminOps", icon: Activity },
    { href: ADMIN_ROUTES.channelsMonitor, labelKey: "nav.adminMonitor", icon: Gauge },
    { href: ADMIN_ROUTES.scheduledTests, labelKey: "nav.adminScheduledTests", icon: CalendarClock },
    { href: ADMIN_ROUTES.opsStrategy, labelKey: "nav.adminOpsStrategy", icon: GitBranch },
    { href: ADMIN_ROUTES.riskControl, labelKey: "nav.adminRiskControl", icon: Shield },
    { href: ADMIN_ROUTES.announcements, labelKey: "nav.adminAnnouncements", icon: Megaphone },
    { href: ADMIN_ROUTES.auditLogs, labelKey: "nav.adminAuditLogs", icon: ScrollText },
    { href: ADMIN_ROUTES.eventsOutbox, labelKey: "nav.adminOutbox", icon: Webhook },
  ],
};

const ADMIN_SYSTEM_SECTION: NavSection = {
  titleKey: "nav.sectionAdminSystem",
  items: [
    // The AI copilot is reached via the floating 小r pet (components/admin/
    // copilot-pet.tsx), not a sidebar entry.
    { href: ADMIN_ROUTES.roles, labelKey: "nav.adminRoles", icon: ShieldCheck },
    { href: ADMIN_ROUTES.apiKeys, labelKey: "nav.adminApiKeys", icon: KeyRound },
    { href: ADMIN_ROUTES.userAttributes, labelKey: "nav.adminUserAttributes", icon: Tags },
    {
      href: ADMIN_ROUTES.notificationTemplates,
      labelKey: "nav.adminNotificationTemplates",
      icon: Mail,
    },
    { href: ADMIN_ROUTES.settings, labelKey: "nav.adminSettings", icon: Settings },
  ],
};

export const ADMIN_SECTIONS: NavSection[] = [
  ADMIN_OVERVIEW_SECTION,
  ADMIN_GATEWAY_SECTION,
  ADMIN_COMMERCE_SECTION,
  ADMIN_AFFILIATE_SECTION,
  ADMIN_OPS_SECTION,
  ADMIN_SYSTEM_SECTION,
];

export function navSectionsForRole(role: "admin" | "user"): NavSection[] {
  if (role === "admin") {
    return [...ADMIN_SECTIONS, GATEWAY_SECTION, WORKSPACE_SECTION, ACCOUNT_SECTION];
  }
  return [WORKSPACE_SECTION, ACCOUNT_SECTION];
}
