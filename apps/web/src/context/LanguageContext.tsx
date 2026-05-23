'use client';

import { ReactNode, createContext, useContext, useEffect, useState } from 'react';

type Language = 'en' | 'zh';

interface LanguageContextType {
  language: Language;
  toggleLanguage: () => void;
  t: (key: string, variables?: Record<string, string | number>) => string;
}

const translations = {
  en: {
    // Navigation & Global Shell
    operatorConsole: 'Operator Console',
    developerConsole: 'Developer Console',
    switchRole: 'Switch to other console view',
    terminateSession: 'Terminate Session',
    liveApi: 'Live API',
    demoData: 'Demo Data',
    demoOnlySwitch: 'Switch demo console role',
    smokeEvidence: 'Smoke Evidence',
    complete: 'Complete',
    notComplete: 'not complete',
    operatorName: 'Operator',
    availableBalance: 'Available Balance',
    themeToggle: 'Toggle Theme',
    authenticating: 'Authenticating operator credentials...',
    languageName: '简体中文',
    
    // Paths
    navOverview: 'Overview',
    navApiKeys: 'API Keys',
    navUsageHistory: 'Usage History',
    navUsageLogs: 'Usage Logs',
    navProviderAccounts: 'Provider Accounts',
    navSchedulerDecisions: 'Scheduler Decisions',

    // Login page
    verifyOperator: 'Verify Operator Credentials',
    consolePassphraseDesc: 'Enter your credential details to authorize a secure session with the administrative gateway control plane.',
    operatorIdentity: 'Operator Identity',
    consolePassphrase: 'Console Security Passphrase',
    authenticate: 'Authenticate',
    decrypting: 'Decrypting Session...',
    quickTest: 'Quick Test Environments (Local Console)',
    adminAccount: 'Admin Account',
    devAccount: 'Developer Account',
    loginError: 'Please provide operator credentials.',
    authRejected: 'Authentication rejected. Verify email and password.',

    // Dashboard Overview (Developer)
    devCat: 'Developer Operations Console',
    devTitle: 'The Client Ingress & Invocations Account Ledger',
    devDesc: 'View personal developer account balances, deploy cryptographic access keys, and evaluate recent gateway proxy invocations.',
    accSummary: 'Account Capital Pool Summary',
    activeBalance: 'Active Balances Balance',
    currentQuotaTitle: 'Current Rate-Limit Capacity',
    balanceUsage: 'Balance Consumption Rate',
    ofAllowed: 'of authorized maximum',
    assignedModels: 'Assigned Target Models',
    scopeAllowed: 'Allowed scope model registry',
    authCredentials: 'Authorized Credentials List',
    apiKeyChannel: 'API Key Channel Name',
    creationDate: 'Creation Date',
    associatedGroup: 'Associated Groups',
    channelStatus: 'Channel Status',
    
    // Admin Overview
    adminCat: 'Technical Control Plane',
    adminTitle: 'The Adaptive Dispatch Interface & Architectural Control',
    adminDesc: 'System-wide availability metrics, core adapter health metrics, target objective compliance, and live scheduler diagnostics.',
    engineHealth: 'Gateway Kernel Status',
    activeConnections: 'Active Client Connections',
    averageLatency: 'Global Latency (Average)',
    sliCompliance: 'Routing SLO Compliance',
    concurrencyRail: 'Adapter Ingress Concurrency Rail',
    rateLimitsSla: 'Rate-limits and SLA weights leases',
    cliConsole: 'Control Plane Command Line',
    dispatcherScribe: 'Interactive Dispatcher Scribe & Simulator',
    scribeDesc: 'Visualizer tracing character-by-character live candidate scoring weight computations in real-time.',
    activeSimulation: 'ACTIVE INVOCATION SCHEDULER MATRIX FEED',
    pauseSimulation: 'PAUSE LIVE SIMULATION FEED',
    initiateSimulation: 'INITIATE LIVE SIMULATION FEED',
    evaluationLogs: 'Evaluation reasoning traces',

    // API Keys page
    apiCat: 'Cryptographic Credentials Vault',
    apiTitle: 'The Token Registry & Gateway Authorizations',
    apiDesc: 'Provision secure, HMAC-hashed API keys scoped to tenant groups. Plaintext values are only shown exactly once during creation.',
    generateKey: 'Generate Channel Key',
    secretKeyGenerated: 'Plaintext Secret Key Generated',
    keyWarning: 'This key is stored securely using an HMAC hash. For absolute security, this plaintext key will only be shown once. Copy it immediately. If misplaced, you must generate a new channel key.',
    copiedClipboard: 'Copied to Clipboard',
    copyPlaintext: 'Copy Plaintext Key',
    activeChannels: 'Active API Channels',
    queryRegistry: 'Querying tenant key registry...',
    noKeys: 'No Access Keys Found',
    noKeysDesc: 'Create a new key to begin invocation traffic through the gateway.',
    keyName: 'Key Identifier',
    prefix: 'Prefix Value',
    allowedModels: 'Allowed Models',
    status: 'Status',
    created: 'Created Date',
    actions: 'Revoke / Toggle',
    revoke: 'Revoke',
    activate: 'Activate',
    deployTitle: 'Generate API Key Channel',
    deployDesc: 'Deploy a new cryptographical key scoped to your account groups.',
    keyNickname: 'Key Nickname',
    allowedTargetModels: 'Allowed Target Models',
    scopeGroupsCsv: 'Scope Account Groups (CSV)',
    cancel: 'Cancel',
    deployChannel: 'Deploy Channel',
    deploying: 'Deploying...',

    // Usage logs page
    usageCat: 'Transactional Telemetry Auditing',
    usageTitle: 'The SLA Invocations Ledger & Audit Evidence',
    usageDesc: 'Real-time gateway transaction logs showing client routing states, model configurations, latency metrics, and costs.',
    auditedTraffic: 'Audited Traffic',
    invocationsEvaluated: 'INVOCATIONS EVALUATED',
    routerSla: 'Router SLA',
    successCoeff: 'ROUTING SUCCESS COEFF',
    payloadRouted: 'Payload Routed',
    totalTokens: 'TOTAL INTEGRATED TOKENS',
    financialCost: 'Financial Cost',
    estimatedDebit: 'ESTIMATED DEBIT VALUE',
    searchPlaceholder: 'Filter by Request ID or Source Endpoint path...',
    filtersLabel: 'Filters:',
    allModelScopes: 'All Model Scopes',
    allResponseStates: 'All Response States',
    successOnly: '200 OK Only',
    errorsOnly: 'System Errors Only',
    globalLogs: 'Global System Audit Logs',
    personalLogs: 'Personal Transaction History',
    showingEvents: 'Showing {filtered} of {total} events',
    fetchingEvidence: 'Fetching audit evidence...',
    noTraffic: 'No Matching Traffic Found',
    noTrafficDesc: 'Try relaxing search query parameters or filters.',
    timestamp: 'Timestamp',
    requestId: 'Request ID',
    sourcePath: 'Source Path',
    resultStatus: 'Result Status',
    reroutedTokens: 'Rerouted Tokens',
    transactCost: 'Transactional Cost',

    // Provider Accounts page
    provCat: 'Upstream Adapter Mappings',
    provTitle: 'The Large Language Model Credentials Pool',
    provDesc: 'Manage credentials, endpoints, priority weights, and active quota levels for connected upstream foundation LLM providers.',
    specifications: 'JSON Schema Specifications',
    resolvingAccounts: 'Resolving active upstream accounts...',
    class: 'Class',
    proxyEndpoint: 'Proxy Endpoint',
    scopeMaps: 'Scope Maps',
    latencyAvg: 'Lease Latency (Avg)',
    quotaRemainder: 'Quota Remainder',
    verifyLink: 'Verify Link',
    verifying: 'Verifying...',
    verified: 'Verified',
    rejected: 'Rejected (401 Auth)',
    declarationsTitle: 'Operator Declarations',
    declarationsDesc: 'To satisfy Zero-Trust configurations, upstream account pools are declared through secure JSON configuration scripts during runtime bootstrap.',
    provisionSchema: 'ACCOUNTS PROVISION SCHEMA (.json)',
    writeOnlyGuarantee: 'Write-Only Cryptographic Guarantee',
    writeOnlyDesc: 'Credentials uploaded through the routing proxy are encrypted as write-only states and can never be queried or serialized again. Administrative API exports will strictly show only base URL routing rules and account adapter names.',

    // Scheduler Decisions
    schedCat: 'Dynamic Scheduling Diagnostics',
    schedTitle: 'The Real-Time Decision Registry & Fallback Evidence',
    schedDesc: 'Inspect scheduler candidate scores (latency, cost, quota) and detailed failover rejection logs generated by the dispatch engine.',
    streaming: 'SIMULATOR: STREAMING',
    paused: 'SIMULATOR: PAUSED',
    accessingLogs: 'Accessing lease logs...',
    leasedUpstream: 'Leased Upstream',
    routed: 'Routed',
    candidatesScores: 'LEASE CANDIDATES SCORES',
    latencyMetric: 'Latency',
    costMetric: 'Cost',
    quotaMetric: 'Quota',
    failoverTitle: 'FAILOVER / EXCLUDED CANDIDATES',
    noFailoverDesc: 'No candidates excluded during routing cycle.',
    reasoningLogs: 'SCHEDULER ENGINE REASONING LOGS',

    // Smoke Drawer
    smokeDrawerTitle: 'v0.1 Smoke Evidence Checklist',
    statusTitle: 'Status',
    smokeDesc: 'The gateway has logged authentic client traffic and successful scheduler routing choices matching public HTTPS upstream providers.',
    smokeIncomplete: 'Missing traffic evidence or public upstream config. In accordance with the v0.1 design directive, the UI must show "not complete" and cannot mock data to deceive users.',
    constraintsMatrix: 'CONSTRAINTS MATRIX',
    modelEntryVerif: 'Model Entry Verification',
    modelRegistered: 'Model "{model}" is registered',
    exists: 'Exists',
    missing: 'Missing',
    publicUpstreamAcc: 'Public Upstream Accounts',
    requiresPublic: 'Requires at least 1 public HTTPS provider account',
    activeAccounts: '{count} active accounts',
    healthyTrafficRegistry: 'Healthy Traffic Registry',
    loggedHealthy: 'Logged healthy responses on all three required endpoints:',
    completeState: 'Complete',
    incompleteState: 'Incomplete',
    schedulerRouting: 'Upstream Scheduler Routing',
    decisionsRouting: 'Decisions routing traffic to public HTTPS accounts on endpoints:',
    diagnosticInstructions: 'Console Diagnostic Instructions:',
    instr1: '1. Connect a model, provider, and active account in the admin panel.',
    instr2: '2. Set target credentials to a valid public HTTPS provider.',
    instr3: '3. Generate client query requests to record evidence logs.'
  },
  zh: {
    // 导航与全局面板
    operatorConsole: '操作员控制台',
    developerConsole: '开发者控制台',
    switchRole: '切换控制台视图',
    terminateSession: '终止会话',
    liveApi: '实时 API',
    demoData: '演示数据',
    demoOnlySwitch: '切换演示控制台角色',
    smokeEvidence: '冒烟测试证据',
    complete: '已完成',
    notComplete: '未完成',
    operatorName: '操作员',
    availableBalance: '可用余额',
    themeToggle: '切换主题',
    authenticating: '正在验证操作员凭证...',
    languageName: 'English',

    // 路由导航
    navOverview: '概览',
    navApiKeys: 'API 密钥',
    navUsageHistory: '使用历史',
    navUsageLogs: '使用日志',
    navProviderAccounts: '服务商账户',
    navSchedulerDecisions: '调度决策',

    // 登录页
    verifyOperator: '验证操作员凭证',
    consolePassphraseDesc: '请输入您的凭证详情，以授权建立与管理网关控制平面的安全会话。',
    operatorIdentity: '操作员身份',
    consolePassphrase: '控制台安全口令',
    authenticate: '身份验证',
    decrypting: '正在解密会话...',
    quickTest: '快速测试环境 (本地控制台)',
    adminAccount: '管理员账户',
    devAccount: '开发者账户',
    loginError: '请提供操作员凭证。',
    authRejected: '身份验证被拒绝。请核对邮箱和密码。',

    // 开发者概览页
    devCat: '开发者运维控制台',
    devTitle: '客户端入口与调用账户总账',
    devDesc: '查看个人开发者账户余额，部署加密访问密钥，并评估最近的网关代理调用记录。',
    accSummary: '账户资金池摘要',
    activeBalance: '活动账户余额',
    currentQuotaTitle: '当前频率限制容量',
    balanceUsage: '余额消耗率',
    ofAllowed: '占授权最大额度',
    assignedModels: '分配的目标模型',
    scopeAllowed: '允许作用域的模型注册表',
    authCredentials: '授权凭证列表',
    apiKeyChannel: 'API 密钥通道名称',
    creationDate: '创建日期',
    associatedGroup: '关联组',
    channelStatus: '通道状态',

    // 管理员概览页
    adminCat: '技术控制平面',
    adminTitle: '自适应调度接口与架构控制',
    adminDesc: '系统范围可用性指标、核心适配器健康指标、目标目标合规性以及实时调度诊断。',
    engineHealth: '网关内核状态',
    activeConnections: '活动客户端连接',
    averageLatency: '全局延迟 (平均)',
    sliCompliance: '调度 SLO 合规率',
    concurrencyRail: '适配器入口并发度指标栏',
    rateLimitsSla: '速率限制与 SLA 权重租约',
    cliConsole: '控制平面命令行',
    dispatcherScribe: '交互式调度记录器与模拟器',
    scribeDesc: '实时逐字追踪候选评分权重计算的模拟器可视化面板。',
    activeSimulation: '活动调用调度矩阵源',
    pauseSimulation: '暂停实时模拟流',
    initiateSimulation: '启动实时模拟流',
    evaluationLogs: '评估推理路径追踪',

    // API 密钥管理页
    apiCat: '密码凭证保险库',
    apiTitle: '令牌注册表与网关授权',
    apiDesc: '配置作用于租户组的安全、经过 HMAC 哈希处理的 API 密钥。明文值在创建过程中仅显示一次。',
    generateKey: '生成通道密钥',
    secretKeyGenerated: '已生成明文密钥',
    keyWarning: '此密钥使用 HMAC 哈希安全存储。为了绝对安全，此明文密钥**仅显示一次**。请立即复制。如果遗失，您必须重新生成通道密钥。',
    copiedClipboard: '已复制到剪贴板',
    copyPlaintext: '复制明文密钥',
    activeChannels: '活动 API 通道',
    queryRegistry: '正在查询租户密钥注册表...',
    noKeys: '未找到访问密钥',
    noKeysDesc: '创建一个新密钥，开始通过网关进行流量传输。',
    keyName: '密钥标识符',
    prefix: '前缀值',
    allowedModels: '允许的模型',
    status: '状态',
    created: '创建日期',
    actions: '撤销 / 启用',
    revoke: '撤销',
    activate: '激活',
    deployTitle: '生成 API 密钥通道',
    deployDesc: '部署一个作用于您的账户组的新加密密钥。',
    keyNickname: '密钥别名',
    allowedTargetModels: '允许的目标模型',
    scopeGroupsCsv: '作用域账户组 (CSV)',
    cancel: '取消',
    deployChannel: '部署通道',
    deploying: '正在部署...',

    // 使用情况页
    usageCat: '事务遥测审计',
    usageTitle: 'SLA 调用账本与审计证据',
    usageDesc: '实时网关交易日志，显示客户端路由状态、模型配置、延迟指标和成本。',
    auditedTraffic: '已审计流量',
    invocationsEvaluated: '个已评估的调用',
    routerSla: '路由器 SLA',
    successCoeff: '路由成功系数',
    payloadRouted: '已路由载荷',
    totalTokens: '总计集成令牌数',
    financialCost: '财务成本',
    estimatedDebit: '预估借记额度',
    searchPlaceholder: '通过请求 ID 或源端点路径过滤...',
    filtersLabel: '筛选器:',
    allModelScopes: '所有模型作用域',
    allResponseStates: '所有响应状态',
    successOnly: '仅 200 OK',
    errorsOnly: '仅系统错误',
    globalLogs: '全局系统审计日志',
    personalLogs: '个人交易历史记录',
    showingEvents: '正在显示 {filtered} / {total} 个事件',
    fetchingEvidence: '正在获取审计证据...',
    noTraffic: '未找到匹配的流量记录',
    noTrafficDesc: '尝试放宽搜索查询参数或筛选条件。',
    timestamp: '时间戳',
    requestId: '请求 ID',
    sourcePath: '源路径',
    resultStatus: '结果状态',
    reroutedTokens: '重定向令牌',
    transactCost: '事务成本',

    // 提供商账户管理
    provCat: '上游适配器映射',
    provTitle: '大语言模型凭证池',
    provDesc: '管理已连接的上游基础大模型提供商的凭证、端点、优先级权重和活动配额水平。',
    specifications: 'JSON 架构配置规范',
    resolvingAccounts: '正在解析活动的提供商账户...',
    class: '类型',
    proxyEndpoint: '代理端点',
    scopeMaps: '范围映射',
    latencyAvg: '租约延迟 (平均)',
    quotaRemainder: '配额余额',
    verifyLink: '校验链路',
    verifying: '正在校验...',
    verified: '已验证',
    rejected: '被拒绝 (401 鉴权)',
    declarationsTitle: '操作员声明规程',
    declarationsDesc: '为了满足零信任配置，上游账户池是通过运行引导期间的安全 JSON 配置文件声明的。',
    provisionSchema: '账户分配架构规范 (.json)',
    writeOnlyGuarantee: '只写加密状态担保',
    writeOnlyDesc: '通过路由代理上传的凭证会被完全加密为只写状态，绝无法再次被查询或序列化。管理 API 导出中将严格仅显示基础 URL 路由规则与服务商适配器名称。',

    // 调度决策
    schedCat: '动态调度诊断',
    schedTitle: '实时决策注册表与回退证据',
    schedDesc: '检查调度候选者得分（延迟、成本、配额）以及分发引擎生成的详细故障转移拒绝日志。',
    streaming: '模拟器: 正在流式传输',
    paused: '模拟器: 已暂停',
    accessingLogs: '正在访问租约日志...',
    leasedUpstream: '已租用上游',
    routed: '已路由',
    candidatesScores: '租约候选者评分明细',
    latencyMetric: '延迟',
    costMetric: '成本',
    quotaMetric: '配额',
    failoverTitle: '故障转移 / 已排除候选者',
    noFailoverDesc: '在此调度周期中没有候选者被排除。',
    reasoningLogs: '调度引擎推理日志记录',

    // 冒烟面板
    smokeDrawerTitle: 'v0.1 冒烟证据核对清单',
    statusTitle: '状态',
    smokeDesc: '网关已记录了与公共 HTTPS 上游提供商匹配的真实客户端流量和成功的调度程序路由选择。',
    smokeIncomplete: '缺少流量证据或公共上游配置。按照 v0.1 设计指令，UI 必须显示 "未完成" (not complete) 状态，不能制造虚假数据来欺骗用户。',
    constraintsMatrix: '约束验证矩阵',
    modelEntryVerif: '模型项验证状态',
    modelRegistered: '模型 "{model}" 已注册',
    exists: '已存在',
    missing: '缺失',
    publicUpstreamAcc: '公共上游服务账户',
    requiresPublic: '需要至少 1 个经过验证的公共 HTTPS 服务商账户',
    activeAccounts: '已验证 {count} 个活动账户',
    healthyTrafficRegistry: '健康流量日志注册表',
    loggedHealthy: '在以下三个必须端点上都记录到了健康的成功响应流量:',
    completeState: '已完成',
    incompleteState: '未完成',
    schedulerRouting: '上游调度分发路由',
    decisionsRouting: '在以下端点上将流量路由至公共 HTTPS 账户的调度决策证据:',
    diagnosticInstructions: '控制台诊断规程说明:',
    instr1: '1. 在管理员面板中连接一个模型、提供商和活动账户。',
    instr2: '2. 将目标上游凭据配置为有效的公共 HTTPS 提供商。',
    instr3: '3. 生成客户端请求，以记录真实的运行证据日志。'
  }
} as const;

