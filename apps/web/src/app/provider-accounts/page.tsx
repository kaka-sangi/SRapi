'use client';

import React, { useState } from 'react';
import {
  FileCode,
  CheckCircle,
  XCircle,
  AlertTriangle,
  Play,
} from 'lucide-react';
import DashboardLayout from '../../components/DashboardLayout';
import { useProviderAccounts } from '@/hooks/queries';
import { useLanguage } from '../../context/LanguageContext';
import { apiService } from '@/lib/api';
import { PageQueryError, PageQueryLoading } from '@/components/layout/page-query-state';
import type { AdminTestResult } from '../../../../../packages/sdk/typescript/src/types.gen';

export default function ProviderAccountsPage() {
  const { language, t } = useLanguage();
  const accountsQuery = useProviderAccounts();
  const accounts = accountsQuery.data ?? [];
  const loading = accountsQuery.isLoading;
  const [testingId, setTestingId] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<{ id: string; result: AdminTestResult } | null>(null);
  const [testError, setTestError] = useState<unknown>(null);

  const handleTestAccount = async (id: string) => {
    setTestingId(id);
    setTestResult(null);
    setTestError(null);

    try {
      const result = await apiService.testProviderAccount(id);
      setTestResult({
        id,
        result,
      });
    } catch (error) {
      setTestError(error);
    } finally {
      setTestingId(null);
    }
  };

  // SRapi v0.1.0 product tone, see docs/PRODUCT_TONE.md.
  const textUpstreamAccounts = language === 'en' ? 'Provider accounts' : '上游账号';
  const textUpstreamDesc = language === 'en'
    ? 'Connected upstream accounts the scheduler can route to. Credentials stay encrypted and write-only.'
    : 'SRapi 可调度的上游账号。凭据始终加密且只写存储。';
  const textVerifyBtn = language === 'en' ? 'Test connection' : '测试连接';
  const textVerifying = language === 'en' ? 'Testing...' : '测试中...';
  const textAdapter = language === 'en' ? 'provider' : '服务商';
  const textNone = language === 'en' ? 'All' : '全部';

  return (
    <DashboardLayout allowedRole="admin">
      <div className="space-y-8 animate-bloom">
        
        {/* Header Block (rounded-2xl) */}
        <div className="bg-srapi-card border border-srapi-border rounded-2xl p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-6 tactile-card">
          <div className="space-y-1">
            <h3 className="font-serif font-medium text-lg tracking-tight">{textUpstreamAccounts}</h3>
            <p className="text-xs text-srapi-text-secondary font-sans leading-relaxed">{textUpstreamDesc}</p>
          </div>
          <button 
            className="px-5 py-3.5 bg-srapi-text-primary text-srapi-bg dark:bg-srapi-text-primary dark:text-srapi-bg hover:bg-transparent hover:text-srapi-text-primary dark:hover:bg-transparent dark:hover:text-srapi-text-primary border border-srapi-text-primary text-xs font-mono tracking-wider uppercase rounded-full transition-all active:scale-[0.96] font-bold flex items-center justify-center gap-1.5 shrink-0 cursor-pointer"
            onClick={() => {
              const docSection = document.getElementById('import-section');
              docSection?.scrollIntoView({ behavior: 'smooth' });
            }}
          >
            <FileCode size={14} />
            {t('specifications')}
          </button>
        </div>

        {/* Upstreams Grid */}
        {testError ? (
          <PageQueryError error={testError} title="Provider account test failed" />
        ) : null}

        {loading ? (
          <PageQueryLoading label={t('resolvingAccounts')} />
        ) : accountsQuery.isError ? (
          <PageQueryError error={accountsQuery.error} onRetry={() => void accountsQuery.refetch()} />
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6 md:gap-8">
            {accounts.map((acc) => {
              const isTesting = testingId === acc.id;
              const hasResult = testResult?.id === acc.id;
              
              return (
                <div key={acc.id} className="bg-srapi-card border border-srapi-border rounded-3xl p-6 space-y-6 flex flex-col justify-between tactile-card">
                  <div className="space-y-4">
                    {/* Provider Vibe */}
                    <div className="flex items-start justify-between">
                      <div className="space-y-1">
                        <span className="text-[9px] font-mono tracking-widest uppercase text-srapi-primary font-bold">
                          {acc.provider_name} {textAdapter}
                        </span>
                        <h4 className="font-serif font-medium text-base tracking-tight text-srapi-text-primary">
                          {acc.name}
                        </h4>
                      </div>

                      <span className={`text-[10px] font-bold border px-2.5 py-0.5 rounded-full ${
                        acc.status === 'active'
                          ? 'border-green-500/20 text-green-700 dark:text-green-500 bg-green-500/10'
                          : acc.status === 'limited'
                          ? 'border-yellow-500/20 text-yellow-600 bg-yellow-500/5'
                          : 'border-srapi-error/20 text-srapi-error bg-srapi-error/5'
                      }`}>
                        {acc.status === 'active'
                          ? (language === 'en' ? 'Active' : '启用')
                          : acc.status === 'limited'
                          ? (language === 'en' ? 'Rate limited' : '被限速')
                          : (language === 'en' ? 'Disabled' : '已停用')}
                      </span>
                    </div>

                    {/* Metadata specs (sandstone cards style) */}
                    <div className="space-y-2.5 text-[10px] font-mono bg-srapi-card-muted/40 p-4 border border-srapi-border rounded-2xl">
                      <div className="flex justify-between">
                        <span className="text-srapi-text-secondary">{t('class')}:</span>
                        <span className="font-semibold text-srapi-text-primary">{acc.runtime_class}</span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-srapi-text-secondary">{t('proxyEndpoint')}:</span>
                        <span className="font-semibold text-srapi-text-primary truncate max-w-[200px]" title={acc.base_url}>
                          {acc.base_url}
                        </span>
                      </div>
                      <div className="flex justify-between">
                        <span className="text-srapi-text-secondary">{t('scopeMaps')}:</span>
                        <span className="font-semibold text-srapi-text-primary truncate max-w-[200px]">
                          {acc.supported_models.join(', ') || textNone}
                        </span>
                      </div>
                    </div>

                    {/* Live dials */}
                    <div className="grid grid-cols-2 gap-6 pt-1">
                      
                      {/* Latency Meter */}
                      <div className="space-y-1.5">
                        <span className="text-[9px] font-mono text-srapi-text-secondary block font-bold tracking-wider">{t('latencyAvg')}</span>
                        <div className="flex items-baseline gap-1">
                          <span className="text-base font-bold text-srapi-text-primary">{acc.latency}</span>
                          <span className="text-[8px] font-mono text-srapi-text-secondary">ms</span>
                        </div>
                        <div className="relative w-full bg-srapi-border h-[1px]">
                          <div 
                            className={`absolute h-3 w-[1px] -top-1 ${acc.latency < 200 ? 'bg-green-700 dark:bg-green-500' : acc.latency < 500 ? 'bg-amber-500' : 'bg-srapi-primary'}`} 
                            style={{ left: `${Math.min(100, (acc.latency / 500) * 100)}%` }}
                          ></div>
                        </div>
                      </div>

                      {/* Quota Meter */}
                      <div className="space-y-1.5">
                        <span className="text-[9px] font-mono text-srapi-text-secondary block font-bold tracking-wider">{t('quotaRemainder')}</span>
                        <div className="flex items-baseline gap-1">
                          <span className="text-base font-bold text-srapi-text-primary">{acc.quota_percentage}</span>
                          <span className="text-[8px] font-mono text-srapi-text-secondary">%</span>
                        </div>
                        <div className="relative w-full bg-srapi-border h-[1px]">
                          <div 
                            className="absolute h-3 w-[1px] bg-srapi-primary -top-1" 
                            style={{ left: `${acc.quota_percentage}%` }}
                          ></div>
                        </div>
                      </div>

                    </div>
                  </div>

                  {/* Test Connection Button Panel */}
                  <div className="pt-4 border-t border-srapi-border/40 flex items-center justify-between gap-3 shrink-0">
                    <button
                      onClick={() => handleTestAccount(acc.id)}
                      disabled={isTesting}
                      className="px-4 py-2 border border-srapi-border hover:bg-srapi-card-muted text-srapi-text-primary hover:text-srapi-primary text-[10px] font-mono font-bold uppercase tracking-wider rounded-full transition-all active:scale-[0.96] flex items-center gap-1.5 cursor-pointer"
                    >
                      <Play size={10} className="text-srapi-primary fill-srapi-primary" />
                      {isTesting ? textVerifying : textVerifyBtn}
                    </button>

                    {hasResult && (
                      <div className={`text-[10px] font-mono flex items-center gap-1 ${
                        testResult.result.ok ? 'text-green-700 dark:text-green-500 animate-pulse' : 'text-srapi-primary'
                      }`}>
                        {testResult.result.ok ? (
                          <>
                            <CheckCircle size={12} />
                            {language === 'en'
                              ? `OK (${testResult.result.latency_ms ?? 0}ms)`
                              : `正常 (${testResult.result.latency_ms ?? 0}ms)`}
                          </>
                        ) : (
                          <>
                            <XCircle size={12} />
                            {testResult.result.message || t('rejected')}
                          </>
                        )}
                      </div>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}

        {/* JSON Schema Specifications Section (rounded-3xl card with tactile feel) */}
        <div id="import-section" className="bg-srapi-card border border-srapi-border rounded-3xl p-8 space-y-6 scroll-mt-24 tactile-card">
          <div className="space-y-1">
            <h3 className="font-serif font-medium text-lg tracking-tight">{t('declarationsTitle')}</h3>
            <p className="text-xs text-srapi-text-secondary leading-relaxed font-sans">
              {t('declarationsDesc')}
            </p>
          </div>

          <div className="space-y-4 font-mono text-[10px]">
            <span className="text-srapi-text-secondary block font-bold uppercase tracking-wider">{t('provisionSchema')}</span>
            <pre className="p-4 bg-srapi-card-muted/50 border border-srapi-border rounded-2xl text-srapi-text-primary overflow-x-auto select-all leading-relaxed">
{`[
  {
    "provider_id": "openai-compatible",
    "name": "openai-gpt-4o-primary",
    "runtime_class": "api_key",
    "credential": {
      "api_key": "sk-proj-..."
    },
    "metadata": {
      "base_url": "https://api.openai.com/v1"
    },
    "status": "active"
  }
]`}
            </pre>

            <div className="p-5 bg-srapi-primary/5 border border-srapi-primary/20 rounded-2xl text-[11px] text-srapi-text-secondary space-y-2 font-sans leading-relaxed">
              <div className="flex items-center gap-2 text-srapi-primary font-bold font-serif">
                <AlertTriangle size={14} />
                {t('writeOnlyGuarantee')}
              </div>
              <p>
                {t('writeOnlyDesc')}
              </p>
            </div>
          </div>
        </div>

      </div>
    </DashboardLayout>
  );
}
