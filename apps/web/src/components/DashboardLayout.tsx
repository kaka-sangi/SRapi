'use client';

import { ReactNode, useState, useEffect } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import { motion, AnimatePresence } from 'framer-motion';
import { 
  LogOut, 
  AlertTriangle,
  CheckCircle,
  X,
  UserCheck,
  Server
} from 'lucide-react';
import { apiService, ApiRuntimeStatus } from '../lib/api';
import { SmokeChecklist } from '../lib/mockData';
import { useLanguage } from '../context/LanguageContext';

interface DashboardLayoutProps {
  children: ReactNode;
  allowedRole?: 'admin' | 'user';
}

export default function DashboardLayout({ children, allowedRole }: DashboardLayoutProps) {
  const router = useRouter();
  const pathname = usePathname();
  const { language, toggleLanguage, t } = useLanguage();
  
  const [user, setUser] = useState<ReturnType<typeof apiService.getCurrentUser>>(null);
  const [isDark, setIsDark] = useState(false);
  const [smokeStatus, setSmokeStatus] = useState<SmokeChecklist | null>(null);
  const [showSmokePanel, setShowSmokePanel] = useState(false);
  const [isLoading, setIsLoading] = useState(true);
  const [runtimeStatus, setRuntimeStatus] = useState<ApiRuntimeStatus | null>(null);

  // Initialize theme and user state
  useEffect(() => {
    const currentUser = apiService.getCurrentUser();
    if (!currentUser) {
      router.push('/');
      return;
    }
    
    if (allowedRole && currentUser.role !== allowedRole) {
      router.push(currentUser.role === 'admin' ? '/admin' : '/dashboard');
      return;
    }

    queueMicrotask(() => {
      setUser(currentUser);
      setIsLoading(false);
    });

    const savedTheme = localStorage.getItem('srapi_theme');
    const systemPrefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    const shouldBeDark = savedTheme === 'dark' || (!savedTheme && systemPrefersDark);

    queueMicrotask(() => setIsDark(shouldBeDark));

    if (shouldBeDark) {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }

    Promise.all([
      apiService.getRuntimeStatus(),
      apiService.getSmokeStatus()
    ]).then(([runtime, status]) => {
      setRuntimeStatus(runtime);
      setSmokeStatus(status);
    });
  }, [router, allowedRole]);

  // Toggle Theme
  const toggleTheme = () => {
    const nextDark = !isDark;
    setIsDark(nextDark);
    if (nextDark) {
      document.documentElement.classList.add('dark');
      localStorage.setItem('srapi_theme', 'dark');
    } else {
      document.documentElement.classList.remove('dark');
      localStorage.setItem('srapi_theme', 'light');
    }
  };

  const handleLogout = async () => {
    await apiService.logout();
    router.push('/');
  };

  const handleRoleSwitch = async () => {
    if (!user || user.authMode !== 'demo') {
      return;
    }

    const newRole = user.role === 'admin' ? 'user' : 'admin';
    const targetEmail = newRole === 'admin' ? 'admin@srapi.local' : 'developer@srapi.local';
    const loggedIn = await apiService.login(targetEmail, 'password123');
    setUser(loggedIn);
    router.push(newRole === 'admin' ? '/admin' : '/dashboard');
  };

  if (isLoading || !user) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-srapi-bg">
        <div className="text-center font-mono">
          <div className="w-8 h-8 border-t-2 border-srapi-primary rounded-full animate-spin mx-auto mb-4"></div>
          <p className="text-xs text-srapi-text-secondary">{t('authenticating')}</p>
        </div>
      </div>
    );
  }

  // Navigation configurations
  const userNavigation = [
    { name: 'Overview', href: '/dashboard' },
    { name: 'API Keys', href: '/api-keys' },
    { name: 'Usage History', href: '/usage' },
  ];

  const adminNavigation = [
    { name: 'Overview', href: '/admin' },
    { name: 'Provider Accounts', href: '/provider-accounts' },
    { name: 'Scheduler Decisions', href: '/scheduler-decisions' },
    { name: 'Usage Logs', href: '/usage' },
  ];

  const navigation = user.role === 'admin' ? adminNavigation : userNavigation;

  const getNavTranslationKey = (name: string) => {
    switch (name) {
      case 'Overview': return t('navOverview');
      case 'API Keys': return t('navApiKeys');
      case 'Usage History': return t('navUsageHistory');
      case 'Usage Logs': return t('navUsageLogs');
      case 'Provider Accounts': return t('navProviderAccounts');
      case 'Scheduler Decisions': return t('navSchedulerDecisions');
      default: return name;
    }
  };

  // Header content details mapping (Academic journal style)
  const getHeaderMeta = () => {
    switch (pathname) {
      case '/dashboard':
        return {
          category: t('devCat'),
          title: t('devTitle'),
          desc: t('devDesc')
        };
      case '/admin':
        return {
          category: t('adminCat'),
          title: t('adminTitle'),
          desc: t('adminDesc')
        };
      case '/api-keys':
        return {
          category: t('apiCat'),
          title: t('apiTitle'),
          desc: t('apiDesc')
        };
      case '/usage':
        return {
          category: t('usageCat'),
          title: t('usageTitle'),
          desc: t('usageDesc')
        };
      case '/provider-accounts':
        return {
          category: t('provCat'),
          title: t('provTitle'),
          desc: t('provDesc')
        };
      case '/scheduler-decisions':
        return {
          category: t('schedCat'),
          title: t('schedTitle'),
          desc: t('schedDesc')
        };
      default:
        return {
          category: 'SRapi Gateway',
          title: 'Management Studio Console',
          desc: 'Management and observation interface for the self-hosted intelligent LLM routing gateway.'
        };
    }
  };

  const meta = getHeaderMeta();
  const isDemoRuntime = user.authMode === 'demo' || runtimeStatus?.mode === 'demo';

  return (
    <div className="min-h-screen bg-srapi-bg text-srapi-text-primary font-sans antialiased pb-24 paper-grain relative">
      
      {/* Editorial Header */}
      <header className="border-b border-srapi-border bg-srapi-bg/85 backdrop-blur-md sticky top-0 z-40 animate-bloom">
        <div className="max-w-6xl mx-auto px-6 md:px-8 h-20 flex items-center justify-between">
          <div className="flex items-center space-x-4">
            <a href={user.role === 'admin' ? '/admin' : '/dashboard'} className="font-serif text-xl font-medium italic tracking-tight text-srapi-primary">
              SRapi.
            </a>
            <span className="text-[10px] font-mono tracking-wider uppercase text-srapi-text-secondary px-2.5 py-0.5 border border-srapi-border rounded-full">
              {user.role === 'admin' ? t('operatorConsole') : t('developerConsole')}
            </span>
            <span className={`hidden md:inline-flex text-[10px] font-mono tracking-wider uppercase px-2.5 py-0.5 border rounded-full items-center gap-1.5 ${
              isDemoRuntime
                ? 'border-srapi-primary/30 text-srapi-primary bg-srapi-primary/5'
                : 'border-srapi-success/30 text-srapi-success bg-srapi-success/5'
            }`} title={runtimeStatus?.apiBaseUrl || ''}>
              <Server size={11} />
              {isDemoRuntime ? t('demoData') : t('liveApi')}
            </span>
          </div>

          <div className="flex items-center space-x-6 md:space-x-8">
            <nav className="flex space-x-6 md:space-x-8 text-xs font-mono tracking-widest uppercase text-srapi-text-secondary">
              {navigation.map((item) => {
                const isActive = pathname === item.href;
                return (
                  <a
                    key={item.name}
                    href={item.href}
                    className={`hover:text-srapi-primary transition-colors ${isActive ? 'text-srapi-primary font-bold border-b border-srapi-primary pb-0.5' : ''}`}
                  >
                    {getNavTranslationKey(item.name)}
                  </a>
                );
              })}
            </nav>

            <div className="flex items-center space-x-4 md:space-x-6 border-l border-srapi-border pl-4 md:pl-6">
              
              {/* Language Switch Button */}
              <button
                onClick={toggleLanguage}
                className="px-2.5 py-1.5 border border-srapi-border hover:bg-srapi-card-muted text-srapi-text-secondary hover:text-srapi-text-primary rounded-xl text-[10px] font-mono tracking-wider transition-all font-bold cursor-pointer"
                title="Switch Console Language / 中英文切换"
              >
                {language === 'en' ? '中文' : 'EN'}
              </button>

              {/* Role Quick-Switch */}
              {user.authMode === 'demo' && (
                <button
                  onClick={handleRoleSwitch}
                  className="p-1.5 border border-srapi-border hover:bg-srapi-card-muted text-srapi-text-secondary hover:text-srapi-text-primary rounded-full transition-colors hidden sm:block cursor-pointer"
                  title={t('demoOnlySwitch')}
                >
                  <UserCheck size={14} />
                </button>
              )}

              {/* Theme Toggle (Tactile slider exactly matching preview.html) */}
              <button 
                onClick={toggleTheme}
                className="relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border border-srapi-border bg-srapi-card-muted transition-colors focus:outline-none" 
                aria-label="Toggle Theme"
              >
                <span className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-srapi-text-primary shadow-sm transition-transform ${isDark ? 'translate-x-4' : 'translate-x-0'}`}></span>
              </button>

              {/* Terminal-style Exit */}
              <button
                onClick={handleLogout}
                className="p-1.5 border border-srapi-error/30 hover:bg-srapi-error/5 text-srapi-error rounded-full transition-colors cursor-pointer"
                title={t('terminateSession')}
              >
                <LogOut size={14} />
              </button>
            </div>
          </div>
        </div>
      </header>

      {/* Main Journal Workspace */}
      <main className="max-w-6xl mx-auto px-6 md:px-8 mt-12 md:mt-16">
        
        {/* Academic Journal Title Block */}
        <div className="border-b border-srapi-border pb-8 mb-12 animate-bloom delay-100 flex flex-col md:flex-row md:items-end justify-between gap-6">
          <div className="space-y-2.5 max-w-3xl">
            <div className="text-[10px] font-mono tracking-widest uppercase text-srapi-primary font-bold">
              {meta.category}
            </div>
            <h2 className="font-serif text-3xl md:text-4xl font-normal tracking-tight leading-tight text-srapi-text-primary">
              {meta.title}
            </h2>
            <p className="text-xs text-srapi-text-secondary leading-relaxed font-sans max-w-2xl">
              {meta.desc}
            </p>
          </div>

          <div className="flex flex-col items-start md:items-end gap-3 shrink-0">
            {/* User credentials summary */}
            <div className="text-right text-[10px] font-mono bg-srapi-card-muted/50 border border-srapi-border px-3 py-1.5 rounded-xl space-y-0.5">
              <div>{t('operatorName')}: <span className="font-bold text-srapi-text-primary">{user.name}</span></div>
              <div>{t('availableBalance')}: <span className="font-bold text-srapi-primary">${user.balance} USD</span></div>
            </div>

            {/* Smoke test badge */}
            {smokeStatus && (
              <button
                onClick={() => setShowSmokePanel(true)}
                className={`quiet-badge transition-all hover:bg-srapi-card-muted rounded-full cursor-pointer ${
                  smokeStatus.v0_1_smoke_evidence_complete
                    ? 'border-srapi-success text-srapi-success bg-srapi-success/5 font-semibold'
                    : 'border-srapi-error text-srapi-error bg-srapi-error/5 hover:border-srapi-error/60'
                }`}
              >
                <span className={`w-1.5 h-1.5 rounded-full ${
                  smokeStatus.v0_1_smoke_evidence_complete ? 'bg-srapi-success animate-pulse' : 'bg-srapi-error'
                }`}></span>
                {t('smokeEvidence')}: {smokeStatus.v0_1_smoke_evidence_complete ? t('complete') : t('notComplete')}
              </button>
            )}
          </div>
        </div>

        {/* Dynamic Route Content */}
        <div className="animate-bloom delay-200">
          {children}
        </div>

      </main>

      {/* v0.1 Smoke Evidence Detail Drawer */}
      <AnimatePresence>
        {showSmokePanel && smokeStatus && (
          <>
            {/* Backdrop */}
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 0.4 }}
              exit={{ opacity: 0 }}
              onClick={() => setShowSmokePanel(false)}
              className="fixed inset-0 bg-black z-50 animate-fade-in"
            />
            {/* Drawer */}
            <motion.div
              initial={{ x: '100%' }}
              animate={{ x: 0 }}
              exit={{ x: '100%' }}
              transition={{ type: 'spring', damping: 20, stiffness: 100 }}
              className="fixed right-0 top-0 bottom-0 w-full max-w-lg bg-srapi-card border-l border-srapi-border p-6 md:p-8 z-50 overflow-y-auto paper-grain shadow-2xl space-y-6"
            >
              <div className="flex items-center justify-between border-b border-srapi-border pb-4">
                <div>
                  <h3 className="font-serif font-bold text-lg tracking-tight">{t('smokeDrawerTitle')}</h3>
                  <p className="text-xs text-srapi-text-secondary font-mono mt-0.5">{smokeStatus.base_url}</p>
                </div>
                <button
                  onClick={() => setShowSmokePanel(false)}
                  className="p-1.5 border border-srapi-border hover:bg-srapi-card-muted rounded-full cursor-pointer"
                >
                  <X size={16} />
                </button>
              </div>

              {/* Status Banner */}
              <div className={`p-5 border rounded-2xl flex items-start gap-3.5 ${
                smokeStatus.v0_1_smoke_evidence_complete
                  ? 'bg-srapi-success/5 border-srapi-success/30 text-srapi-success'
                  : 'bg-srapi-error/5 border-srapi-error/30 text-srapi-error'
              }`}>
                {smokeStatus.v0_1_smoke_evidence_complete ? (
                  <CheckCircle size={20} className="mt-0.5 flex-shrink-0" />
                ) : (
                  <AlertTriangle size={20} className="mt-0.5 flex-shrink-0" />
                )}
                <div className="space-y-1">
                  <h4 className="font-semibold text-sm">
                    {t('statusTitle')}: {smokeStatus.v0_1_smoke_evidence_complete ? t('completeState') : t('notComplete')}
                  </h4>
                  <p className="text-xs text-srapi-text-secondary leading-relaxed font-sans">
                    {smokeStatus.v0_1_smoke_evidence_complete
                      ? t('smokeDesc')
                      : t('smokeIncomplete')}
                  </p>
                </div>
              </div>

              {/* Conditions List */}
              <div className="space-y-5">
                <span className="text-[10px] uppercase font-mono tracking-wider text-srapi-text-secondary font-bold block border-b border-srapi-border pb-2">
                  {t('constraintsMatrix')}
                </span>
                
                <div className="space-y-4">
                  {/* Model */}
                  <div className="flex items-start justify-between">
                    <div className="text-xs">
                      <p className="font-semibold">{t('modelEntryVerif')}</p>
                      <p className="text-[10px] text-srapi-text-secondary mt-0.5">{t('modelRegistered', { model: smokeStatus.model })}</p>
                    </div>
                    <span className={`quiet-badge ${smokeStatus.model_exists ? 'border-srapi-success text-srapi-success' : 'border-srapi-error text-srapi-error'}`}>
                      {smokeStatus.model_exists ? t('exists') : t('missing')}
                    </span>
                  </div>

                  {/* Upstream Account */}
                  <div className="flex items-start justify-between">
                    <div className="text-xs">
                      <p className="font-semibold">{t('publicUpstreamAcc')}</p>
                      <p className="text-[10px] text-srapi-text-secondary mt-0.5">{t('requiresPublic')}</p>
                    </div>
                    <div className="flex flex-col items-end gap-1 font-mono text-[10px]">
                      <span className={`quiet-badge ${smokeStatus.public_https_upstream_account_count > 0 ? 'border-srapi-success text-srapi-success bg-srapi-success/5' : 'border-srapi-error text-srapi-error'}`}>
                        {t('activeAccounts', { count: smokeStatus.public_https_upstream_account_count })}
                      </span>
                      <span className="text-[8px] text-srapi-text-secondary">({smokeStatus.active_account_count} active accounts)</span>
                    </div>
                  </div>

                  {/* Traffic */}
                  <div className="space-y-2 border-t border-srapi-border/40 pt-3">
                    <div className="flex items-center justify-between text-xs">
                      <span className="font-semibold">{t('healthyTrafficRegistry')}</span>
                      <span className={`quiet-badge ${smokeStatus.missing_usage_endpoints.length === 0 ? 'border-srapi-success text-srapi-success bg-srapi-success/5' : 'border-srapi-error text-srapi-error bg-srapi-error/5'}`}>
                        {smokeStatus.missing_usage_endpoints.length === 0 ? t('completeState') : t('incompleteState')}
                      </span>
                    </div>
                    <p className="text-[10px] text-srapi-text-secondary">{t('loggedHealthy')}</p>
                    <div className="grid grid-cols-3 gap-2 mt-1">
                      {['/v1/chat/completions', '/v1/responses', '/v1/messages'].map((ep) => {
                        const hasTraffic = smokeStatus.usage_endpoints.includes(ep);
                        return (
                          <div key={ep} className={`p-2 border text-center font-mono text-[9px] rounded-lg ${
                            hasTraffic ? 'border-srapi-success/30 bg-srapi-success/5 text-srapi-success' : 'border-srapi-border text-srapi-text-secondary bg-srapi-card-muted/40'
                          }`}>
                            {ep.replace('/v1', '')}
                          </div>
                        );
                      })}
                    </div>
                  </div>

                  {/* Decisions */}
                  <div className="space-y-2 border-t border-srapi-border/40 pt-3">
                    <div className="flex items-center justify-between text-xs">
                      <span className="font-semibold">{t('schedulerRouting')}</span>
                      <span className={`quiet-badge ${smokeStatus.missing_real_upstream_scheduler_decision_endpoints.length === 0 ? 'border-srapi-success text-srapi-success bg-srapi-success/5' : 'border-srapi-error text-srapi-error bg-srapi-error/5'}`}>
                        {smokeStatus.missing_real_upstream_scheduler_decision_endpoints.length === 0 ? t('completeState') : t('incompleteState')}
                      </span>
                    </div>
                    <p className="text-[10px] text-srapi-text-secondary">{t('decisionsRouting')}</p>
                    <div className="grid grid-cols-3 gap-2 mt-1">
                      {['/v1/chat/completions', '/v1/responses', '/v1/messages'].map((ep) => {
                        const hasDecision = smokeStatus.real_upstream_scheduler_decision_endpoints.includes(ep);
                        return (
                          <div key={ep} className={`p-2 border text-center font-mono text-[9px] rounded-lg ${
                            hasDecision ? 'border-srapi-success/30 bg-srapi-success/5 text-srapi-success' : 'border-srapi-border text-srapi-text-secondary bg-srapi-card-muted/40'
                          }`}>
                            {ep.replace('/v1', '')}
                          </div>
                        );
                      })}
                    </div>
                  </div>
                </div>

                <div className="p-4 bg-srapi-card-muted border border-srapi-border rounded-2xl text-[10px] font-mono text-srapi-text-secondary space-y-2">
                  <p className="font-bold text-xs text-srapi-text-primary">{t('diagnosticInstructions')}</p>
                  <p>{t('instr1')}</p>
                  <p>{t('instr2')}</p>
                  <p>{t('instr3')}</p>
                  <pre className="p-2 bg-srapi-card border border-srapi-border rounded-lg text-[9px] text-srapi-text-primary overflow-x-auto mt-2">
                    node tools/srapi-admin.mjs smoke-status
                  </pre>
                </div>
              </div>
            </motion.div>
          </>
        )}
      </AnimatePresence>

    </div>
  );
}