type TranslationKey = keyof typeof translations.en;

const LanguageContext = createContext<LanguageContextType | undefined>(undefined);

function getInitialLanguage(): Language {
  return 'en';
}

export function LanguageProvider({ children }: { children: ReactNode }) {
  const [language, setLanguage] = useState<Language>(getInitialLanguage);

  useEffect(() => {
    const savedLang = localStorage.getItem('srapi_lang');
    if (savedLang === 'en' || savedLang === 'zh') {
      queueMicrotask(() => setLanguage(savedLang));
    }
  }, []);

  const toggleLanguage = () => {
    const nextLang = language === 'en' ? 'zh' : 'en';
    setLanguage(nextLang);
    localStorage.setItem('srapi_lang', nextLang);
  };

  const t = (key: string, variables?: Record<string, string | number>) => {
    const dict = translations[language];
    const translationKey = key as TranslationKey;
    let template: string = dict[translationKey] || translations.en[translationKey] || key;
    
    if (variables) {
      Object.entries(variables).forEach(([k, v]) => {
        template = template.replace(new RegExp(`{${k}}`, 'g'), String(v));
      });
    }
    
    return template;
  };

  return (
    <LanguageContext.Provider value={{ language, toggleLanguage, t }}>
      {children}
    </LanguageContext.Provider>
  );
}

export function useLanguage() {
  const context = useContext(LanguageContext);
  if (!context) {
    throw new Error('useLanguage must be used within a LanguageProvider');
  }
  return context;
}
