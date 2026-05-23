'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { motion } from 'framer-motion';
import { Shield, Key, ArrowRight } from 'lucide-react';
import { apiService, ApiRuntimeStatus } from '../lib/api';
import { useLanguage } from '../context/LanguageContext';

export default function Home() {
  const router = useRouter();
  const { language, toggleLanguage, t } = useLanguage();
  
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [isDark, setIsDark] = useState(false);
  const [runtimeStatus, setRuntimeStatus] = useState<ApiRuntimeStatus | null>(null);

  useEffect(() => {
    // Check if already authenticated
    const currentUser = apiService.getCurrentUser();
    if (currentUser) {
      router.push(currentUser.role === 'admin' ? '/admin' : '/dashboard');
    }

    const savedTheme = localStorage.getItem('srapi_theme');
    const systemPrefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
    const shouldBeDark = savedTheme === 'dark' || (!savedTheme && systemPrefersDark);

    queueMicrotask(() => setIsDark(shouldBeDark));

    if (shouldBeDark) {
      document.documentElement.classList.add('dark');
    } else {
      document.documentElement.classList.remove('dark');
    }

    apiService.getRuntimeStatus().then(setRuntimeStatus);
  }, [router]);

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

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email || !password) {
      setError(t('loginError'));
      return;
    }

    setIsLoading(true);
    setError('');

    try {
      const user = await apiService.login(email, password);
      // Wait a moment for transition animation
      setTimeout(() => {
        router.push(user.role === 'admin' ? '/admin' : '/dashboard');
      }, 500);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t('authRejected'));
    } finally {
      setIsLoading(false);
    }
  };

  const loadDemoUser = async (role: 'admin' | 'user') => {
    setEmail(role === 'admin' ? 'admin@srapi.local' : 'developer@srapi.local');
    setPassword('password123');
  };

  const isDemoRuntime = runtimeStatus?.mode !== 'live';

  return (
    <div className="min-h-screen flex flex-col md:flex-row bg-srapi-bg text-srapi-text-primary font-sans antialiased transition-colors duration-300 paper-grain relative">
      
      {/* Theme and Language Controls (Top-Right) */}
      <div className="absolute top-6 right-6 z-30 flex items-center gap-4">
        {/* Language switch */}
        <button
          onClick={toggleLanguage}
          className="px-2.5 py-1.5 border border-srapi-border bg-srapi-card-muted hover:bg-srapi-card text-srapi-text-secondary hover:text-srapi-text-primary rounded-xl text-[10px] font-mono tracking-wider transition-all font-bold cursor-pointer"
        >
          {language === 'en' ? '中文' : 'EN'}
        </button>

        {/* Theme toggle */}
        <button 
          onClick={toggleTheme}
          className="relative inline-flex h-5 w-9 shrink-0 cursor-pointer rounded-full border border-srapi-border bg-srapi-card-muted transition-colors focus:outline-none" 
          aria-label="Toggle Theme"
        >
          <span className={`pointer-events-none inline-block h-4 w-4 transform rounded-full bg-srapi-text-primary shadow-sm transition-transform ${isDark ? 'translate-x-4' : 'translate-x-0'}`}></span>
        </button>
      </div>

      {/* Left Column: Academic / Technical Pitch Layout */}
      <div className="flex-1 flex flex-col justify-between p-8 md:p-16 border-b md:border-b-0 md:border-r border-srapi-border bg-srapi-card-muted/20">
        
        {/* Brand */}
        <div className="flex items-center space-x-3.5">
          <a href="#" className="font-serif text-2xl font-medium italic tracking-tight text-srapi-primary">
            SRapi.
          </a>
          <span className="text-[10px] font-mono tracking-wider uppercase text-srapi-text-secondary px-2 py-0.5 border border-srapi-border rounded-full">
            v0.1 Core Studio
          </span>
          <span className={`text-[10px] font-mono tracking-wider uppercase px-2 py-0.5 border rounded-full ${
            isDemoRuntime
              ? 'border-srapi-primary/30 text-srapi-primary bg-srapi-primary/5'
              : 'border-srapi-success/30 text-srapi-success bg-srapi-success/5'
          }`}>
            {isDemoRuntime ? t('demoData') : t('liveApi')}
          </span>
        </div>

        {/* Pitch content */}
        <div className="my-auto py-16 md:py-0 space-y-8 max-w-lg">
          <motion.div
            initial={{ opacity: 0, y: 15 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.6 }}
            className="space-y-4"
          >
            <div className="text-[10px] font-mono tracking-widest uppercase text-srapi-primary font-bold">
              SPECIFICATION PORTAL
            </div>
            <h2 className="font-serif text-3xl md:text-5xl font-normal text-srapi-text-primary leading-[1.15] tracking-tight">
              {language === 'en' ? 'Adaptive dispatch, built for the resilient API gateway.' : '自适应调度分发，为高韧性 API 网关而生。'}
            </h2>
          </motion.div>
          
          <motion.p 
            initial={{ opacity: 0, y: 15 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.6, delay: 0.15 }}
            className="text-xs md:text-sm text-srapi-text-secondary leading-relaxed font-sans"
          >
            {language === 'en' 
              ? 'SRapi balances real-time client traffic demand with active upstream model quotas. Designed as an academic, low-latency LLM router, the kernel scores individual accounts based on dynamic SLA health, queue weights, and cost indices.' 
              : 'SRapi 能够完美平衡实时客户端流量需求与活动上游大模型配额。作为一个学术化、低延迟的 LLM 路由器，内核基于动态 SLA 健康度、队列权重和成本指数为各个账户进行动态评分。'}
          </motion.p>

          {/* Quick specs grid */}
          <motion.div 
            initial={{ opacity: 0, y: 15 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.6, delay: 0.3 }}
            className="grid grid-cols-1 sm:grid-cols-2 gap-6 pt-2"
          >
            <div className="p-5 border border-srapi-border bg-srapi-card rounded-2xl tactile-card space-y-2">
              <Shield size={16} className="text-srapi-primary" />
              <div className="text-xs font-bold text-srapi-text-primary font-serif">
                {language === 'en' ? 'Zero-Trust Mappings' : '零信任映射规程'}
              </div>
              <div className="text-[10px] text-srapi-text-secondary font-mono leading-relaxed">
                {language === 'en' 
                  ? 'Secure credentials vault with write-only authentication tokens and cookie hygiene.' 
                  : '安全的凭证保险库，配备只写认证令牌与严格的 Cookie 安全保护。'}
              </div>
            </div>
            
            <div className="p-5 border border-srapi-border bg-srapi-card rounded-2xl tactile-card space-y-2">
              <Key size={16} className="text-srapi-primary" />
              <div className="text-xs font-bold text-srapi-text-primary font-serif">
                {language === 'en' ? 'Optimal Routing Core' : '最优调度内核'}
              </div>
              <div className="text-[10px] text-srapi-text-secondary font-mono leading-relaxed">
                {language === 'en' 
                  ? 'Real-time candidate weighting filters. Prevent upstreams from hitting concurrency rate-limits.' 
                  : '实时候选者权重过滤机制，防止上游连接触发并发速率限制阀值。'}
              </div>
            </div>
          </motion.div>
        </div>

        {/* Footer */}
        <div className="text-[10px] font-mono text-srapi-text-secondary">
          © 2026 SRapi dispatch team. Academic Self-hosted Console.
        </div>
      </div>

      {/* Right Column: Physical Login Credentials Sheet */}
      <div className="flex-1 flex items-center justify-center p-6 md:p-16">
        <motion.div 
          initial={{ opacity: 0, scale: 0.98 }}
          animate={{ opacity: 1, scale: 1 }}
          transition={{ duration: 0.4 }}
          className="w-full max-w-md bg-srapi-card border border-srapi-border rounded-3xl p-8 md:p-10 space-y-8 shadow-[0_8px_30px_rgba(25,25,25,0.015)] dark:shadow-none tactile-card"
        >
          <div className="space-y-2">
            <h3 className="font-serif font-normal text-2xl tracking-tight text-srapi-text-primary">{t('verifyOperator')}</h3>
            <p className="text-xs text-srapi-text-secondary font-sans leading-relaxed">
              {t('consolePassphraseDesc')}
            </p>
          </div>

          <form onSubmit={handleLogin} className="space-y-5">
            {error && (
              <div className="p-4 border border-srapi-error/20 bg-srapi-error/5 text-srapi-error rounded-2xl text-[10px] font-mono">
                {error}
              </div>
            )}

            <div className="space-y-1.5">
              <label htmlFor="email" className="text-[9px] uppercase font-mono font-bold tracking-wider text-srapi-text-secondary block">
                {t('operatorIdentity')}
              </label>
              <input
                id="email"
                type="email"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="operator@srapi.local"
                className="w-full px-3.5 py-3.5 border border-srapi-border bg-srapi-bg text-srapi-text-primary rounded-xl text-xs outline-none focus:border-srapi-primary transition-all font-mono placeholder:text-srapi-text-secondary/40"
              />
            </div>

            <div className="space-y-1.5">
              <label htmlFor="password" className="text-[9px] uppercase font-mono font-bold tracking-wider text-srapi-text-secondary block">
                {t('consolePassphrase')}
              </label>
              <input
                id="password"
                type="password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••••••"
                className="w-full px-3.5 py-3.5 border border-srapi-border bg-srapi-bg text-srapi-text-primary rounded-xl text-xs outline-none focus:border-srapi-primary transition-all font-mono placeholder:text-srapi-text-secondary/40"
              />
            </div>

            <button
              id="login-submit"
              type="submit"
              disabled={isLoading}
              className="w-full bg-[#191919] hover:bg-neutral-800 dark:bg-[#F1EFEA] dark:hover:bg-white text-white dark:text-[#111110] text-xs font-mono tracking-widest uppercase py-4 rounded-full transition-all active:scale-[0.96] mt-4 font-bold border border-neutral-800 dark:border-white shadow-md disabled:opacity-40 disabled:cursor-not-allowed flex items-center justify-center gap-2 cursor-pointer"
            >
              {isLoading ? t('decrypting') : t('authenticate')}
              <ArrowRight size={14} />
            </button>
          </form>

          {/* Quick-Access Test Accounts for local testing */}
          <div className="pt-6 border-t border-srapi-border space-y-4">
            <span className="text-[9px] uppercase font-mono tracking-wider text-srapi-text-secondary font-bold block">
              {t('quickTest')}
            </span>
            <div className="flex gap-4">
              <button
                type="button"
                onClick={() => loadDemoUser('admin')}
                className="flex-1 py-3 px-4 border border-srapi-border bg-srapi-card-muted/50 hover:bg-srapi-card-muted text-[10px] font-bold text-srapi-text-primary rounded-xl transition-all cursor-pointer font-mono text-center block hover:border-srapi-primary/40 active:scale-[0.97]"
              >
                ● {t('adminAccount')}
              </button>
              <button
                type="button"
                onClick={() => loadDemoUser('user')}
                className="flex-1 py-3 px-4 border border-srapi-border bg-srapi-card-muted/50 hover:bg-srapi-card-muted text-[10px] font-bold text-srapi-text-primary rounded-xl transition-all cursor-pointer font-mono text-center block hover:border-srapi-primary/40 active:scale-[0.97]"
              >
                ■ {t('devAccount')}
              </button>
            </div>
          </div>
        </motion.div>
      </div>

    </div>
  );
}
