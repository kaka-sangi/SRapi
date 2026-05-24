'use client';

import React, { useState } from 'react';
import { ChevronRight, Copy, Check } from 'lucide-react';
import DashboardLayout from '../../components/DashboardLayout';
import { useApiKeys, useLiveCurrentUser, useUsageLogs } from '@/hooks/queries';
import { useLanguage } from '../../context/LanguageContext';
import { PageQueryError, PageQueryLoading } from '@/components/layout/page-query-state';

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
  const textSynthesizing = language === 'en' ? 'Loading...' : '加载中...';

  return (
    <DashboardLayout allowedRole="user">
      {loading ? (
        <PageQueryLoading label={textSynthesizing} />
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
            <div className="grid grid-cols-1 md:grid-cols-3 gap-6 md:gap-8">
              
              {/* Card 1: Balance with a real industrial Dial */}
              <div className="bg-srapi-card border border-srapi-border rounded-2xl p-6 tactile-card flex flex-col justify-between h-[155px]">
                <div>
                  <div className="text-[10px] font-mono tracking-wider uppercase text-srapi-text-secondary mb-2">{textAvailableYield}</div>
                  <div className="flex items-baseline space-x-2">
                    <span className="font-serif text-3xl font-medium tracking-tight text-srapi-primary">{formattedBalance}</span>
                    <span className="text-[9px] font-mono text-srapi-text-secondary uppercase">{user?.currency || textUsdCredits}</span>
                  </div>
                </div>
                
                <div className="w-full">
                  <div className="relative w-full bg-srapi-border h-[1px]">
                    <div className="absolute h-3 w-[1px] bg-srapi-primary -top-1" style={{ left: '0%' }}></div>
                  </div>
                  <div className="flex justify-between items-center mt-2.5 text-[9px] font-mono text-srapi-text-secondary">
                    <span>{textAccountUsage}: {totalTokens.toLocaleString()} tokens</span>
                    <span>{textRpmLimit}: {user?.rpm_limit ?? textNotProvided}</span>
                  </div>
                </div>
              </div>

              {/* Card 2: Active Keys count */}
              <div className="bg-srapi-card border border-srapi-border rounded-2xl p-6 tactile-card flex flex-col justify-between h-[155px]">
                <div>
                  <div className="text-[10px] font-mono tracking-wider uppercase text-srapi-text-secondary mb-2">{textActiveChannels}</div>
                  <div className="flex items-baseline space-x-2">
                    <span className="font-serif text-3xl font-medium tracking-tight text-srapi-text-primary">{activeKeysCount}</span>
                    <span className="text-xs font-mono font-medium text-srapi-text-secondary">{textCredentials}</span>
                  </div>
                </div>
                <div className="text-[10px] font-mono text-srapi-text-secondary border-t border-srapi-border/40 pt-2.5 flex items-center justify-between">
                  <span>{textRoutingStatus}</span>
                  <span className="text-green-700 dark:text-green-500 font-bold">{textOperational}</span>
                </div>
              </div>

              {/* Card 3: Costs Accrued */}
              <div className="bg-srapi-card border border-srapi-border rounded-2xl p-6 tactile-card flex flex-col justify-between h-[155px]">
                <div>
                  <div className="text-[10px] font-mono tracking-wider uppercase text-srapi-text-secondary mb-2">{textEpochCost}</div>
                  <div className="flex items-baseline space-x-2">
                    <span className="font-serif text-3xl font-medium tracking-tight text-srapi-text-primary">${totalCost.toFixed(4)}</span>
                    <span className="text-xs font-mono font-medium text-srapi-text-secondary">{textRoutedDebits}</span>
                  </div>
                </div>
                <div className="text-[10px] font-mono text-srapi-text-secondary border-t border-srapi-border/40 pt-2.5 flex items-center justify-between">
                  <span>{textSuccessRate}</span>
                  <span className="text-green-700 dark:text-green-500 font-bold">
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
                className="text-[10px] font-mono uppercase text-srapi-primary hover:underline flex items-center gap-0.5"
              >
                {textConfigureKeys}
                <ChevronRight size={10} />
              </a>
            </div>

            <div className="space-y-6">
              {keys.map((k) => (
                <div key={k.id} className="bg-srapi-card border border-srapi-border rounded-2xl p-6 tactile-card flex flex-col sm:flex-row sm:items-center justify-between gap-4">
                  <div className="space-y-1.5">
                    <div className="flex items-center space-x-3">
                      <span className={`text-[10px] font-mono ${k.status === 'active' ? 'text-green-700 dark:text-green-500' : 'text-srapi-text-secondary'}`}>
                        {k.status === 'active' ? textActive : textSuspended}
                      </span>
                      <h3 className="font-serif text-base font-medium">{k.name}</h3>
                      <span className="text-[9px] font-mono tracking-wider uppercase bg-srapi-card-muted border border-srapi-border px-2.5 py-0.5 rounded-full text-srapi-text-secondary font-bold">
                        {textApiKey}
                      </span>
                    </div>

                    <div className="flex items-center space-x-2">
                      <code className="text-[11px] font-mono bg-srapi-card-muted border border-srapi-border px-2 py-0.5 rounded text-srapi-text-secondary">
                        {k.prefix}
                      </code>
                      <button 
                        onClick={() => handleCopyPrefix(k.prefix, k.id)}
                        className="p-1 hover:bg-srapi-card-muted rounded border border-transparent hover:border-srapi-border text-srapi-text-secondary cursor-pointer"
                        title="Copy Key Prefix"
                      >
                        {copiedId === k.id ? <Check size={12} className="text-green-700" /> : <Copy size={12} />}
                      </button>
                    </div>
                  </div>

                  <div className="flex flex-row items-center gap-6 text-[11px] font-mono text-srapi-text-secondary">
                    <div className="text-left sm:text-right">
                      <span className="opacity-60 block text-[9px] font-bold tracking-wider">{textAllowedModels}</span>
                      <span className="text-srapi-text-primary font-medium">{k.allowed_models.join(', ') || 'All'}</span>
                    </div>
                    <div className="text-left sm:text-right border-l border-srapi-border/40 pl-6 h-8 flex flex-col justify-center">
                      <span className="opacity-60 block text-[9px] font-bold tracking-wider">{textCreatedDate}</span>
                      <span className="text-srapi-text-primary font-medium">{new Date(k.created_at).toLocaleDateString()}</span>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Section: Invocations history table */}
          <div className="space-y-5">
            <div className="flex items-baseline justify-between border-b border-srapi-border pb-2">
              <span className="font-serif text-lg italic text-srapi-text-primary">{textRecentTransactions}</span>
              <a 
                href="/usage" 
                className="text-[10px] font-mono uppercase text-srapi-primary hover:underline flex items-center gap-0.5"
              >
                {textInspectLogs}
                <ChevronRight size={10} />
              </a>
            </div>

            <div className="border border-srapi-border rounded-2xl overflow-hidden shadow-[0_4px_20px_rgba(25,25,25,0.01)] dark:shadow-none bg-srapi-card">
              <div className="overflow-x-auto scrollbar-none">
                <table className="w-full text-left border-collapse text-xs min-w-[650px]">
                  <thead>
                    <tr className="border-b border-srapi-border bg-srapi-card-muted/60 text-[10px] font-mono tracking-wider text-srapi-text-secondary uppercase">
                      <th className="py-4 px-6 font-medium">{textTimestamp}</th>
                      <th className="py-4 px-6 font-medium">{textTransactionId}</th>
                      <th className="py-4 px-6 font-medium">{textSelectedModel}</th>
                      <th className="py-4 px-6 font-medium">{textSourcePath}</th>
                      <th className="py-4 px-6 font-medium text-right">{textReroutedTokens}</th>
                      <th className="py-4 px-6 font-medium text-right">{textYieldCost}</th>
                      <th className="py-4 px-6 font-medium text-right">{textStatus}</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-srapi-border/60 font-mono text-[11px]">
                    {recentLogs.map((log) => (
                      <tr key={log.request_id} className="hover:bg-srapi-card-muted/20">
                        <td className="py-4.5 px-6 text-srapi-text-secondary">{new Date(log.created_at).toLocaleTimeString()}</td>
                        <td className="py-4.5 px-6 font-medium text-srapi-text-primary select-all">{log.request_id.slice(0, 15)}...</td>
                        <td className="py-4.5 px-6">
                          <span className="px-2.5 py-0.5 bg-srapi-card-muted border border-srapi-border rounded text-[9px] font-bold text-srapi-text-primary">
                            {log.model}
                          </span>
                        </td>
                        <td className="py-4.5 px-6 text-srapi-text-secondary">{log.source_endpoint}</td>
                        <td className="py-4.5 px-6 text-right font-medium text-srapi-text-primary">{log.total_tokens.toLocaleString()}</td>
                        <td className="py-4.5 px-6 text-right text-srapi-primary font-bold">${log.cost.toFixed(5)}</td>
                        <td className="py-4.5 px-6 text-right">
                          <span className={`text-[10px] font-bold border px-2.5 py-0.5 rounded-full ${
                            log.success 
                              ? 'text-green-700 dark:text-green-500 bg-green-500/10 border-green-500/20' 
                              : 'text-srapi-error bg-srapi-error/5 border-srapi-error/20'
                          }`}>
                            {log.success ? (language === 'en' ? 'Success' : '成功') : (language === 'en' ? 'Failed' : '失败')}
                          </span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          </div>

        </div>
      )}
    </DashboardLayout>
  );
}
