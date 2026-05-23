'use client';

import React, { useState, useEffect } from 'react';
import { ChevronRight, Copy, Check } from 'lucide-react';
import DashboardLayout from '../../components/DashboardLayout';
import { apiService } from '../../lib/api';
import { MockApiKey, MockUsageLog } from '../../lib/mockData';
import { useLanguage } from '../../context/LanguageContext';

export default function UserDashboard() {
  const { language } = useLanguage();
  
  const [keys, setKeys] = useState<MockApiKey[]>([]);
  const [recentLogs, setRecentLogs] = useState<MockUsageLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [copiedId, setCopiedId] = useState<string | null>(null);

  useEffect(() => {
    async function loadDashboardData() {
      try {
        const [fetchedKeys, fetchedLogs] = await Promise.all([
          apiService.listApiKeys(),
          apiService.listUsageLogs()
        ]);
        setKeys(fetchedKeys.slice(0, 3));
        setRecentLogs(fetchedLogs.slice(0, 5));
      } catch (err) {
        console.error('Failed to load dashboard logs', err);
      } finally {
        setLoading(false);
      }
    }
    loadDashboardData();
  }, []);

  const handleCopyPrefix = (prefix: string, id: string) => {
    navigator.clipboard.writeText(prefix);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 1500);
  };

  const activeKeysCount = keys.filter(k => k.status === 'active').length;
  const totalCost = recentLogs.reduce((acc, log) => acc + log.cost, 0);

  // Localized literals
  const textEpochPerformance = language === 'en' ? 'Current Epoch Performance' : '当前周期运行表现';
  const textAvailableYield = language === 'en' ? 'Available Account Yield' : '可用账户余额';
  const textUsdCredits = language === 'en' ? 'USD CREDITS' : '美元余额';
  const textAccountUsage = language === 'en' ? 'Account usage' : '账户已消耗';
  const textCap = language === 'en' ? 'Cap' : '额度上限';
  const textActiveChannels = language === 'en' ? 'Active Channels' : '活动代理通道';
  const textCredentials = language === 'en' ? 'Credentials' : '项认证凭证';
  const textRoutingStatus = language === 'en' ? 'Routing status:' : '路由分发状态:';
  const textOperational = language === 'en' ? '● OPERATIONAL' : '● 正在运行';
  const textEpochCost = language === 'en' ? 'Epoch Cost Accumulation' : '当前周期累计费用';
  const textRoutedDebits = language === 'en' ? 'Routed Debits' : '路由消耗金额';
  const textSlaAvailability = language === 'en' ? 'SLA availability:' : 'SLA 可用率:';
  const textAuthorizedChannels = language === 'en' ? 'Authorized Channels' : '授权分发通道';
  const textConfigureKeys = language === 'en' ? 'Configure Keys' : '配置通道密钥';
  const textActive = language === 'en' ? '● Active' : '● 启用';
  const textSuspended = language === 'en' ? '■ Suspended' : '■ 禁用';
  const textApiKey = language === 'en' ? 'API KEY' : 'API 密钥';
  const textAllowedModels = language === 'en' ? 'ALLOWED MODELS' : '允许的目标模型';
  const textCreatedDate = language === 'en' ? 'CREATED DATE' : '创建日期';
  const textRecentTransactions = language === 'en' ? 'Recent Ingress Transactions' : '近期网关入口交易明细';
  const textInspectLogs = language === 'en' ? 'Inspect Logs' : '审计流量日志';
  const textTimestamp = language === 'en' ? 'Timestamp' : '时间戳';
  const textTransactionId = language === 'en' ? 'Transaction ID' : '交易请求 ID';
  const textSelectedModel = language === 'en' ? 'Selected Model' : '分发大模型';
  const textSourcePath = language === 'en' ? 'Source Path' : '来源端点';
  const textReroutedTokens = language === 'en' ? 'Rerouted Tokens' : '重定向 Token 数';
  const textYieldCost = language === 'en' ? 'Yield Cost' : '消耗花费';
  const textStatus = language === 'en' ? 'Status' : '状态';
  const textSynthesizing = language === 'en' ? 'Synthesizing developer metrics...' : '正在合成开发者度量指标数据...';

  return (
    <DashboardLayout allowedRole="user">
      {loading ? (
        <div className="py-12 text-center font-mono">
          <div className="w-6 h-6 border-t-2 border-srapi-primary rounded-full animate-spin mx-auto mb-3"></div>
          <p className="text-xs text-srapi-text-secondary">{textSynthesizing}</p>
        </div>
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
                    <span className="font-serif text-3xl font-medium tracking-tight text-srapi-primary">$42.50</span>
                    <span className="text-[9px] font-mono text-srapi-text-secondary uppercase">{textUsdCredits}</span>
                  </div>
                </div>
                
                {/* 1px letterpress Dial indicating Quota Remainder */}
                <div className="w-full">
                  <div className="relative w-full bg-srapi-border h-[1px]">
                    <div className="absolute h-3 w-[1px] bg-srapi-primary -top-1" style={{ left: '42.5%' }}></div>
                  </div>
                  <div className="flex justify-between items-center mt-2.5 text-[9px] font-mono text-srapi-text-secondary">
                    <span>{textAccountUsage}: 42.5%</span>
                    <span>{textCap}: $100.0</span>
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
                  <span>{textSlaAvailability}</span>
                  <span className="text-green-700 dark:text-green-500 font-bold">99.98%</span>
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
                            {log.success ? '200 OK' : '500 ERR'}
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
