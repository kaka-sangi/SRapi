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
import { AppShell } from "@/components/layout";
import { EmptyState, Skeleton } from '@/components/ui';
import { useSchedulerDecisions } from '@/hooks/queries';
import { useLanguage } from '../../context/LanguageContext';
import { PageQueryError } from '@/components/layout/page-query-state';

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
    <AppShell allowedRole="admin">
      <div className="space-y-8 animate-bloom">
        
        {/* Header Block (rounded-2xl) */}
        <div className="surface flex flex-col justify-between gap-6 rounded-2xl p-6 sm:flex-row sm:items-center">
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
          <SchedulerSkeleton />
        ) : decisionsQuery.isError ? (
          <PageQueryError error={decisionsQuery.error} onRetry={() => void decisionsQuery.refetch()} />
        ) : decisions.length === 0 ? (
          <EmptyState
            icon={<GitBranch size={18} />}
            title={language === 'en' ? 'No scheduler decisions yet' : '暂无调度决策'}
            description={
              language === 'en'
                ? 'Send a gateway request to record real scheduler evidence.'
                : '发出网关请求后，这里会显示真实调度证据。'
            }
          />
        ) : (
          <div className="space-y-6">
            {decisions.map((dec) => {
              const isExpanded = expandedId === dec.request_id;
              
              return (
                <div 
                  key={dec.request_id} 
                  className={`surface overflow-hidden rounded-3xl transition-all ${
                    isExpanded ? 'border-srapi-primary/50 shadow-lg ring-1 ring-srapi-primary' : 'hover:bg-srapi-card-muted/10'
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
                          <span className="px-2 py-0.5 bg-srapi-card-muted border border-srapi-border rounded text-2xs font-bold text-srapi-text-secondary font-mono">
                            {dec.model}
                          </span>
                        </div>
                        <div className="text-2xs font-mono text-srapi-text-secondary flex items-center gap-1.5 font-bold">
                          <span>{dec.source_endpoint}</span>
                          <span>•</span>
                          <span>{new Date(dec.created_at).toLocaleTimeString()}</span>
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-4 text-xs font-mono">
                      <div className="text-right hidden md:block space-y-0.5">
                        <span className="text-2xs text-srapi-text-secondary block font-bold tracking-wider">{t('leasedUpstream')}</span>
                        <span className="font-bold text-srapi-text-primary font-serif italic">{dec.selected_account_name}</span>
                      </div>

                      <div className="flex items-center gap-2.5">
                        <span className="rounded-full border border-srapi-success/20 bg-srapi-success/10 px-2.5 py-0.5 text-2xs font-bold uppercase text-srapi-success">
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
                          <span className="text-2xs uppercase font-mono tracking-widest text-srapi-text-secondary font-bold block border-b border-srapi-border pb-1">
                            {t('candidatesScores')}
                          </span>

                          <div className="space-y-4.5">
                            {dec.scores.map((cand, idx) => (
                              <div key={idx} className="space-y-2">
                                <div className="flex justify-between text-2xs font-mono">
                                  <span className="font-bold text-srapi-text-primary font-serif">{cand.account}</span>
                                  <span className="font-bold text-srapi-primary">{language === 'en' ? 'Score' : '评分'}: {cand.score}</span>
                                </div>

                                {/* Score matrix bars */}
                                <div className="space-y-1.5 text-2xs font-mono text-srapi-text-secondary bg-srapi-card p-3 border border-srapi-border rounded-xl">
                                  {/* Latency */}
                                  <div className="flex items-center gap-3">
                                    <span className="w-20 flex items-center gap-0.5"><Clock size={10} /> {t('latencyMetric')}</span>
                                    <div className="flex-grow h-[1px] bg-srapi-border relative">
                                      <div className="absolute -top-[3px] h-2 w-[1px] bg-srapi-success" style={{ left: `${cand.latency * 100}%` }}></div>
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
                                      <div className="absolute -top-[3px] h-2 w-[1px] bg-srapi-info" style={{ left: `${cand.quota * 100}%` }}></div>
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
                          <span className="text-2xs uppercase font-mono tracking-widest text-srapi-text-secondary font-bold block border-b border-srapi-border pb-1">
                            {textDecisionsCount} ({dec.rejected_count})
                          </span>

                          {dec.rejected_reasons.length === 0 ? (
                            <div className="p-5 border border-dashed border-srapi-border rounded-2xl text-center text-xs text-srapi-text-secondary font-mono">
                              {t('noFailoverDesc')}
                            </div>
                          ) : (
                            <div className="space-y-3 text-2xs font-mono">
                              {dec.rejected_reasons.map((re, idx) => (
                                <div key={idx} className="p-4 border border-srapi-primary/20 bg-srapi-primary/5 rounded-2xl text-srapi-primary flex items-start gap-2.5">
                                  <AlertCircle size={14} className="mt-0.5 flex-shrink-0" />
                                  <div className="space-y-0.5">
                                    <p className="font-bold">{re.account}</p>
                                    <p className="text-2xs text-srapi-text-secondary font-sans leading-relaxed">{re.reason}</p>
                                  </div>
                                </div>
                              ))}
                            </div>
                          )}
                        </div>

                      </div>

                      {/* Raw reasoning logs console */}
                      <div className="space-y-2.5">
                        <span className="text-2xs uppercase font-mono tracking-widest text-srapi-text-secondary font-bold flex items-center gap-1.5">
                          <TermIcon size={12} />
                          {t('reasoningLogs')}
                        </span>

                        <pre className="p-4 bg-srapi-card border border-srapi-border rounded-2xl text-2xs font-mono text-srapi-text-primary leading-relaxed space-y-1.5 overflow-x-auto select-all shadow-inner">
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
    </AppShell>
  );
}

/** Shimmer cards mirroring the collapsed decision feed while it loads. */
function SchedulerSkeleton() {
  return (
    <div className="space-y-6">
      {Array.from({ length: 3 }).map((_, i) => (
        <div key={i} className="surface flex items-center justify-between gap-4 rounded-3xl p-6">
          <div className="flex items-center gap-3.5">
            <Skeleton className="h-9 w-9 rounded-xl" />
            <div className="space-y-2">
              <Skeleton className="h-3 w-48" />
              <Skeleton className="h-3 w-32" />
            </div>
          </div>
          <Skeleton className="h-5 w-20 rounded-full" />
        </div>
      ))}
    </div>
  );
}
