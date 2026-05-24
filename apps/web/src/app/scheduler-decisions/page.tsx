'use client';

import React, { useState } from 'react';
import {
  GitBranch,
  Terminal as TermIcon,
  RefreshCw,
  ChevronDown,
  ChevronUp,
  AlertCircle,
  Clock,
  Coins,
  Cpu,
} from 'lucide-react';
import DashboardLayout from '../../components/DashboardLayout';
import { useSchedulerDecisions } from '@/hooks/queries';
import { useLanguage } from '../../context/LanguageContext';
import { PageQueryError, PageQueryLoading } from '@/components/layout/page-query-state';

export default function SchedulerDecisionsPage() {
  const { language, t } = useLanguage();
  const decisionsQuery = useSchedulerDecisions();
  const loading = decisionsQuery.isLoading;
  const [expandedId, setExpandedId] = useState<string | null>(null);
  const decisions = decisionsQuery.data ?? [];

  const toggleExpand = (id: string) => {
    setExpandedId(prev => prev === id ? null : id);
  };

  // SRapi v0.1.0 product tone, see docs/PRODUCT_TONE.md.
  const textTitle = language === 'en' ? 'Live scheduling decisions' : '实时调度决策';
  const textDesc = language === 'en'
    ? 'Each decision shows the picked provider account, candidate scores, and why other candidates were rejected.'
    : '每个决策呈现选中的上游账号、候选评分，以及其他候选被排除的原因。';
  const textDecisionsCount = language === 'en' ? 'REJECTED CANDIDATES' : '被排除的候选';

  return (
    <DashboardLayout allowedRole="admin">
      <div className="space-y-8 animate-bloom">
        
        {/* Header Block (rounded-2xl) */}
        <div className="bg-srapi-card border border-srapi-border rounded-2xl p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-6 tactile-card">
          <div className="space-y-1">
            <h3 className="font-serif font-medium text-lg tracking-tight">{textTitle}</h3>
            <p className="text-xs text-srapi-text-secondary font-sans leading-relaxed">{textDesc}</p>
          </div>

          <button
            onClick={() => void decisionsQuery.refetch()}
            className="px-5 py-3.5 border border-srapi-border rounded-full text-xs font-mono font-bold tracking-wider uppercase transition-all active:scale-[0.96] flex items-center gap-2 cursor-pointer text-srapi-text-secondary hover:bg-srapi-card-muted"
          >
            <RefreshCw size={12} />
            {language === 'en' ? 'Refresh' : '刷新'}
          </button>
        </div>

        {/* Decisions Feed Logs */}
        {loading ? (
          <PageQueryLoading label={t('accessingLogs')} />
        ) : decisionsQuery.isError ? (
          <PageQueryError error={decisionsQuery.error} onRetry={() => void decisionsQuery.refetch()} />
        ) : decisions.length === 0 ? (
          <div className="py-16 border border-dashed border-srapi-border rounded-2xl text-center space-y-3.5">
            <GitBranch className="mx-auto text-srapi-text-secondary opacity-40" size={28} />
            <p className="text-xs font-bold text-srapi-text-primary font-serif">
              {language === 'en' ? 'No scheduler decisions yet' : '暂无调度决策'}
            </p>
            <p className="text-[10px] text-srapi-text-secondary font-mono">
              {language === 'en'
                ? 'Send a gateway request to record real scheduler evidence.'
                : '发出网关请求后，这里会显示真实调度证据。'}
            </p>
          </div>
        ) : (
          <div className="space-y-6">
            {decisions.map((dec) => {
              const isExpanded = expandedId === dec.request_id;
              
              return (
                <div 
                  key={dec.request_id} 
                  className={`bg-srapi-card border border-srapi-border rounded-3xl transition-all overflow-hidden tactile-card ${
                    isExpanded ? 'ring-1 ring-srapi-primary border-srapi-primary/50 shadow-lg' : 'hover:bg-srapi-card-muted/10'
                  }`}
                >
                  
                  {/* Top Summary Banner */}
                  <div 
                    onClick={() => toggleExpand(dec.request_id)}
                    className="p-5 md:p-6 flex flex-col md:flex-row md:items-center justify-between gap-4 cursor-pointer select-none"
                  >
                    <div className="flex flex-wrap items-center gap-3.5">
                      <div className="p-2 bg-srapi-card-muted border border-srapi-border rounded-xl text-srapi-primary">
                        <GitBranch size={16} />
                      </div>
                      
                      <div className="space-y-1">
                        <div className="flex items-center gap-2.5">
                          <code className="text-xs font-bold text-srapi-text-primary select-all">
                            {dec.request_id}
                          </code>
                          <span className="px-2 py-0.5 bg-srapi-card-muted border border-srapi-border rounded text-[9px] font-bold text-srapi-text-secondary font-mono">
                            {dec.model}
                          </span>
                        </div>
                        <div className="text-[10px] font-mono text-srapi-text-secondary flex items-center gap-1.5 font-bold">
                          <span>{dec.source_endpoint}</span>
                          <span>•</span>
                          <span>{new Date(dec.created_at).toLocaleTimeString()}</span>
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-4 text-xs font-mono">
                      <div className="text-right hidden md:block space-y-0.5">
                        <span className="text-[9px] text-srapi-text-secondary block font-bold tracking-wider">{t('leasedUpstream')}</span>
                        <span className="font-bold text-srapi-text-primary font-serif italic">{dec.selected_account_name}</span>
                      </div>

                      <div className="flex items-center gap-2.5">
                        <span className="text-[10px] font-bold border border-green-500/20 text-green-700 bg-green-500/10 px-2.5 py-0.5 rounded-full uppercase">
                          {t('routed')}
                        </span>
                        {isExpanded ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
                      </div>
                    </div>
                  </div>

                  {/* Expanded Scoring Breakdown and Logs */}
                  {isExpanded && (
                    <div className="border-t border-srapi-border p-6 space-y-6 bg-srapi-card-muted/10 animate-bloom">
                      
                      {/* Grid sections */}
                      <div className="grid grid-cols-1 md:grid-cols-2 gap-6 md:gap-8">
                        
                        {/* Candidates Score Breakdown */}
                        <div className="space-y-4">
                          <span className="text-[10px] uppercase font-mono tracking-widest text-srapi-text-secondary font-bold block border-b border-srapi-border pb-1">
                            {t('candidatesScores')}
                          </span>

                          <div className="space-y-4.5">
                            {dec.scores.map((cand, idx) => (
                              <div key={idx} className="space-y-2">
                                <div className="flex justify-between text-[11px] font-mono">
                                  <span className="font-bold text-srapi-text-primary font-serif">{cand.account}</span>
                                  <span className="font-bold text-srapi-primary">{language === 'en' ? 'Score' : '评分'}: {cand.score}</span>
                                </div>

                                {/* Score matrix bars */}
                                <div className="space-y-1.5 text-[9px] font-mono text-srapi-text-secondary bg-srapi-card p-3 border border-srapi-border rounded-xl">
                                  {/* Latency */}
                                  <div className="flex items-center gap-3">
                                    <span className="w-20 flex items-center gap-0.5"><Clock size={10} /> {t('latencyMetric')}</span>
                                    <div className="flex-grow h-[1px] bg-srapi-border relative">
                                      <div className="absolute h-2 w-[1px] bg-green-700 -top-[3px]" style={{ left: `${cand.latency * 100}%` }}></div>
                                    </div>
                                    <span className="w-6 text-right">{(cand.latency * 10).toFixed(1)}</span>
                                  </div>

                                  {/* Cost */}
                                  <div className="flex items-center gap-3">
                                    <span className="w-20 flex items-center gap-0.5"><Coins size={10} /> {t('costMetric')}</span>
                                    <div className="flex-grow h-[1px] bg-srapi-border relative">
                                      <div className="absolute h-2 w-[1px] bg-srapi-primary -top-[3px]" style={{ left: `${cand.cost * 100}%` }}></div>
                                    </div>
                                    <span className="w-6 text-right">{(cand.cost * 10).toFixed(1)}</span>
                                  </div>

                                  {/* Quota */}
                                  <div className="flex items-center gap-3">
                                    <span className="w-20 flex items-center gap-0.5"><Cpu size={10} /> {t('quotaMetric')}</span>
                                    <div className="flex-grow h-[1px] bg-srapi-border relative">
                                      <div className="absolute h-2 w-[1px] bg-blue-500 -top-[3px]" style={{ left: `${cand.quota * 100}%` }}></div>
                                    </div>
                                    <span className="w-6 text-right">{(cand.quota * 10).toFixed(1)}</span>
                                  </div>
                                </div>
                              </div>
                            ))}
                          </div>
                        </div>

                        {/* Failover and Exclusions */}
                        <div className="space-y-4">
                          <span className="text-[10px] uppercase font-mono tracking-widest text-srapi-text-secondary font-bold block border-b border-srapi-border pb-1">
                            {textDecisionsCount} ({dec.rejected_count})
                          </span>

                          {dec.rejected_reasons.length === 0 ? (
                            <div className="p-5 border border-dashed border-srapi-border rounded-2xl text-center text-xs text-srapi-text-secondary font-mono">
                              {t('noFailoverDesc')}
                            </div>
                          ) : (
                            <div className="space-y-3 text-[10px] font-mono">
                              {dec.rejected_reasons.map((re, idx) => (
                                <div key={idx} className="p-4 border border-srapi-primary/20 bg-srapi-primary/5 rounded-2xl text-srapi-primary flex items-start gap-2.5">
                                  <AlertCircle size={14} className="mt-0.5 flex-shrink-0" />
                                  <div className="space-y-0.5">
                                    <p className="font-bold">{re.account}</p>
                                    <p className="text-[9px] text-srapi-text-secondary font-sans leading-relaxed">{re.reason}</p>
                                  </div>
                                </div>
                              ))}
                            </div>
                          )}
                        </div>

                      </div>

                      {/* Raw reasoning logs console */}
                      <div className="space-y-2.5">
                        <span className="text-[10px] uppercase font-mono tracking-widest text-srapi-text-secondary font-bold flex items-center gap-1.5">
                          <TermIcon size={12} />
                          {t('reasoningLogs')}
                        </span>

                        <pre className="p-4 bg-srapi-card border border-srapi-border rounded-2xl text-[10px] font-mono text-srapi-text-primary leading-relaxed space-y-1.5 overflow-x-auto select-all shadow-inner">
                          {dec.logs.map((logLine, idx) => (
                            <div key={idx} className="truncate">
                              {logLine}
                            </div>
                          ))}
                        </pre>
                      </div>

                    </div>
                  )}

                </div>
              );
            })}
          </div>
        )}

      </div>
    </DashboardLayout>
  );
}
