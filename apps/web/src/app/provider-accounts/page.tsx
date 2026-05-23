'use client';

import React, { useState, useEffect } from 'react';
import { 
  FileCode, 
  CheckCircle, 
  XCircle, 
  AlertTriangle,
  Play
} from 'lucide-react';
import DashboardLayout from '../../components/DashboardLayout';
import { apiService } from '../../lib/api';
import { MockProviderAccount } from '../../lib/mockData';
import { useLanguage } from '../../context/LanguageContext';

export default function ProviderAccountsPage() {
  const { language, t } = useLanguage();
  const [accounts, setAccounts] = useState<MockProviderAccount[]>([]);
  const [loading, setLoading] = useState(true);
  const [testingId, setTestingId] = useState<string | null>(null);
  const [testResult, setTestResult] = useState<{ id: string; success: boolean; latency: number } | null>(null);

  useEffect(() => {
    async function loadAccounts() {
      setLoading(true);
      try {
        const fetchedAccounts = await apiService.listProviderAccounts();
        setAccounts(fetchedAccounts);
      } catch (err) {
        console.error('Failed to load provider accounts', err);
      } finally {
        setLoading(false);
      }
    }
    loadAccounts();
  }, []);

  const handleTestAccount = async (id: string) => {
    setTestingId(id);
    setTestResult(null);
    
    // Simulate API lease connection test latency
    setTimeout(() => {
      const targetAcc = accounts.find(a => a.id === id);
      const isFailed = targetAcc?.status === 'disabled';
      const responseLatency = targetAcc ? Math.round(targetAcc.latency * 0.9 + Math.random() * 20) : 120;
      
      setTestResult({
        id,
        success: !isFailed,
        latency: responseLatency
      });
      setTestingId(null);
      
      // Clear result after 4 seconds
      setTimeout(() => {
        setTestResult(null);
      }, 4000);
    }, 1200);
  };

  // Localized literals
  const textUpstreamAccounts = language === 'en' ? 'Upstream Provider Accounts' : '上游大模型服务商账户';
  const textUpstreamDesc = language === 'en' 
    ? 'Register secure, write-only credentials and proxy endpoints scoped for upstream LLM foundation providers.' 
    : '配置并注册用于安全调用上游 LLM 基础大模型提供商的只写凭证与代理路由端点。';
  const textVerifyBtn = language === 'en' ? 'Verify Link' : '校验链路';
  const textVerifying = language === 'en' ? 'Verifying...' : '正在校验...';
  const textAdapter = language === 'en' ? 'adapter' : '适配器';
  const textNone = language === 'en' ? 'None' : '无限制';

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
        {loading ? (
          <div className="py-12 text-center font-mono">
            <div className="w-6 h-6 border-t-2 border-srapi-primary rounded-full animate-spin mx-auto mb-3"></div>
            <p className="text-xs text-srapi-text-secondary">{t('resolvingAccounts')}</p>
          </div>
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
                        {acc.status === 'active' ? (language === 'en' ? 'ACTIVE' : '启用') : acc.status === 'limited' ? (language === 'en' ? 'LIMITED' : '流量受限') : (language === 'en' ? 'DISABLED' : '暂停')}
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
                        testResult.success ? 'text-green-700 dark:text-green-500 animate-pulse' : 'text-srapi-primary'
                      }`}>
                        {testResult.success ? (
                          <>
                            <CheckCircle size={12} />
                            {language === 'en' ? `Verified (${testResult.latency}ms)` : `校验成功 (${testResult.latency}ms)`}
                          </>
                        ) : (
                          <>
                            <XCircle size={12} />
                            {t('rejected')}
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
