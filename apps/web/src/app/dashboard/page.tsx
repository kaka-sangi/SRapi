'use client';

import React, { useState } from 'react';
import { ChevronRight, Copy, Check, KeyRound, Inbox } from 'lucide-react';
import { AppShell } from '@/components/layout';
import { EmptyState, Skeleton } from '@/components/ui';
import { useApiKeys, useLiveCurrentUser, useUsageLogs } from '@/hooks/queries';
import { useLanguage } from '../../context/LanguageContext';
import { PageQueryError } from '@/components/layout/page-query-state';

export default function UserDashboard() {
  const { language } = useLanguage();
  const currentUserQuery = useLiveCurrentUser();
  const apiKeysQuery = useApiKeys();
  const usageQuery = useUsageLogs();
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const loading = currentUserQuery.isLoading || apiKeysQuery.isLoading || usageQuery.isLoading;
  const user = currentUserQuery.data;
  const keys = (apiKeysQuery.data ?? []).slice(0, 3);
  const recentLogs = (usageQuery.data ?? []).slice(0, 5);

  const handleCopyPrefix = (prefix: string, id: string) => {
    navigator.clipboard.writeText(prefix);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 1500);
  };

  const activeKeysCount = keys.filter((k) => k.status === 'active').length;
  const totalCost = recentLogs.reduce((acc, log) => acc + log.cost, 0);
  const successfulRequests = recentLogs.filter((log) => log.success).length;
  const successRate = recentLogs.length > 0 ? (successfulRequests / recentLogs.length) * 100 : null;
  const totalTokens = recentLogs.reduce((acc, log) => acc + log.total_tokens, 0);
  const balance = user ? Number.parseFloat(user.balance) : 0;
  const formattedBalance = Number.isFinite(balance)
    ? new Intl.NumberFormat(language === 'en' ? 'en-US' : 'zh-CN', {
        style: 'currency',
        currency: user?.currency || 'USD',
        maximumFractionDigits: 4,
      }).format(balance)
    : `${user?.balance ?? '0'} ${user?.currency ?? 'USD'}`;

  // SRapi v0.1.0 product tone, see docs/PRODUCT_TONE.md.
  const textEpochPerformance = language === 'en' ? 'This period' : '本周期';
  const textAvailableYield = language === 'en' ? 'Available balance' : '可用余额';
  const textUsdCredits = language === 'en' ? 'USD' : '美元';
  const textAccountUsage = language === 'en' ? 'Used' : '已用';
  const textRpmLimit = language === 'en' ? 'RPM limit' : 'RPM 限制';
  const textActiveChannels = language === 'en' ? 'Active API keys' : '启用中的密钥';
  const textCredentials = language === 'en' ? 'keys' : '个';
  const textRoutingStatus = language === 'en' ? 'Gateway:' : '网关：';
  const textOperational = language === 'en' ? 'Live API' : '实时 API';
  const textEpochCost = language === 'en' ? 'Spend this period' : '本周期花费';
  const textRoutedDebits = language === 'en' ? 'spent' : '已花费';
  const textSuccessRate = language === 'en' ? 'Success rate:' : '成功率：';
  const textNotProvided = language === 'en' ? 'Not provided' : '未提供';
  const textAuthorizedChannels = language === 'en' ? 'Your API keys' : '你的 API 密钥';
  const textConfigureKeys = language === 'en' ? 'Manage keys' : '管理密钥';
  const textActive = language === 'en' ? '● Active' : '● 启用';
  const textSuspended = language === 'en' ? '■ Disabled' : '■ 已停用';
  const textApiKey = language === 'en' ? 'API KEY' : 'API 密钥';
  const textAllowedModels = language === 'en' ? 'ALLOWED MODELS' : '可用模型';
  const textCreatedDate = language === 'en' ? 'CREATED' : '创建时间';
  const textRecentTransactions = language === 'en' ? 'Recent requests' : '最近的请求';
  const textInspectLogs = language === 'en' ? 'View all' : '查看全部';
  const textTimestamp = language === 'en' ? 'Time' : '时间';
  const textTransactionId = language === 'en' ? 'Request ID' : '请求 ID';
  const textSelectedModel = language === 'en' ? 'Model' : '模型';
  const textSourcePath = language === 'en' ? 'Endpoint' : '接入点';
  const textReroutedTokens = language === 'en' ? 'Tokens' : 'Token';
  const textYieldCost = language === 'en' ? 'Cost' : '花费';
  const textStatus = language === 'en' ? 'Status' : '状态';
  const textNoKeys = language === 'en' ? 'No API keys yet' : '还没有 API 密钥';
  const textNoKeysDesc =
    language === 'en'
      ? 'Create your first key to start routing requests through the gateway.'
      : '创建第一个密钥即可开始通过网关路由请求。';
  const textNoLogs = language === 'en' ? 'No requests yet' : '暂无请求记录';
  const textNoLogsDesc =
    language === 'en'
      ? 'Usage will appear here once your keys start serving traffic.'
      : '密钥开始承载流量后，用量会显示在这里。';

  return (
    <AppShell allowedRole="user">
      {loading ? (
        <DashboardSkeleton />
      ) : currentUserQuery.isError || apiKeysQuery.isError || usageQuery.isError ? (
        <PageQueryError
          error={currentUserQuery.error || apiKeysQuery.error || usageQuery.error}
          onRetry={() => {
            void currentUserQuery.refetch();
            void apiKeysQuery.refetch();
            void usageQuery.refetch();
          }}
        />
      ) : (
        <div className="space-y-12">
          {/* Section: Performance Epoch Indicators */}
          <div className="space-y-5">
            <div className="font-serif text-sm italic text-srapi-text-secondary">{textEpochPerformance}</div>
            <div className="grid grid-cols-1 gap-6 md:grid-cols-3 md:gap-8">
              {/* Card 1: Balance with a notch dial */}
              <div className="surface stat-accent flex h-[155px] flex-col justify-between rounded-2xl p-6">
                <div>
                  <div className="mb-2 font-mono text-2xs uppercase tracking-wider text-srapi-text-secondary">
                    {textAvailableYield}
                  </div>
                  <div className="flex items-baseline space-x-2">
                    <span className="font-serif text-3xl font-medium tracking-tight text-srapi-primary">{formattedBalance}</span>
                    <span className="font-mono text-2xs uppercase text-srapi-text-secondary">{user?.currency || textUsdCredits}</span>
                  </div>
                </div>

                <div className="w-full">
                  <div className="notch-rail">
                    <div className="notch-pin bg-srapi-primary" style={{ left: '0%' }} />
                  </div>
                  <div className="mt-2.5 flex items-center justify-between font-mono text-2xs text-srapi-text-secondary">
                    <span>{textAccountUsage}: {totalTokens.toLocaleString()} tokens</span>
                    <span>{textRpmLimit}: {user?.rpm_limit ?? textNotProvided}</span>
                  </div>
                </div>
              </div>

              {/* Card 2: Active Keys count */}
              <div className="surface stat-accent flex h-[155px] flex-col justify-between rounded-2xl p-6">
                <div>
                  <div className="mb-2 font-mono text-2xs uppercase tracking-wider text-srapi-text-secondary">
                    {textActiveChannels}
                  </div>
                  <div className="flex items-baseline space-x-2">
                    <span className="font-serif text-3xl font-medium tracking-tight text-srapi-text-primary">{activeKeysCount}</span>
                    <span className="font-mono text-xs font-medium text-srapi-text-secondary">{textCredentials}</span>
                  </div>
                </div>
                <div className="flex items-center justify-between border-t border-srapi-border/40 pt-2.5 font-mono text-2xs text-srapi-text-secondary">
                  <span>{textRoutingStatus}</span>
                  <span className="font-bold text-srapi-success">{textOperational}</span>
                </div>
              </div>

              {/* Card 3: Costs Accrued */}
              <div className="surface stat-accent flex h-[155px] flex-col justify-between rounded-2xl p-6">
                <div>
                  <div className="mb-2 font-mono text-2xs uppercase tracking-wider text-srapi-text-secondary">
                    {textEpochCost}
                  </div>
                  <div className="flex items-baseline space-x-2">
                    <span className="font-serif text-3xl font-medium tracking-tight text-srapi-text-primary">${totalCost.toFixed(4)}</span>
                    <span className="font-mono text-xs font-medium text-srapi-text-secondary">{textRoutedDebits}</span>
                  </div>
                </div>
                <div className="flex items-center justify-between border-t border-srapi-border/40 pt-2.5 font-mono text-2xs text-srapi-text-secondary">
                  <span>{textSuccessRate}</span>
                  <span className="font-bold text-srapi-success">
                    {successRate === null ? textNotProvided : `${successRate.toFixed(1)}%`}
                  </span>
                </div>
              </div>
            </div>
          </div>

          {/* Section: Access Channels list */}
          <div className="space-y-5">
            <div className="flex items-baseline justify-between border-b border-srapi-border pb-2">
              <span className="font-serif text-lg italic text-srapi-text-primary">{textAuthorizedChannels}</span>
              <a
                href="/api-keys"
                className="flex items-center gap-0.5 font-mono text-2xs uppercase text-srapi-primary hover:underline"
              >
                {textConfigureKeys}
                <ChevronRight size={10} />
              </a>
            </div>

            {keys.length === 0 ? (
              <EmptyState
                icon={<KeyRound size={18} />}
                title={textNoKeys}
                description={textNoKeysDesc}
                action={
                  <a
                    href="/api-keys"
                    className="flex items-center gap-1 font-mono text-2xs font-bold uppercase tracking-wider text-srapi-primary hover:underline"
                  >
                    {textConfigureKeys}
                    <ChevronRight size={12} />
                  </a>
                }
              />
            ) : (
              <div className="space-y-6">
                {keys.map((k) => (
                  <div key={k.id} className="surface flex flex-col justify-between gap-4 rounded-2xl p-6 sm:flex-row sm:items-center">
                    <div className="space-y-1.5">
                      <div className="flex items-center space-x-3">
                        <span className={`font-mono text-2xs ${k.status === 'active' ? 'text-srapi-success' : 'text-srapi-text-secondary'}`}>
                          {k.status === 'active' ? textActive : textSuspended}
                        </span>
                        <h3 className="font-serif text-base font-medium">{k.name}</h3>
                        <span className="rounded-full border border-srapi-border bg-srapi-card-muted px-2.5 py-0.5 font-mono text-2xs font-bold uppercase tracking-wider text-srapi-text-secondary">
                          {textApiKey}
                        </span>
                      </div>

                      <div className="flex items-center space-x-2">
                        <code className="rounded border border-srapi-border bg-srapi-card-muted px-2 py-0.5 font-mono text-2xs text-srapi-text-secondary">
                          {k.prefix}
                        </code>
                        <button
                          onClick={() => handleCopyPrefix(k.prefix, k.id)}
                          className="cursor-pointer rounded border border-transparent p-1 text-srapi-text-secondary hover:border-srapi-border hover:bg-srapi-card-muted"
                          title="Copy Key Prefix"
                        >
                          {copiedId === k.id ? <Check size={12} className="text-srapi-success" /> : <Copy size={12} />}
                        </button>
                      </div>
                    </div>

                    <div className="flex flex-row items-center gap-6 font-mono text-2xs text-srapi-text-secondary">
                      <div className="text-left sm:text-right">
                        <span className="block font-bold tracking-wider opacity-60">{textAllowedModels}</span>
                        <span className="font-medium text-srapi-text-primary">{k.allowed_models.join(', ') || 'All'}</span>
                      </div>
                      <div className="flex h-8 flex-col justify-center border-l border-srapi-border/40 pl-6 text-left sm:text-right">
                        <span className="block font-bold tracking-wider opacity-60">{textCreatedDate}</span>
                        <span className="font-medium text-srapi-text-primary">{new Date(k.created_at).toLocaleDateString()}</span>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Section: Invocations history table */}
          <div className="space-y-5">
            <div className="flex items-baseline justify-between border-b border-srapi-border pb-2">
              <span className="font-serif text-lg italic text-srapi-text-primary">{textRecentTransactions}</span>
              <a
                href="/usage"
                className="flex items-center gap-0.5 font-mono text-2xs uppercase text-srapi-primary hover:underline"
              >
                {textInspectLogs}
                <ChevronRight size={10} />
              </a>
            </div>

            {recentLogs.length === 0 ? (
              <EmptyState icon={<Inbox size={18} />} title={textNoLogs} description={textNoLogsDesc} />
            ) : (
              <div className="surface overflow-hidden rounded-2xl">
                <div className="overflow-x-auto scrollbar-none">
                  <table className="w-full min-w-[650px] border-collapse text-left text-xs">
                    <thead>
                      <tr className="border-b border-srapi-border bg-srapi-card-muted/60 font-mono text-2xs uppercase tracking-wider text-srapi-text-secondary">
                        <th className="px-6 py-4 font-medium">{textTimestamp}</th>
                        <th className="px-6 py-4 font-medium">{textTransactionId}</th>
                        <th className="px-6 py-4 font-medium">{textSelectedModel}</th>
                        <th className="px-6 py-4 font-medium">{textSourcePath}</th>
                        <th className="px-6 py-4 text-right font-medium">{textReroutedTokens}</th>
                        <th className="px-6 py-4 text-right font-medium">{textYieldCost}</th>
                        <th className="px-6 py-4 text-right font-medium">{textStatus}</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-srapi-border/60 font-mono text-2xs">
                      {recentLogs.map((log) => (
                        <tr key={log.request_id} className="hover:bg-srapi-card-muted/20">
                          <td className="px-6 py-4 text-srapi-text-secondary">{new Date(log.created_at).toLocaleTimeString()}</td>
                          <td className="select-all px-6 py-4 font-medium text-srapi-text-primary">{log.request_id.slice(0, 15)}...</td>
                          <td className="px-6 py-4">
                            <span className="rounded border border-srapi-border bg-srapi-card-muted px-2.5 py-0.5 text-2xs font-bold text-srapi-text-primary">
                              {log.model}
                            </span>
                          </td>
                          <td className="px-6 py-4 text-srapi-text-secondary">{log.source_endpoint}</td>
                          <td className="px-6 py-4 text-right font-medium text-srapi-text-primary">{log.total_tokens.toLocaleString()}</td>
                          <td className="px-6 py-4 text-right font-bold text-srapi-primary">${log.cost.toFixed(5)}</td>
                          <td className="px-6 py-4 text-right">
                            <span
                              className={`rounded-full border px-2.5 py-0.5 text-2xs font-bold ${
                                log.success
                                  ? 'border-srapi-success/20 bg-srapi-success/10 text-srapi-success'
                                  : 'border-srapi-error/20 bg-srapi-error/5 text-srapi-error'
                              }`}
                            >
                              {log.success ? (language === 'en' ? 'Success' : '成功') : (language === 'en' ? 'Failed' : '失败')}
                            </span>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </AppShell>
  );
}

/** Shimmer placeholder mirroring the dashboard's stat row + list while data loads. */
function DashboardSkeleton() {
  return (
    <div className="space-y-12">
      <div className="space-y-5">
        <Skeleton className="h-4 w-24" />
        <div className="grid grid-cols-1 gap-6 md:grid-cols-3 md:gap-8">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="surface flex h-[155px] flex-col justify-between rounded-2xl p-6">
              <div className="space-y-3">
                <Skeleton className="h-3 w-20" />
                <Skeleton className="h-8 w-32" />
              </div>
              <Skeleton className="h-3 w-full" />
            </div>
          ))}
        </div>
      </div>
      <div className="space-y-5">
        <Skeleton className="h-5 w-40" />
        {Array.from({ length: 3 }).map((_, i) => (
          <Skeleton key={i} className="h-24 w-full rounded-2xl" />
        ))}
      </div>
    </div>
  );
}
