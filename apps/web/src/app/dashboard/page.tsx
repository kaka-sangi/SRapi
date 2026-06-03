import { AppShell } from "@/components/layout/app-shell";
import { GatewayOverview } from "@/components/features/gateway-overview";

export default function DashboardPage() {
  return (
    <AppShell allowedRole="user">
      <GatewayOverview />
    </AppShell>
  );
}
