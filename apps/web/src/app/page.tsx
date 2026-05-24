'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import { Shield, Key, ArrowRight } from 'lucide-react';
import { apiService, ApiRuntimeStatus } from '../lib/api';
import { useLanguage } from '../context/LanguageContext';
import { homeRouteForRole } from '@/lib/routes';

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
      router.push(homeRouteForRole(currentUser.role));
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
      router.push(homeRouteForRole(user.role));
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t('authRejected'));
    } finally {
      setIsLoading(false);
    }
  };

  const isApiOffline = runtimeStatus && !runtimeStatus.connected;

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
            v0.1.0
          </span>
          <span className={`text-[10px] font-mono tracking-wider uppercase px-2 py-0.5 border rounded-full ${
            isApiOffline
              ? 'border-srapi-error/30 text-srapi-error bg-srapi-error/5'
              : 'border-srapi-success/30 text-srapi-success bg-srapi-success/5'
          }`}>
            {isApiOffline ? t('apiOffline') : t('liveApi')}
          </span>
        </div>

        {/* Pitch content. CSS-driven bloom keeps first paint visible even
            if JS is slow or fails. */}
        <div className="my-auto py-16 md:py-0 space-y-8 max-w-lg">
          <div className="space-y-4 animate-bloom">
            <div className="text-[10px] font-mono tracking-widest uppercase text-srapi-primary font-bold">
              {language === 'en' ? 'SELF-HOSTED AI GATEWAY' : '自托管 AI 网关'}
            </div>
            <h2 className="font-serif text-3xl md:text-5xl font-normal text-srapi-text-primary leading-[1.15] tracking-tight">
              {language === 'en'
                ? 'One endpoint, every provider, your accounts, your control.'
                : '一个入口，接入所有服务商，账号自管，调度可控。'}
            </h2>
          </div>

          <p className="text-xs md:text-sm text-srapi-text-secondary leading-relaxed font-sans animate-bloom delay-100">
            {language === 'en'
              ? 'SRapi routes OpenAI, Anthropic, Gemini and CLI / web-session accounts through one OpenAI-compatible endpoint. Bring your own provider accounts, set quotas and rate limits, and the built-in scheduler picks the best account for every request.'
              : 'SRapi 把 OpenAI、Anthropic、Gemini 以及 CLI / 反代账号统一在一个 OpenAI 兼容的接口之后。你接入自己的上游账号，设置配额与限速，内置调度器为每一笔请求挑选最合适的账号。'}
          </p>

          {/* Highlights */}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-6 pt-2 animate-bloom delay-200">
            <div className="p-5 border border-srapi-border bg-srapi-card rounded-2xl tactile-card space-y-2">
              <Shield size={16} className="text-srapi-primary" aria-hidden="true" />
              <div className="text-xs font-bold text-srapi-text-primary font-serif">
                {language === 'en' ? 'Your accounts stay yours' : '账号始终在你手里'}
              </div>
              <div className="text-[10px] text-srapi-text-secondary font-mono leading-relaxed">
                {language === 'en'
                  ? 'Self-hosted. Provider credentials are encrypted at rest and never returned by the admin API.'
                  : '自托管部署。上游凭据落库前加密，管理 API 永不回显明文。'}
              </div>
            </div>

            <div className="p-5 border border-srapi-border bg-srapi-card rounded-2xl tactile-card space-y-2">
              <Key size={16} className="text-srapi-primary" aria-hidden="true" />
              <div className="text-xs font-bold text-srapi-text-primary font-serif">
                {language === 'en' ? 'Smart, transparent routing' : '调度可解释、可观测'}
              </div>
              <div className="text-[10px] text-srapi-text-secondary font-mono leading-relaxed">
                {language === 'en'
                  ? 'The scheduler weighs health, quota, cost and session affinity, and shows you exactly why each account was picked.'
                  : '调度器同时考虑健康度、配额、成本与会话亲和，并清晰呈现每一次为什么选中这个上游。'}
              </div>
            </div>
          </div>
        </div>

        {/* Footer */}
        <div className="text-[10px] font-mono text-srapi-text-secondary">
          {language === 'en'
            ? '© 2026 SRapi · Self-hosted AI gateway · v0.1.0'
            : '© 2026 SRapi · 自托管 AI 网关 · v0.1.0'}
        </div>
      </div>

      {/* Right Column: Physical Login Credentials Sheet */}
      <div className="flex-1 flex items-center justify-center p-6 md:p-16">
        <div
          className="w-full max-w-md bg-srapi-card border border-srapi-border rounded-3xl p-8 md:p-10 space-y-8 shadow-[0_8px_30px_rgba(25,25,25,0.015)] dark:shadow-none tactile-card animate-bloom-soft"
        >
          <div className="space-y-2">
            <h3 className="font-serif font-normal text-2xl tracking-tight text-srapi-text-primary">{t('verifyOperator')}</h3>
            <p className="text-xs text-srapi-text-secondary font-sans leading-relaxed">
              {t('consolePassphraseDesc')}
            </p>
          </div>

          <form method="post" onSubmit={handleLogin} className="space-y-5">
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
                name="email"
                type="email"
                required
                autoComplete="username"
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
                name="password"
                type="password"
                required
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••••••"
                className="w-full px-3.5 py-3.5 border border-srapi-border bg-srapi-bg text-srapi-text-primary rounded-xl text-xs outline-none focus:border-srapi-primary transition-all font-mono placeholder:text-srapi-text-secondary/40"
              />
            </div>

            <button
              id="login-submit"
              type="submit"
              disabled={isLoading || runtimeStatus === null}
              className="w-full bg-[#191919] hover:bg-neutral-800 dark:bg-[#F1EFEA] dark:hover:bg-white text-white dark:text-[#111110] text-xs font-mono tracking-widest uppercase py-4 rounded-full transition-all active:scale-[0.96] mt-4 font-bold border border-neutral-800 dark:border-white shadow-md disabled:opacity-40 disabled:cursor-not-allowed flex items-center justify-center gap-2 cursor-pointer"
            >
              {isLoading || runtimeStatus === null ? t('decrypting') : t('authenticate')}
              <ArrowRight size={14} />
            </button>
          </form>
        </div>
      </div>

    </div>
  );
}
