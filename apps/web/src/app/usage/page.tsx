'use client';

import React, { useMemo, useState } from 'react';
import {
  Activity,
  Search,
  Filter,
  TrendingUp,
  TrendingDown,
  AlertTriangle,
  Calculator,
} from 'lucide-react';
import DashboardLayout from '../../components/DashboardLayout';
import { apiService } from '../../lib/api';
import { useUsageLogs } from '@/hooks/queries';
import { useLanguage } from '../../context/LanguageContext';
import { PageQueryError, PageQueryLoading } from '@/components/layout/page-query-state';

export default function UsagePage() {
  const { language, t } = useLanguage();
  const logsQuery = useUsageLogs();
  const logs = useMemo(() => logsQuery.data ?? [], [logsQuery.data]);
  const loading = logsQuery.isLoading;
  const [user] = useState(() => apiService.getCurrentUser());

  // Filters State
  const [selectedModel, setSelectedModel] = useState('all');
  const [statusFilter, setStatusFilter] = useState('all');
  const [searchQuery, setSearchQuery] = useState('');

  const filteredLogs = useMemo(() => {
    let result = logs;

    // Filter by model
    if (selectedModel !== 'all') {
      result = result.filter(log => log.model === selectedModel);
    }

    // Filter by status
    if (statusFilter === 'success') {
      result = result.filter(log => log.success === true);
    } else if (statusFilter === 'failure') {
      result = result.filter(log => log.success === false);
    }

    // Filter by search query
    if (searchQuery.trim() !== '') {
      const q = searchQuery.toLowerCase();
      result = result.filter(log => 
        log.request_id.toLowerCase().includes(q) || 
        log.source_endpoint.toLowerCase().includes(q)
      );
    }

    return result;
  }, [selectedModel, statusFilter, searchQuery, logs]);

  // Aggregate stats
  const totalRequests = filteredLogs.length;
  const successfulRequests = filteredLogs.filter(log => log.success).length;
  const successRate = totalRequests > 0 ? (successfulRequests / totalRequests) * 100 : 100;
  const totalTokens = filteredLogs.reduce((acc, log) => acc + log.total_tokens, 0);
  const totalCost = filteredLogs.reduce((acc, log) => acc + log.cost, 0);

  // SRapi v0.1.0 product tone, see docs/PRODUCT_TONE.md.
  const textSuccessRateUpper = language === 'en' ? 'SUCCESS RATE' : '成功率';

  return (
    <DashboardLayout>
      <div className="space-y-8 animate-bloom">
        
        {/* Usage Overview Banner */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-6">
          
          {/* Card 1: Requests */}
          <div className="bg-srapi-card border border-srapi-border rounded-2xl p-6 tactile-card space-y-3">
            <div className="flex items-center justify-between text-srapi-text-secondary text-[10px] font-mono uppercase tracking-wider font-bold">
              <span>{t('auditedTraffic')}</span>
              <Activity size={14} className="text-srapi-primary" />
            </div>
            <div className="space-y-1">
              <div className="font-serif text-3xl font-medium tracking-tight text-srapi-text-primary">{totalRequests}</div>
              <div className="text-[9px] font-mono text-srapi-text-secondary uppercase">{t('invocationsEvaluated')}</div>
            </div>
          </div>

          {/* Card 2: Success Rate */}
          <div className="bg-srapi-card border border-srapi-border rounded-2xl p-6 tactile-card space-y-3">
            <div className="flex items-center justify-between text-srapi-text-secondary text-[10px] font-mono uppercase tracking-wider font-bold">
              <span>{t('routerSla')}</span>
              {successRate >= 99 ? (
                <TrendingUp size={14} className="text-srapi-success" />
              ) : (
                <TrendingDown size={14} className="text-srapi-primary" />
              )}
            </div>
            <div className="space-y-1">
              <div className={`font-serif text-3xl font-medium tracking-tight ${successRate >= 99 ? 'text-srapi-success' : 'text-srapi-primary'}`}>
                {successRate.toFixed(2)}%
              </div>
              <div className="text-[9px] font-mono text-srapi-text-secondary uppercase">{textSuccessRateUpper}</div>
            </div>
          </div>

          {/* Card 3: Total Tokens */}
          <div className="bg-srapi-card border border-srapi-border rounded-2xl p-6 tactile-card space-y-3">
            <div className="flex items-center justify-between text-srapi-text-secondary text-[10px] font-mono uppercase tracking-wider font-bold">
              <span>{t('payloadRouted')}</span>
              <Calculator size={14} className="text-srapi-primary" />
            </div>
            <div className="space-y-1">
              <div className="font-serif text-3xl font-medium tracking-tight text-srapi-text-primary">
                {totalTokens >= 1000 ? `${(totalTokens / 1000).toFixed(1)}k` : totalTokens}
              </div>
              <div className="text-[9px] font-mono text-srapi-text-secondary uppercase">{t('totalTokens')}</div>
            </div>
          </div>

          {/* Card 4: Accrued Cost */}
          <div className="bg-srapi-card border border-srapi-border rounded-2xl p-6 tactile-card space-y-3">
            <div className="flex items-center justify-between text-srapi-text-secondary text-[10px] font-mono uppercase tracking-wider font-bold">
              <span>{t('financialCost')}</span>
              <span className="text-xs font-bold text-srapi-primary">USD</span>
            </div>
            <div className="space-y-1">
              <div className="font-serif text-3xl font-medium tracking-tight text-srapi-primary">
                ${totalCost.toFixed(5)}
              </div>
              <div className="text-[9px] font-mono text-srapi-text-secondary uppercase">{t('estimatedDebit')}</div>
            </div>
          </div>

        </div>

        {/* Filter Toolbar */}
        <div className="bg-srapi-card border border-srapi-border rounded-2xl p-4 flex flex-col md:flex-row md:items-center justify-between gap-4 tactile-card">
          
          {/* Search bar */}
          <div className="flex-1 relative">
            <Search className="absolute left-4 top-4 text-srapi-text-secondary" size={14} />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder={t('searchPlaceholder')}
              className="w-full pl-10 pr-4 py-3 border border-srapi-border bg-srapi-bg text-srapi-text-primary rounded-xl text-xs outline-none focus:border-srapi-primary font-mono placeholder:text-srapi-text-secondary/40"
            />
          </div>

          {/* Filter dropdowns */}
          <div className="flex flex-wrap items-center gap-3">
            <div className="flex items-center gap-1.5 text-xs text-srapi-text-secondary font-mono">
              <Filter size={12} />
              <span className="font-bold">{t('filtersLabel')}</span>
            </div>

            <select
              value={selectedModel}
              onChange={(e) => setSelectedModel(e.target.value)}
              className="px-3.5 py-3 border border-srapi-border bg-srapi-bg text-srapi-text-primary rounded-xl text-xs outline-none focus:border-srapi-primary font-mono cursor-pointer"
            >
              <option value="all">{t('allModelScopes')}</option>
              <option value="gpt-4o-mini">gpt-4o-mini</option>
              <option value="claude-3-5-sonnet">claude-3-5-sonnet</option>
              <option value="gemini-1.5-flash">gemini-1.5-flash</option>
            </select>

            <select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value)}
              className="px-3.5 py-3 border border-srapi-border bg-srapi-bg text-srapi-text-primary rounded-xl text-xs outline-none focus:border-srapi-primary font-mono cursor-pointer"
            >
              <option value="all">{t('allResponseStates')}</option>
              <option value="success">{t('successOnly')}</option>
              <option value="failure">{t('errorsOnly')}</option>
            </select>
          </div>

        </div>

        {/* Logs Table Card (rounded-3xl card with tactile feel) */}
        <div className="bg-srapi-card border border-srapi-border rounded-3xl p-6 space-y-5 tactile-card">
          <div className="flex items-center justify-between border-b border-srapi-border pb-3">
            <h4 className="font-serif text-lg italic text-srapi-text-primary">
              {user?.role === 'admin' ? t('globalLogs') : t('personalLogs')}
            </h4>
            <span className="text-[10px] font-mono text-srapi-text-secondary bg-srapi-card-muted/50 border border-srapi-border px-2.5 py-1 rounded-lg font-bold">
              {t('showingEvents', { filtered: filteredLogs.length, total: logs.length })}
            </span>
          </div>

          {loading ? (
            <PageQueryLoading label={t('fetchingEvidence')} />
          ) : logsQuery.isError ? (
            <PageQueryError error={logsQuery.error} onRetry={() => void logsQuery.refetch()} />
          ) : filteredLogs.length === 0 ? (
            <div className="py-16 border border-dashed border-srapi-border rounded-2xl text-center space-y-3.5">
              <AlertTriangle className="mx-auto text-srapi-text-secondary opacity-40" size={28} />
              <p className="text-xs font-bold text-srapi-text-primary font-serif">{t('noTraffic')}</p>
              <p className="text-[10px] text-srapi-text-secondary font-mono">{t('noTrafficDesc')}</p>
            </div>
          ) : (
            <div className="overflow-x-auto scrollbar-none border border-srapi-border rounded-2xl shadow-[0_4px_20px_rgba(25,25,25,0.015)] dark:shadow-none bg-srapi-card">
              <table className="w-full text-left border-collapse text-xs min-w-[700px]">
                <thead>
                  <tr className="bg-srapi-card-muted/65 border-b border-srapi-border font-mono text-srapi-text-secondary text-[10px] uppercase tracking-wider">
                    <th className="py-4 px-6 font-medium">{t('timestamp')}</th>
                    <th className="py-4 px-6 font-medium">{t('requestId')}</th>
                    <th className="py-4 px-6 font-medium">Model</th>
                    <th className="py-4 px-6 font-medium">{t('sourcePath')}</th>
                    <th className="py-4 px-6 font-medium">{t('resultStatus')}</th>
                    <th className="py-4 px-6 font-medium text-right">{t('reroutedTokens')}</th>
                    <th className="py-4 px-6 font-medium text-right font-bold">{t('transactCost')}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-srapi-border font-mono text-[11px]">
                  {filteredLogs.map((log) => (
                    <tr key={log.request_id} className="hover:bg-srapi-card-muted/20 transition-colors">
                      <td className="py-4.5 px-6 whitespace-nowrap text-srapi-text-secondary">
                        {new Date(log.created_at).toLocaleString()}
                      </td>
                      <td className="py-4.5 px-6 whitespace-nowrap text-srapi-text-primary font-semibold select-all">
                        {log.request_id}
                      </td>
                      <td className="py-4.5 px-6 whitespace-nowrap">
                        <span className="px-2 py-0.5 bg-srapi-card-muted border border-srapi-border rounded text-[9px] font-bold text-srapi-text-primary">
                          {log.model}
                        </span>
                      </td>
                      <td className="py-4.5 px-6 whitespace-nowrap text-srapi-text-secondary">
                        {log.source_endpoint}
                      </td>
                      <td className="py-4.5 px-6 whitespace-nowrap">
                        <span className={`text-[10px] font-bold border px-2.5 py-0.5 rounded-full ${
                          log.success 
                            ? 'border-green-500/20 text-green-700 dark:text-green-500 bg-green-500/10' 
                            : 'border-srapi-error/20 text-srapi-error bg-srapi-error/5'
                        }`}>
                          {log.success ? (language === 'en' ? 'Success' : '成功') : (language === 'en' ? 'Failed' : '失败')}
                        </span>
                      </td>
                      <td className="py-4.5 px-6 text-right whitespace-nowrap font-medium text-srapi-text-primary">
                        {log.total_tokens.toLocaleString()}
                      </td>
                      <td className="py-4.5 px-6 text-right whitespace-nowrap text-srapi-primary font-bold">
                        ${log.cost.toFixed(6)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

      </div>
    </DashboardLayout>
  );
}
