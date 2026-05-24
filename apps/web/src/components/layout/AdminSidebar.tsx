"use client";

import React, { useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import {
  LayoutDashboard,
  Activity,
  Users,
  Layers,
  Sparkles,
  KeyRound,
  Megaphone,
  Network,
  Shield,
  Ticket,
  Tag,
  BarChart3,
  Settings,
  DollarSign,
  HeartPulse,
  Share2,
  ShoppingBag,
  ChevronDown,
  ChevronRight,
} from "lucide-react";
import { cn } from "@/lib/cn";
import { ADMIN_HOME_ROUTE } from "@/lib/routes";
import { useLanguage } from "@/context/LanguageContext";

interface SidebarItem {
  name: string;
  nameZh: string;
  href: string;
  icon: React.ComponentType<{ size?: number; className?: string }>;
}

interface SidebarGroup {
  name: string;
  nameZh: string;
  icon: React.ComponentType<{ size?: number; className?: string }>;
  items: SidebarItem[];
}

export function AdminSidebar() {
  const pathname = usePathname();
  const router = useRouter();
  const { language } = useLanguage();

  // Collapsible state for menu groups
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>({
    channels: pathname.includes("/admin/channels"),
    affiliates: pathname.includes("/admin/affiliates"),
    orders: pathname.includes("/admin/orders"),
  });

  const toggleGroup = (groupKey: string) => {
    setOpenGroups((prev) => ({
      ...prev,
      [groupKey]: !prev[groupKey],
    }));
  };

  const isChinese = language === "zh";

  const mainItems: SidebarItem[] = [
    { name: "Dashboard", nameZh: "数据面板", href: ADMIN_HOME_ROUTE, icon: LayoutDashboard },
    { name: "Operations", nameZh: "大屏监控", href: "/admin/ops", icon: Activity },
    { name: "User Management", nameZh: "用户管理", href: "/admin/users", icon: Users },
    { name: "Group Settings", nameZh: "分组配置", href: "/admin/groups", icon: Layers },
    { name: "Subscriptions", nameZh: "订阅计划", href: "/admin/subscriptions", icon: Sparkles },
    { name: "Accounts Pool", nameZh: "账号池", href: "/admin/accounts", icon: KeyRound },
    { name: "Announcements", nameZh: "系统公告", href: "/admin/announcements", icon: Megaphone },
    { name: "Proxies", nameZh: "代理配置", href: "/admin/proxies", icon: Network },
    { name: "Risk Control", nameZh: "安全风控", href: "/admin/risk-control", icon: Shield },
    { name: "Redeem Codes", nameZh: "兑换码", href: "/admin/redeem", icon: Ticket },
    { name: "Promo Codes", nameZh: "优惠码", href: "/admin/promo-codes", icon: Tag },
    { name: "Usage History", nameZh: "用量记录", href: "/admin/usage", icon: BarChart3 },
    { name: "System Settings", nameZh: "系统设置", href: "/admin/settings", icon: Settings },
  ];

  const groupMenus: Record<string, SidebarGroup> = {
    channels: {
      name: "Channels",
      nameZh: "渠道管理",
      icon: Layers,
      items: [
        { name: "Pricing", nameZh: "频道定价", href: "/admin/channels/pricing", icon: DollarSign },
        { name: "Monitor", nameZh: "频道状态", href: "/admin/channels/monitor", icon: HeartPulse },
      ],
    },
    affiliates: {
      name: "Affiliates",
      nameZh: "推广返利",
      icon: Share2,
      items: [
        { name: "Invites", nameZh: "邀请记录", href: "/admin/affiliates/invites", icon: Users },
        { name: "Rebates", nameZh: "返利明细", href: "/admin/affiliates/rebates", icon: DollarSign },
        { name: "Transfers", nameZh: "提现记录", href: "/admin/affiliates/transfers", icon: BarChart3 },
      ],
    },
    orders: {
      name: "Orders",
      nameZh: "订单管理",
      icon: ShoppingBag,
      items: [
        { name: "Dashboard", nameZh: "支付仪表盘", href: "/admin/orders/dashboard", icon: LayoutDashboard },
        { name: "Orders List", nameZh: "订单列表", href: "/admin/orders", icon: ShoppingBag },
        { name: "Plans Manager", nameZh: "套餐设定", href: "/admin/orders/plans", icon: Sparkles },
      ],
    },
  };

  const renderItem = (item: SidebarItem) => {
    const isActive = pathname === item.href;
    const Icon = item.icon;

    return (
      <button
        key={item.href}
        onClick={() => router.push(item.href)}
        className={cn(
          "w-full text-left px-3.5 py-2.5 rounded-xl font-mono text-xs uppercase tracking-wider flex items-center gap-3 transition-all cursor-pointer",
          isActive
            ? "bg-srapi-primary/10 text-srapi-primary font-bold border-l-2 border-srapi-primary"
            : "text-srapi-text-secondary hover:text-srapi-text-primary hover:bg-srapi-card-muted/40"
        )}
      >
        <Icon size={14} className={isActive ? "text-srapi-primary" : "text-srapi-text-secondary"} />
        <span>{isChinese ? item.nameZh : item.name}</span>
      </button>
    );
  };

  return (
    <aside className="bg-srapi-card border border-srapi-border rounded-3xl p-5 space-y-2.5 tactile-card">
      <div className="pb-3 border-b border-srapi-border mb-3 font-serif text-[11px] font-bold tracking-widest text-srapi-text-secondary uppercase">
        {isChinese ? "控制台中心" : "OPERATOR CONSOLE"}
      </div>

      <div className="space-y-1">
        {mainItems.slice(0, 5).map(renderItem)}

        {/* Collapsible Menu Groups */}
        {Object.entries(groupMenus).map(([key, group]) => {
          const isOpen = openGroups[key];
          const GroupIcon = group.icon;
          const isGroupActive = group.items.some((item) => pathname === item.href);

          return (
            <div key={key} className="space-y-1">
              <button
                onClick={() => toggleGroup(key)}
                className={cn(
                  "w-full text-left px-3.5 py-2.5 rounded-xl font-mono text-xs uppercase tracking-wider flex items-center justify-between transition-all cursor-pointer",
                  isGroupActive
                    ? "text-srapi-primary font-semibold"
                    : "text-srapi-text-secondary hover:text-srapi-text-primary hover:bg-srapi-card-muted/40"
                )}
              >
                <div className="flex items-center gap-3">
                  <GroupIcon size={14} />
                  <span>{isChinese ? group.nameZh : group.name}</span>
                </div>
                {isOpen ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
              </button>

              {isOpen && (
                <div className="pl-4 border-l border-srapi-border/60 ml-5 space-y-1 animate-bloom-soft">
                  {group.items.map(renderItem)}
                </div>
              )}
            </div>
          );
        })}

        {mainItems.slice(5).map(renderItem)}
      </div>
    </aside>
  );
}
