'use client';

import React, { useState, useEffect } from 'react';
import { 
  Settings, 
  GitBranch, 
  Activity, 
  Cpu,
  Play
} from 'lucide-react';
import DashboardLayout from '../../components/DashboardLayout';
import { apiService } from '../../lib/api';
import { MockSlo } from '../../lib/mockData';
import { useLanguage } from '../../context/LanguageContext';

interface SimStep {
  text: string;
  highlight: boolean;
}

interface OverviewStats {
  providers: number;
  models: number;
  accounts: number;
  usage_logs: number;
  decisions: number;
}

export default function AdminDashboard() {
  const { language } = useLanguage();
  
  const [stats, setStats] = useState<OverviewStats | null>(null);
  const [slos, setSlos] = useState<MockSlo[]>([]);
  const [loading, setLoading] = useState(true);

  // Simulation state
  const [simModel, setSimModel] = useState('claude-3-7-sonnet');
  const [simStrategy, setSimStrategy] = useState('BAL');
  const [isSimulating, setIsSimulating] = useState(false);
  const [showSkeleton, setShowSkeleton] = useState(false);
  const [simSteps, setSimSteps] = useState<SimStep[]>([]);
  const [activeStepIdx, setActiveStepIdx] = useState(-1);
  const [typedLines, setTypedLines] = useState<string[]>([]);
  const [currentLineText, setCurrentLineText] = useState('');
  const [showPlaceholder, setShowPlaceholder] = useState(true);

  useEffect(() => {
    async function loadAdminData() {
      try {
        const [fetchedStats, fetchedSlos] = await Promise.all([
          apiService.getOverviewStats(),
          apiService.listSlos()
        ]);
        setStats(fetchedStats);
        setSlos(fetchedSlos);
      } catch (err) {
        console.error('Failed to fetch admin stats', err);
      } finally {
        setLoading(false);
      }
    }
    loadAdminData();
  }, []);

  // Execution algorithm for streaming dispatcher typewriter
  const runSimulation = () => {
    setIsSimulating(true);
    setShowPlaceholder(false);
    setShowSkeleton(true);
    setTypedLines([]);
    setCurrentLineText('');
    setActiveStepIdx(-1);

    const reqId = `req_${Math.random().toString(36).substring(2, 10)}`;

    let steps: SimStep[] = [];
    if (simStrategy === 'BAL') {
      steps = [
        { text: language === 'en' 
            ? `[1/5] [INFO] Request classified: ID=${reqId} | Model=${simModel} | Priority=NORMAL` 
            : `[1/5] [系统] 请求已分类识别: ID=${reqId} | 目标模型=${simModel} | 优先级=普通`, highlight: false },
        { text: language === 'en' 
            ? `[2/5] [SCHEDULER] Capability check passed. Allowed groups resolved.` 
            : `[2/5] [调度器] 租户权限校验通过。映射关联账户组解析完成。`, highlight: false },
        { text: language === 'en' 
            ? `[3/5] [EVALUATE] Evaluated 3 potential candidates...` 
            : `[3/5] [评估核对] 网关已对 3 个潜在候选上游执行健康建模...`, highlight: false },
        { text: language === 'en' 
            ? `[4/5] [FILTER] Rejected candidate [openai-pro-02] Reason: COOLDOWN_UNTIL_EXPIRED` 
            : `[4/5] [熔断过滤] 剔除候选上游 [openai-pro-02] 原因：冷却熔断期 (COOLDOWN) 未结束`, highlight: false },
        { text: language === 'en' 
            ? `[5/5] [RESOLVED] Selected: [claude-sonnet-01] | Net Score: 0.94\n  - Health:  1.00 (W: 0.3) -> 0.300\n  - Quota:   0.85 (W: 0.2) -> 0.170\n  - Cache:   0.90 (W: 0.1) -> 0.090 [Affinity Hit]\n  - Sticky:  1.00 (W: 0.1) -> 0.100 [Session Bound]\n  - Cost:    0.92 (W: 0.1) -> 0.092`
            : `[5/5] [最优仲裁] 选中上游：[claude-sonnet-01] | 净评分：0.94\n  - 健康系数:  1.00 (权重: 0.3) -> 0.300\n  - 配额余量:  0.85 (权重: 0.2) -> 0.170\n  - 缓存亲和:  0.90 (权重: 0.1) -> 0.090 [Affinity 命中]\n  - 粘性连接:  1.00 (权重: 0.1) -> 0.100 [会话绑定]\n  - 价格指数:  0.92 (权重: 0.1) -> 0.092`, highlight: true }
      ];
    } else if (simStrategy === 'COST') {
      steps = [
        { text: language === 'en'
            ? `[1/5] [INFO] Request classified: ID=${reqId} | Model=${simModel} | Priority=LOW_TIER`
            : `[1/5] [系统] 请求已分类识别: ID=${reqId} | 目标模型=${simModel} | 优先级=低成本`, highlight: false },
        { text: language === 'en'
            ? `[2/5] [SCHEDULER] Policy filter loaded: COST_SAVER_STRICT.`
            : `[2/5] [调度器] 过滤策略已载入：严格成本节约优先 (COST_SAVER_STRICT)。`, highlight: false },
        { text: language === 'en'
            ? `[3/5] [EVALUATE] Scanning cost margins for 2 accounts...`
            : `[3/5] [评估核对] 网关已对 2 个低廉候选上游执行比价建模...`, highlight: false },
        { text: language === 'en'
            ? `[4/5] [COST] Preferred cheapest channel: third-party-cheap`
            : `[4/5] [价格过滤] 优选最廉价分发通道：third-party-cheap`, highlight: false },
        { text: language === 'en'
            ? `[5/5] [RESOLVED] Selected: [third-party-cheap] | Net Score: 0.98\n  - Cost:    1.00 (W: 0.3) -> 0.300 [Price Best]\n  - Health:  0.85 (W: 0.15) -> 0.127\n  - Quota:   0.90 (W: 0.2) -> 0.180\n  - Cache:   0.00 (W: 0.15) -> 0.000`
            : `[5/5] [最优仲裁] 选中上游：[third-party-cheap] | 净评分：0.98\n  - 价格指数:  1.00 (权重: 0.3) -> 0.300 [价格最优]\n  - 健康系数:  0.85 (权重: 0.15) -> 0.127\n  - 配额余量:  0.90 (权重: 0.2) -> 0.180\n  - 缓存亲和:  0.00 (权重: 0.15) -> 0.000`, highlight: true }
      ];
    } else {
      steps = [
        { text: language === 'en'
            ? `[1/5] [INFO] Request classified: ID=${reqId} | Model=${simModel} | Priority=HIGH_PRO`
            : `[1/5] [系统] 请求已分类识别: ID=${reqId} | 目标模型=${simModel} | 优先级=高级尊享`, highlight: false },
        { text: language === 'en'
            ? `[2/5] [SCHEDULER] Priority lease requested. Hard-sticky lock enabled.`
            : `[2/5] [调度器] 检测到高品质租约请求。已启用强粘性锁。`, highlight: false },
        { text: language === 'en'
            ? `[3/5] [EVALUATE] Scanning metrics for 3 active accounts...`
            : `[3/5] [评估核对] 对 3 个活动上游执行品质与高可用 SLA 比对...`, highlight: false },
        { text: language === 'en'
            ? `[4/5] [HEALTH] Target account [claude-sonnet-01] verified at 100% success rate.`
            : `[4/5] [健康检测] 目标账号 [claude-sonnet-01] 通过了 100% 成功率高水位校验。`, highlight: false },
        { text: language === 'en'
            ? `[5/5] [RESOLVED] Selected: [claude-sonnet-01] | Net Score: 0.97\n  - Health:  1.00 (W: 0.4) -> 0.400 [Flawless Snapshot]\n  - Latency: 0.95 (W: 0.2) -> 0.190\n  - Quota:   0.85 (W: 0.15) -> 0.127\n  - Sticky:  1.00 (W: 0.1) -> 0.100`
            : `[5/5] [最优仲裁] 选中上游：[claude-sonnet-01] | 净评分：0.97\n  - 健康系数:  1.00 (权重: 0.4) -> 0.400 [SLA 无瑕疵]\n  - 延迟指标:  0.95 (权重: 0.2) -> 0.190\n  - 配额余量:  0.85 (权重: 0.15) -> 0.127\n  - 粘性锁定:  1.00 (权重: 0.1) -> 0.100`, highlight: true }
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

  // Localized texts
  const textDecrypting = language === 'en' ? 'Decrypting control plane telemetry...' : '正在解密控制平面度量指标...';
  const textEpochPerformance = language === 'en' ? 'Current Epoch Performance' : '当前周期运行表现';
  const textAdapterRegistry = language === 'en' ? 'Adapter Registry' : '适配器注册数';
  const textConnectedPlatforms = language === 'en' ? 'CONNECTED PLATFORMS' : '已连接平台';
  const textLiveCredentials = language === 'en' ? 'Live Credentials' : '活动服务商凭证数';
  const textActiveAccountEntries = language === 'en' ? 'ACTIVE ACCOUNT ENTRIES' : '个活动提供商账户';
  const textSchedulerLeases = language === 'en' ? 'Scheduler Leases' : '调度决策数';
  const textEvaluationTelemetries = language === 'en' ? 'EVALUATION TELEMETRIES' : '条决策遥测日志';
  const textGatewayLogs = language === 'en' ? 'Gateway Logs' : '网关流量日志数';
  const textAuditedTransactionSlots = language === 'en' ? 'AUDITED TRANSACTION SLOTS' : '个已审计交易插槽';
  
  const textSloTitle = language === 'en' ? 'Service Level Objective (SLO) Health' : '服务等级目标 (SLO) 健康度';
  const textComplianceRate = language === 'en' ? 'Compliance rate' : '合规率';
  const textMinThreshold = language === 'en' ? 'Min Threshold' : '最低目标值';
  const textScaleCap = language === 'en' ? 'Scale Cap: 100%' : '刻度上限: 100%';
  
  const textOperatorCli = language === 'en' ? 'Operator CLI Diagnostic Reference' : '操作员 CLI 诊断参考说明';
  const textDiagnosticInterface = language === 'en' ? 'Diagnostic Command Interface' : '诊断命令行接口';
  const textCliDesc = language === 'en' ? 'Execute deep configuration and health audits directly inside the container terminal.' : '直接在容器终端内执行深入的配置与健康审计。';
  const textCliCmd1 = language === 'en' ? '1. EXPORT UPSTREAM HEALTH CRITERIA' : '1. 导出上游健康检查标准';
  const textCliCmd2 = language === 'en' ? '2. CONFIGURE ANTHROPIC UPSTREAM ACCOUNT' : '2. 配置上游服务商账户';
  
  const textScribeTitle = language === 'en' ? 'Dispatcher Scribe & Simulator' : '智能调度记录仪与模拟器';
  const textConfigureSim = language === 'en' ? 'Configure Dispatch Simulation' : '配置分发模拟选项';
  const textSimDesc = language === 'en' ? 'Trigger mock gateway query requests. The monitoring output will trace candidate evaluations and optimal routing decisions in real-time.' : '手动触发网关模拟查询请求，观察日志追溯评分计算与最优路由调度决策。';
  const textModelScope = language === 'en' ? 'Model Request Scope' : '请求目标大模型范围';
  const textStrategy = language === 'en' ? 'Scheduler Strategy' : '调度程序优先级算法';
  
  const textBal = language === 'en' ? 'Balanced (Weight Latency/Cost)' : '平衡策略 (均衡延迟与成本)';
  const textCost = language === 'en' ? 'Cost Saver (Strict Price Margin)' : '成本省钱模式 (严格低价优先)';
  const textQuality = language === 'en' ? 'Quality First (100% SLA Pool)' : '质量第一 (100% SLA 连接池)';
  const textExecuteDispatch = language === 'en' ? 'Execute Dispatch Pipeline' : '启动路由调度分发';
  
  const textTraceLog = language === 'en' ? 'Dispatch Trace Log' : '调度引擎追踪日志';
  const textLiveEpoch = language === 'en' ? 'Epoch Index: Live' : '当前索引: 实时监控';
  const textAwaiting = language === 'en' ? 'Awaiting routing instructions...' : '等待注入调度路由决策指令...';

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
                        {slo.status === 'healthy' ? (language === 'en' ? 'HEALTHY' : '健康良好') : (language === 'en' ? 'DEGRADED' : '降级预警')}
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
