'use client';

import React, { useEffect, useState } from 'react';
import {
  Settings,
  GitBranch,
  Activity,
  Cpu,
  Play,
} from 'lucide-react';
import DashboardLayout from '../../components/DashboardLayout';
import { useOverviewStats, useSlos } from '@/hooks/queries';
import { useLanguage } from '../../context/LanguageContext';

interface SimStep {
  text: string;
  highlight: boolean;
}

export default function AdminDashboard() {
  const { language } = useLanguage();
  const statsQuery = useOverviewStats();
  const slosQuery = useSlos();
  const stats = statsQuery.data ?? null;
  const slos = slosQuery.data ?? [];
  const loading = statsQuery.isLoading || slosQuery.isLoading;

  // Simulation state stays local — it's a UI-only walkthrough animation.
  const [simModel, setSimModel] = useState('claude-3-7-sonnet');
  const [simStrategy, setSimStrategy] = useState('BAL');
  const [isSimulating, setIsSimulating] = useState(false);
  const [showSkeleton, setShowSkeleton] = useState(false);
  const [simSteps, setSimSteps] = useState<SimStep[]>([]);
  const [activeStepIdx, setActiveStepIdx] = useState(-1);
  const [typedLines, setTypedLines] = useState<string[]>([]);
  const [currentLineText, setCurrentLineText] = useState('');
  const [showPlaceholder, setShowPlaceholder] = useState(true);

  // Execution algorithm for streaming dispatcher typewriter
  const runSimulation = () => {
    setIsSimulating(true);
    setShowPlaceholder(false);
    setShowSkeleton(true);
    setTypedLines([]);
    setCurrentLineText('');
    setActiveStepIdx(-1);

    const reqId = `req_${Math.random().toString(36).substring(2, 10)}`;

    // SRapi v0.1.0: simulator log lines follow docs/PRODUCT_TONE.md §6.7.
    let steps: SimStep[] = [];
    if (simStrategy === 'BAL') {
      steps = [
        { text: language === 'en'
            ? `[1/5] request    id=${reqId} model=${simModel}`
            : `[1/5] 请求受理  id=${reqId} 模型=${simModel}`, highlight: false },
        { text: language === 'en'
            ? `[2/5] scheduler  capability ok, account groups resolved`
            : `[2/5] 调度器    能力匹配通过，账号组已解析`, highlight: false },
        { text: language === 'en'
            ? `[3/5] candidates 3 accounts to score`
            : `[3/5] 候选      待评分账号 3 个`, highlight: false },
        { text: language === 'en'
            ? `[4/5] excluded   openai-pro-02 cooldown not expired`
            : `[4/5] 已排除    openai-pro-02仅冷却未结束`, highlight: false },
        { text: language === 'en'
            ? `[5/5] selected   claude-sonnet-01  score=0.94\n      - health 1.00 (w 0.3) -> 0.300\n      - quota  0.85 (w 0.2) -> 0.170\n      - cache  0.90 (w 0.1) -> 0.090\n      - sticky 1.00 (w 0.1) -> 0.100\n      - cost   0.92 (w 0.1) -> 0.092`
            : `[5/5] 已选中    claude-sonnet-01  评分=0.94\n      - 健康 1.00 (权 0.3) -> 0.300\n      - 配额 0.85 (权 0.2) -> 0.170\n      - 缓存 0.90 (权 0.1) -> 0.090\n      - 粘性 1.00 (权 0.1) -> 0.100\n      - 成本 0.92 (权 0.1) -> 0.092`, highlight: true }
      ];
    } else if (simStrategy === 'COST') {
      steps = [
        { text: language === 'en'
            ? `[1/5] request    id=${reqId} model=${simModel} priority=cost`
            : `[1/5] 请求受理  id=${reqId} 模型=${simModel} 优先级=低成本`, highlight: false },
        { text: language === 'en'
            ? `[2/5] scheduler  policy=cost-saver-strict`
            : `[2/5] 调度器    策略=严格低成本`, highlight: false },
        { text: language === 'en'
            ? `[3/5] candidates 2 accounts to score`
            : `[3/5] 候选      待评分账号 2 个`, highlight: false },
        { text: language === 'en'
            ? `[4/5] cost       cheapest match: third-party-cheap`
            : `[4/5] 成本      价格最低：third-party-cheap`, highlight: false },
        { text: language === 'en'
            ? `[5/5] selected   third-party-cheap  score=0.98\n      - cost   1.00 (w 0.3)  -> 0.300\n      - health 0.85 (w 0.15) -> 0.127\n      - quota  0.90 (w 0.2)  -> 0.180\n      - cache  0.00 (w 0.15) -> 0.000`
            : `[5/5] 已选中    third-party-cheap  评分=0.98\n      - 成本 1.00 (权 0.3)  -> 0.300\n      - 健康 0.85 (权 0.15) -> 0.127\n      - 配额 0.90 (权 0.2)  -> 0.180\n      - 缓存 0.00 (权 0.15) -> 0.000`, highlight: true }
      ];
    } else {
      steps = [
        { text: language === 'en'
            ? `[1/5] request    id=${reqId} model=${simModel} priority=quality`
            : `[1/5] 请求受理  id=${reqId} 模型=${simModel} 优先级=高品质`, highlight: false },
        { text: language === 'en'
            ? `[2/5] scheduler  policy=quality-first, sticky lock on`
            : `[2/5] 调度器    策略=品质优先，粘性锁已开`, highlight: false },
        { text: language === 'en'
            ? `[3/5] candidates 3 accounts to score`
            : `[3/5] 候选      待评分账号 3 个`, highlight: false },
        { text: language === 'en'
            ? `[4/5] health     claude-sonnet-01 success_rate=100%`
            : `[4/5] 健康      claude-sonnet-01 成功率=100%`, highlight: false },
        { text: language === 'en'
            ? `[5/5] selected   claude-sonnet-01  score=0.97\n      - health  1.00 (w 0.4)  -> 0.400\n      - latency 0.95 (w 0.2)  -> 0.190\n      - quota   0.85 (w 0.15) -> 0.127\n      - sticky  1.00 (w 0.1)  -> 0.100`
            : `[5/5] 已选中    claude-sonnet-01  评分=0.97\n      - 健康 1.00 (权 0.4)  -> 0.400\n      - 延迟 0.95 (权 0.2)  -> 0.190\n      - 配额 0.85 (权 0.15) -> 0.127\n      - 粘性 1.00 (权 0.1)  -> 0.100`, highlight: true }
      ];
    }

    setSimSteps(steps);

    // 500ms server simulation lag, then trigger streaming
    setTimeout(() => {
      setShowSkeleton(false);
      setActiveStepIdx(0);
    }, 600);
  };

  // Handle typing progression inside step index
  useEffect(() => {
    if (activeStepIdx < 0 || activeStepIdx >= simSteps.length) {
      if (activeStepIdx >= simSteps.length) {
        const timeout = setTimeout(() => setIsSimulating(false), 0);
        return () => clearTimeout(timeout);
      }
      return;
    }

    const currentStep = simSteps[activeStepIdx];
    const textToType = currentStep.text;
    let charIdx = 0;
    queueMicrotask(() => setCurrentLineText(''));

    const interval = setInterval(() => {
      if (charIdx < textToType.length) {
        setCurrentLineText((prev) => prev + textToType.charAt(charIdx));
        charIdx++;
      } else {
        clearInterval(interval);
        // Complete current line
        setTypedLines((prev) => [...prev, textToType]);
        setCurrentLineText('');
        // Proceed to next step with slight pause
        setTimeout(() => {
          setActiveStepIdx((prev) => prev + 1);
        }, 180);
      }
    }, 10);

    return () => clearInterval(interval);
  }, [activeStepIdx, simSteps]);

  // SRapi v0.1.0 product tone, see docs/PRODUCT_TONE.md.
  const textDecrypting = language === 'en' ? 'Loading...' : '加载中...';
  const textEpochPerformance = language === 'en' ? 'Live metrics' : '实时指标';
  const textAdapterRegistry = language === 'en' ? 'Providers' : '服务商';
  const textConnectedPlatforms = language === 'en' ? 'CONNECTED' : '已接入';
  const textLiveCredentials = language === 'en' ? 'Provider accounts' : '上游账号';
  const textActiveAccountEntries = language === 'en' ? 'ACTIVE' : '活动';
  const textSchedulerLeases = language === 'en' ? 'Scheduler decisions' : '调度决策';
  const textEvaluationTelemetries = language === 'en' ? 'RECORDED' : '条记录';
  const textGatewayLogs = language === 'en' ? 'Requests' : '请求';
  const textAuditedTransactionSlots = language === 'en' ? 'LOGGED' : '条日志';

  const textSloTitle = language === 'en' ? 'SLO health' : 'SLO 健康度';
  const textComplianceRate = language === 'en' ? 'attainment' : '达成率';
  const textMinThreshold = language === 'en' ? 'Target' : '目标';
  const textScaleCap = language === 'en' ? '100% scale' : '100% 刻度';

  const textOperatorCli = language === 'en' ? 'CLI quick reference' : 'CLI 快速参考';
  const textDiagnosticInterface = language === 'en' ? 'Common admin commands' : '常用管理命令';
  const textCliDesc = language === 'en'
    ? 'Run these inside the SRapi container or your local checkout. They never touch credentials.'
    : '在 SRapi 容器或本地仓库中运行，不会接触凭据。';
  const textCliCmd1 = language === 'en' ? '1. WHO AM I (CURRENT ADMIN SESSION)' : '1. 查看当前管理者会话';
  const textCliCmd2 = language === 'en' ? '2. CONFIGURE A NEW PROVIDER ACCOUNT' : '2. 配置一个上游账号';

  const textScribeTitle = language === 'en' ? 'Routing simulator' : '调度模拟器';
  const textConfigureSim = language === 'en' ? 'Simulator' : '模拟器';
  const textSimDesc = language === 'en'
    ? 'Send a mock request and watch the scheduler pick a provider account in real time.'
    : '发出一个模拟请求，看调度器如何选中上游账号。';
  const textModelScope = language === 'en' ? 'Model' : '模型';
  const textStrategy = language === 'en' ? 'Strategy' : '策略';

  const textBal = language === 'en' ? 'Balanced (latency / cost)' : '平衡（延迟 / 成本）';
  const textCost = language === 'en' ? 'Cost saver (cheapest first)' : '成本优先（低价优先）';
  const textQuality = language === 'en' ? 'Quality first (best SLA)' : '品质优先（最佳 SLA）';
  const textExecuteDispatch = language === 'en' ? 'Run simulation' : '运行模拟';

  const textTraceLog = language === 'en' ? 'Trace log' : '调度日志';
  const textLiveEpoch = language === 'en' ? 'Live' : '实时';
  const textAwaiting = language === 'en' ? 'Click "Run simulation" to start.' : '点击 "运行模拟" 开始。';

  return (
    <DashboardLayout allowedRole="admin">
      {loading ? (
        <div className="py-12 text-center font-mono">
          <div className="w-6 h-6 border-t-2 border-srapi-primary rounded-full animate-spin mx-auto mb-3"></div>
          <p className="text-xs text-srapi-text-secondary">{textDecrypting}</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-12 gap-8 lg:gap-12 items-start">
          
          {/* LEFT PANEL: The Operational Status Desk (7 columns) */}
          <div className="lg:col-span-7 space-y-12 animate-bloom">
            
            {/* Section A: Current Epoch Metrics */}
            <div>
              <div className="font-serif text-sm italic text-srapi-text-secondary mb-5">{textEpochPerformance}</div>
              <div className="grid grid-cols-2 gap-6 md:gap-8">
                
                {/* Metric Card 1 */}
                <div className="bg-srapi-card border border-srapi-border rounded-2xl p-6 tactile-card space-y-3">
                  <div className="flex items-center justify-between text-srapi-text-secondary">
                    <span className="text-[10px] font-mono uppercase tracking-wider">{textAdapterRegistry}</span>
                    <Cpu size={14} className="text-srapi-primary" />
                  </div>
                  <div className="space-y-1">
                    <div className="font-serif text-3xl font-medium tracking-tight text-srapi-primary">{stats?.providers}</div>
                    <div className="text-[9px] font-mono text-srapi-text-secondary">{textConnectedPlatforms}</div>
                  </div>
                </div>

                {/* Metric Card 2 */}
                <div className="bg-srapi-card border border-srapi-border rounded-2xl p-6 tactile-card space-y-3">
                  <div className="flex items-center justify-between text-srapi-text-secondary">
                    <span className="text-[10px] font-mono uppercase tracking-wider">{textLiveCredentials}</span>
                    <Settings size={14} className="text-srapi-primary" />
                  </div>
                  <div className="space-y-1">
                    <div className="font-serif text-3xl font-medium tracking-tight text-srapi-text-primary">{stats?.accounts}</div>
                    <div className="text-[9px] font-mono text-srapi-text-secondary">{textActiveAccountEntries}</div>
                  </div>
                </div>

                {/* Metric Card 3 */}
                <div className="bg-srapi-card border border-srapi-border rounded-2xl p-6 tactile-card space-y-3">
                  <div className="flex items-center justify-between text-srapi-text-secondary">
                    <span className="text-[10px] font-mono uppercase tracking-wider">{textSchedulerLeases}</span>
                    <GitBranch size={14} className="text-srapi-primary" />
                  </div>
                  <div className="space-y-1">
                    <div className="font-serif text-3xl font-medium tracking-tight text-srapi-text-primary">{stats?.decisions}</div>
                    <div className="text-[9px] font-mono text-srapi-text-secondary">{textEvaluationTelemetries}</div>
                  </div>
                </div>

                {/* Metric Card 4 */}
                <div className="bg-srapi-card border border-srapi-border rounded-2xl p-6 tactile-card space-y-3">
                  <div className="flex items-center justify-between text-srapi-text-secondary">
                    <span className="text-[10px] font-mono uppercase tracking-wider">{textGatewayLogs}</span>
                    <Activity size={14} className="text-srapi-primary" />
                  </div>
                  <div className="space-y-1">
                    <div className="font-serif text-3xl font-medium tracking-tight text-srapi-text-primary">{stats?.usage_logs}</div>
                    <div className="text-[9px] font-mono text-srapi-text-secondary">{textAuditedTransactionSlots}</div>
                  </div>
                </div>

              </div>
            </div>

            {/* Section B: Service Level Objective Health */}
            <div>
              <div className="font-serif text-sm italic text-srapi-text-secondary mb-5">{textSloTitle}</div>
              <div className="space-y-6">
                {slos.map((slo) => (
                  <div key={slo.id} className="bg-srapi-card border border-srapi-border rounded-2xl p-6 tactile-card space-y-4">
                    <div className="flex items-center justify-between">
                      <div className="flex items-center space-x-2.5">
                        <span className="font-serif text-base font-medium">{slo.name}</span>
                        <span className="text-[9px] font-mono bg-srapi-card-muted border border-srapi-border px-2 py-0.5 rounded-full text-srapi-text-secondary font-bold">
                          {slo.window}
                        </span>
                      </div>
                      <span className={`text-[9px] font-mono uppercase border px-2.5 py-0.5 rounded-full font-bold ${
                        slo.status === 'healthy' 
                          ? 'border-green-500/20 text-green-700 dark:text-green-500 bg-green-500/10' 
                          : 'border-srapi-primary/20 text-srapi-primary bg-srapi-primary/5'
                      }`}>
                        {slo.status === 'healthy' ? (language === 'en' ? 'HEALTHY' : '健康') : (language === 'en' ? 'AT RISK' : '预警')}
                      </span>
                    </div>

                    <div className="flex items-baseline space-x-2">
                      <span className="font-serif text-3xl font-medium tracking-tight text-srapi-text-primary">{slo.availability.toFixed(2)}%</span>
                      <span className="text-[10px] font-mono text-srapi-text-secondary uppercase">{textComplianceRate}</span>
                    </div>

                    {/* Fine Sandbox Linear Slider Dial */}
                    <div>
                      <div className="relative w-full bg-srapi-border h-[1px]">
                        <div className="absolute h-3 w-[1px] bg-srapi-primary -top-1" style={{ left: `${Math.min(100, Math.max(0, slo.availability))}%` }}></div>
                      </div>
                      <div className="flex justify-between items-center mt-2.5 text-[9px] font-mono text-srapi-text-secondary">
                        <span>{textMinThreshold}: {slo.objective}%</span>
                        <span>{textScaleCap}</span>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>

            {/* Section C: Terminal CLI Command Reference */}
            <div>
              <div className="font-serif text-sm italic text-srapi-text-secondary mb-5">{textOperatorCli}</div>
              <div className="bg-srapi-card border border-srapi-border rounded-2xl p-6 tactile-card space-y-5">
                <div className="space-y-1">
                  <h3 className="font-serif text-base font-medium">{textDiagnosticInterface}</h3>
                  <p className="text-xs text-srapi-text-secondary">{textCliDesc}</p>
                </div>

                <div className="space-y-4 font-mono text-[10px]">
                  <div className="space-y-1.5">
                    <span className="text-srapi-text-secondary block font-bold tracking-wider">{textCliCmd1}</span>
                    <pre className="p-3 bg-srapi-card-muted border border-srapi-border rounded-xl text-srapi-text-primary overflow-x-auto select-all">
node tools/srapi-admin.mjs whoami --json
                    </pre>
                  </div>

                  <div className="space-y-1.5">
                    <span className="text-srapi-text-secondary block font-bold tracking-wider">{textCliCmd2}</span>
                    <pre className="p-3 bg-srapi-card-muted border border-srapi-border rounded-xl text-srapi-text-primary overflow-x-auto select-all">
node tools/srapi-admin.mjs configure-openai-account --model gpt-4o-mini --upstream-model gpt-4o-mini
                    </pre>
                  </div>
                </div>
              </div>
            </div>

          </div>

          {/* RIGHT PANEL: The Dispatcher Flow (5 columns) */}
          <div className="lg:col-span-5 space-y-8 lg:sticky lg:top-28 animate-bloom delay-300">
            <div className="font-serif text-sm italic text-srapi-text-secondary">{textScribeTitle}</div>

            {/* Simulator Configuration */}
            <div className="bg-srapi-card border border-srapi-border rounded-3xl p-6 tactile-card space-y-5">
              <div>
                <h3 className="font-serif text-base font-medium">{textConfigureSim}</h3>
                <p className="text-xs text-srapi-text-secondary leading-relaxed mt-1">
                  {textSimDesc}
                </p>
              </div>

              <div className="space-y-4">
                <div>
                  <label className="text-[9px] uppercase font-mono font-bold tracking-wider text-srapi-text-secondary block mb-1.5">{textModelScope}</label>
                  <select 
                    value={simModel} 
                    onChange={(e) => setSimModel(e.target.value)}
                    disabled={isSimulating}
                    className="w-full text-xs bg-srapi-bg border border-srapi-border rounded-xl px-3 py-3.5 font-mono text-srapi-text-primary cursor-pointer hover:border-srapi-primary/40 focus:border-srapi-primary transition-all disabled:opacity-50"
                  >
                    <option value="claude-3-7-sonnet">claude-3-7-sonnet</option>
                    <option value="gpt-4o">gpt-4o</option>
                    <option value="deepseek-r1">deepseek-r1</option>
                  </select>
                </div>

                <div>
                  <label className="text-[9px] uppercase font-mono font-bold tracking-wider text-srapi-text-secondary block mb-1.5">{textStrategy}</label>
                  <select 
                    value={simStrategy} 
                    onChange={(e) => setSimStrategy(e.target.value)}
                    disabled={isSimulating}
                    className="w-full text-xs bg-srapi-bg border border-srapi-border rounded-xl px-3 py-3.5 font-mono text-srapi-text-primary cursor-pointer hover:border-srapi-primary/40 focus:border-srapi-primary transition-all disabled:opacity-50"
                  >
                    <option value="BAL">{textBal}</option>
                    <option value="COST">{textCost}</option>
                    <option value="QUALITY">{textQuality}</option>
                  </select>
                </div>

                <button 
                  onClick={runSimulation}
                  disabled={isSimulating}
                  className="w-full bg-srapi-text-primary text-srapi-bg dark:bg-srapi-text-primary dark:text-srapi-bg text-xs font-mono tracking-widest uppercase py-4 rounded-full transition-all active:scale-[0.96] mt-4 font-bold border border-srapi-text-primary hover:bg-transparent hover:text-srapi-text-primary dark:hover:bg-transparent dark:hover:text-srapi-text-primary shadow-sm disabled:opacity-40 disabled:cursor-not-allowed flex items-center justify-center gap-2 cursor-pointer"
                >
                  <Play size={12} fill="currentColor" />
                  {textExecuteDispatch}
                </button>
              </div>
            </div>

            {/* Simulated Stream Log Box (rounded-3xl card with tactile feel) */}
            <div className="bg-srapi-card border border-srapi-border rounded-3xl p-6 min-h-[380px] flex flex-col tactile-card relative overflow-hidden">
              <div className="flex justify-between items-baseline text-[9px] font-mono text-srapi-text-secondary border-b border-srapi-border pb-3 mb-4 shrink-0 uppercase tracking-widest">
                <span>{textTraceLog}</span>
                <span>{textLiveEpoch}</span>
              </div>

              {/* Streaming Content Workspace */}
              <div className="flex-grow flex flex-col justify-start">
                
                {/* 1. Placeholder screen */}
                {showPlaceholder && (
                  <div className="my-auto text-center py-20">
                    <p className="font-serif text-sm italic text-srapi-text-secondary opacity-60">
                      {textAwaiting}
                    </p>
                  </div>
                )}

                {/* 2. Skeleton calculation buffer */}
                {showSkeleton && (
                  <div className="space-y-4 py-4 w-full">
                    <div className="h-3 w-3/4 bg-srapi-card-muted rounded-full animate-pulse"></div>
                    <div className="h-3 w-1/2 bg-srapi-card-muted rounded-full animate-pulse delay-75"></div>
                    <div className="h-3 w-5/6 bg-srapi-card-muted rounded-full animate-pulse delay-150"></div>
                  </div>
                )}

                {/* 3. Real-time stream printing logs */}
                {!showPlaceholder && !showSkeleton && (
                  <div className="space-y-3.5 font-mono text-[11px] leading-relaxed text-srapi-text-secondary">
                    {/* Typed Lines */}
                    {typedLines.map((line, i) => {
                      const isHighlighted = simSteps[i]?.highlight;
                      return (
                        <div 
                          key={i} 
                          className={`border-l-2 border-srapi-border pl-3.5 py-1 whitespace-pre-wrap ${
                            isHighlighted ? 'text-srapi-primary font-semibold' : 'text-srapi-text-primary'
                          }`}
                        >
                          {line}
                        </div>
                      );
                    })}

                    {/* Currently Printing Line */}
                    {activeStepIdx >= 0 && activeStepIdx < simSteps.length && (
                      <div className="border-l-2 border-srapi-border pl-3.5 py-1 whitespace-pre-wrap text-srapi-text-primary">
                        {currentLineText}
                        <span className="stream-cursor"></span>
                      </div>
                    )}
                  </div>
                )}

              </div>
            </div>

          </div>

        </div>
      )}
    </DashboardLayout>
  );
}
