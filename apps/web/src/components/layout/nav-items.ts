import {
  LayoutDashboard,
  KeyRound,
  ChartLine,
  CloudCog,
  Route,
  Radio,
  Users,
  Group,
  Globe,
  Cable,
  CreditCard,
  ReceiptText,
  HeartPulse,
  SlidersHorizontal,
  CircleUser,
  Wallet,
  Gem,
  Handshake,
  Layers,
  CircleDollarSign,
  Building2,
  Ticket,
  UserPlus,
  Coins,
  ArrowLeftRight,
  Scale,
  Percent,
  ShieldAlert,
  Lock,
  Megaphone,
  FileSearch,
  Receipt,
  FileText,
  UserCog,
  MailCheck,
  Compass,
  BrainCircuit,
  Send,
  Sparkles,
  Stethoscope,
  Replace,
  OctagonAlert,
  Bug,
  Fingerprint,
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

// Admin navigation is grouped by domain so every admin surface is reachable from
// the sidebar (previously only ~11 of the ~22 admin pages had a nav entry, which
// left commerce / affiliate / ops sub-pages reachable only by typing the URL).
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
    { href: ADMIN_ROUTES.quickSetup, labelKey: "nav.adminQuickSetup", icon: Sparkles },
    { href: ADMIN_ROUTES.providers, labelKey: "nav.adminProviders", icon: Cable },
    { href: ADMIN_ROUTES.models, labelKey: "nav.adminModels", icon: BrainCircuit },
    { href: ADMIN_ROUTES.channelsPricing, labelKey: "nav.adminChannelsPricing", icon: CircleDollarSign },
    { href: ADMIN_ROUTES.payloadRules, labelKey: "nav.adminPayloadRules", icon: Replace },
    { href: ADMIN_ROUTES.accounts, labelKey: "nav.adminAccounts", icon: CloudCog },
    { href: ADMIN_ROUTES.groups, labelKey: "nav.adminGroups", icon: Group },
  ],
};

const ADMIN_COMMERCE_SECTION: NavSection = {
  titleKey: "nav.sectionAdminCommerce",
  items: [
    { href: ADMIN_ROUTES.subscriptions, labelKey: "nav.adminSubscriptions", icon: CreditCard },
    { href: ADMIN_ROUTES.ordersPlans, labelKey: "nav.adminOrdersPlans", icon: Layers },
    { href: ADMIN_ROUTES.orders, labelKey: "nav.adminOrders", icon: ReceiptText },
    { href: ADMIN_ROUTES.paymentProviders, labelKey: "nav.adminPaymentProviders", icon: Building2 },
    { href: ADMIN_ROUTES.promoCodes, labelKey: "nav.adminPromoCodes", icon: Ticket },
    { href: ADMIN_ROUTES.redeem, labelKey: "nav.adminRedeem", icon: Gem },
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
    {
      href: ADMIN_ROUTES.affiliatesWithdrawals,
      labelKey: "nav.adminAffiliatesWithdrawals",
      icon: Wallet,
    },
    {
      href: ADMIN_ROUTES.affiliatesManualAdjustments,
      labelKey: "nav.adminAffiliatesManualAdjustments",
      icon: Scale,
    },
    { href: ADMIN_ROUTES.affiliatesRules, labelKey: "nav.adminAffiliatesRules", icon: Percent },
  ],
};

const ADMIN_OPS_SECTION: NavSection = {
  titleKey: "nav.sectionAdminOps",
  items: [
    { href: ADMIN_ROUTES.ops, labelKey: "nav.adminOps", icon: HeartPulse },
    { href: ADMIN_ROUTES.opsStrategy, labelKey: "nav.adminOpsStrategy", icon: Route },
    { href: ADMIN_ROUTES.errorPassthrough, labelKey: "nav.adminErrorPassthrough", icon: OctagonAlert },
    { href: ADMIN_ROUTES.errorLogs, labelKey: "nav.adminErrorLogs", icon: Bug },
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
    { href: ADMIN_ROUTES.tlsProfiles, labelKey: "nav.adminTlsProfiles", icon: Fingerprint },
    { href: ADMIN_ROUTES.roles, labelKey: "nav.adminRoles", icon: Lock },
    { href: ADMIN_ROUTES.apiKeys, labelKey: "nav.adminApiKeys", icon: KeyRound },
    { href: ADMIN_ROUTES.auditLogs, labelKey: "nav.adminAuditLogs", icon: FileSearch },
    { href: ADMIN_ROUTES.userAttributes, labelKey: "nav.adminUserAttributes", icon: UserCog },
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
  ADMIN_AFFILIATE_SECTION,
  ADMIN_OPS_SECTION,
  ADMIN_SYSTEM_SECTION,
];

export function navSectionsForRole(role: "admin" | "user"): NavSection[] {
  if (role === "admin") {
    return [...ADMIN_SECTIONS, WORKSPACE_SECTION, ACCOUNT_SECTION];
  }
  return [WORKSPACE_SECTION, ACCOUNT_SECTION];
}
