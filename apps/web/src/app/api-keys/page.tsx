'use client';

import React, { useCallback, useState, useEffect } from 'react';
import { 
  Key, 
  Plus, 
  Copy, 
  Check, 
  AlertCircle, 
  Power, 
  Sparkles,
  X
} from 'lucide-react';
import DashboardLayout from '../../components/DashboardLayout';
import { apiService } from '../../lib/api';
import { MockApiKey } from '../../lib/mockData';
import { useLanguage } from '../../context/LanguageContext';

export default function ApiKeysPage() {
  const { language, t } = useLanguage();
  const [keys, setKeys] = useState<MockApiKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [copiedId, setCopiedId] = useState<string | null>(null);
  
  // Creation Form State
  const [name, setName] = useState('');
  const [selectedModels, setSelectedModels] = useState<string[]>(['gpt-4o-mini']);
  const [groupIds, setGroupIds] = useState('group-01');
  const [isCreating, setIsCreating] = useState(false);
  const [showCreateModal, setShowCreateModal] = useState(false);
  
  // Single-display key result
  const [generatedKey, setGeneratedKey] = useState<MockApiKey | null>(null);
  const [copiedPlaintext, setCopiedPlaintext] = useState(false);

  const loadKeys = useCallback(async () => {
    setLoading(true);
    try {
      const data = await apiService.listApiKeys();
      setKeys(data);
    } catch (err) {
      console.error('Failed to load API keys', err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    queueMicrotask(loadKeys);
  }, [loadKeys]);

  const handleCopy = (text: string, id: string) => {
    navigator.clipboard.writeText(text);
    setCopiedId(id);
    setTimeout(() => setCopiedId(null), 1500);
  };

  const handleCopyPlaintext = (text: string) => {
    navigator.clipboard.writeText(text);
    setCopiedPlaintext(true);
    setTimeout(() => setCopiedPlaintext(false), 1500);
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name) return;
    
    setIsCreating(true);
    try {
      const newKey = await apiService.createApiKey(
        name,
        selectedModels,
        groupIds.split(',').map(g => g.trim()).filter(Boolean)
      );
      setGeneratedKey(newKey);
      setName('');
      setSelectedModels(['gpt-4o-mini']);
      setGroupIds('group-01');
      setShowCreateModal(false);
      await loadKeys();
    } catch (err) {
      console.error('Failed to create key', err);
    } finally {
      setIsCreating(false);
    }
  };

  const handleToggleStatus = async (id: string, currentStatus: 'active' | 'disabled') => {
    try {
      const updated = await apiService.toggleApiKeyStatus(id, currentStatus);
      if (updated) {
        setKeys(prev => prev.map(k => k.id === id ? { ...k, status: updated.status } : k));
      }
    } catch (err) {
      console.error('Failed to toggle status', err);
    }
  };

  const handleModelToggle = (model: string) => {
    setSelectedModels(prev => 
      prev.includes(model) 
        ? prev.filter(m => m !== model) 
        : [...prev, model]
    );
  };

  // Localized string values
  const textAccessCredentials = language === 'en' ? 'Access Control Credentials' : '访问控制安全凭证';
  const textAccessDesc = language === 'en' 
    ? 'Generate secure, scoped tokens to authenticate your client applications with the SRapi scheduling engine.' 
    : '生成作用域安全令牌，以授权您的客户端应用程序调用 SRapi 自适应调度分发网关。';
  const textActiveUpper = language === 'en' ? 'ACTIVE' : '启用';
  const textDisabledUpper = language === 'en' ? 'DISABLED' : '已撤销';

  return (
    <DashboardLayout>
      <div className="space-y-8 animate-bloom">
        
        {/* Top Operational Header (rounded-2xl) */}
        <div className="bg-srapi-card border border-srapi-border rounded-2xl p-6 flex flex-col sm:flex-row sm:items-center justify-between gap-6 tactile-card">
          <div className="space-y-1">
            <h3 className="font-serif font-medium text-lg tracking-tight">{textAccessCredentials}</h3>
            <p className="text-xs text-srapi-text-secondary leading-relaxed">{textAccessDesc}</p>
          </div>
          <button
            onClick={() => {
              setGeneratedKey(null);
              setShowCreateModal(true);
            }}
            className="px-5 py-3.5 bg-srapi-text-primary text-srapi-bg dark:bg-srapi-text-primary dark:text-srapi-bg hover:bg-transparent hover:text-srapi-text-primary dark:hover:bg-transparent dark:hover:text-srapi-text-primary border border-srapi-text-primary text-xs font-mono tracking-wider uppercase rounded-full transition-all active:scale-[0.96] font-bold flex items-center justify-center gap-1.5 shrink-0 cursor-pointer"
          >
            <Plus size={14} />
            {t('generateKey')}
          </button>
        </div>

        {/* Secure Plaintext Single-Show Container (rounded-2xl) */}
        {generatedKey && generatedKey.plaintextKey && (
          <div className="p-6 border border-srapi-primary/30 bg-srapi-primary/5 rounded-2xl space-y-4 animate-bloom relative">
            <div className="flex items-start gap-3.5">
              <AlertCircle className="text-srapi-primary mt-0.5 flex-shrink-0" size={18} />
              <div className="space-y-1">
                <h4 className="text-xs font-extrabold text-srapi-primary font-mono uppercase tracking-wider">{t('secretKeyGenerated')}</h4>
                <p className="text-xs text-srapi-text-secondary leading-relaxed font-sans">
                  {t('keyWarning')}
                </p>
              </div>
            </div>

            <div className="flex flex-col sm:flex-row items-stretch sm:items-center gap-3">
              <code className="flex-grow p-3.5 bg-srapi-card border border-srapi-border rounded-xl text-xs font-mono font-bold text-srapi-text-primary overflow-x-auto select-all block">
                {generatedKey.plaintextKey}
              </code>
              <button
                onClick={() => handleCopyPlaintext(generatedKey.plaintextKey!)}
                className="px-5 py-3.5 bg-srapi-primary hover:bg-srapi-primary-hover text-white rounded-xl transition-all cursor-pointer flex items-center justify-center gap-1.5 font-mono text-xs font-bold shrink-0"
              >
                {copiedPlaintext ? <Check size={14} /> : <Copy size={14} />}
                {copiedPlaintext ? t('copiedClipboard') : t('copyPlaintext')}
              </button>
            </div>
          </div>
        )}

        {/* API Keys Table (rounded-3xl card with tactile feel) */}
        <div className="bg-srapi-card border border-srapi-border rounded-3xl p-6 space-y-5 tactile-card">
          <h4 className="font-serif text-lg italic text-srapi-text-primary">{t('activeChannels')}</h4>

          {loading ? (
            <div className="py-12 text-center font-mono">
              <div className="w-6 h-6 border-t-2 border-srapi-primary rounded-full animate-spin mx-auto mb-3"></div>
              <p className="text-xs text-srapi-text-secondary">{t('queryRegistry')}</p>
            </div>
          ) : keys.length === 0 ? (
            <div className="py-16 border border-dashed border-srapi-border rounded-2xl text-center space-y-3.5">
              <Key className="mx-auto text-srapi-text-secondary opacity-40" size={28} />
              <p className="text-xs font-bold text-srapi-text-primary font-serif">{t('noKeys')}</p>
              <p className="text-[10px] text-srapi-text-secondary font-mono">{t('noKeysDesc')}</p>
            </div>
          ) : (
            <div className="overflow-x-auto scrollbar-none border border-srapi-border rounded-2xl shadow-[0_4px_20px_rgba(25,25,25,0.015)] dark:shadow-none bg-srapi-card">
              <table className="w-full text-left border-collapse text-xs min-w-[700px]">
                <thead>
                  <tr className="bg-srapi-card-muted/65 border-b border-srapi-border font-mono text-srapi-text-secondary text-[10px] uppercase tracking-wider">
                    <th className="py-4 px-6 font-medium">{t('keyName')}</th>
                    <th className="py-4 px-6 font-medium">{t('prefix')}</th>
                    <th className="py-4 px-6 font-medium">{t('allowedModels')}</th>
                    <th className="py-4 px-6 font-medium">{t('status')}</th>
                    <th className="py-4 px-6 font-medium">{t('created')}</th>
                    <th className="py-4 px-6 font-medium text-right">{t('actions')}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-srapi-border font-mono text-[11px]">
                  {keys.map((k) => (
                    <tr key={k.id} className="hover:bg-srapi-card-muted/20 transition-colors">
                      <td className="py-4.5 px-6 whitespace-nowrap font-sans font-bold text-srapi-text-primary">
                        {k.name}
                      </td>
                      <td className="py-4.5 px-6 whitespace-nowrap">
                        <div className="flex items-center gap-2">
                          <code className="bg-srapi-card-muted px-2 py-0.5 border border-srapi-border rounded text-srapi-text-secondary text-[10px]">
                            {k.prefix}
                          </code>
                          <button
                            onClick={() => handleCopy(k.prefix, k.id)}
                            className="p-1 hover:bg-srapi-card-muted rounded border border-transparent hover:border-srapi-border transition-all text-srapi-text-secondary hover:text-srapi-text-primary cursor-pointer"
                          >
                            {copiedId === k.id ? <Check size={12} className="text-green-700" /> : <Copy size={12} />}
                          </button>
                        </div>
                      </td>
                      <td className="py-4.5 px-6">
                        <div className="flex flex-wrap gap-1 max-w-[240px]">
                          {k.allowed_models.map(m => (
                            <span key={m} className="px-1.5 py-0.5 bg-srapi-card-muted border border-srapi-border rounded text-[9px] font-bold text-srapi-text-secondary">
                              {m}
                            </span>
                          ))}
                        </div>
                      </td>
                      <td className="py-4.5 px-6 whitespace-nowrap">
                        <span className={`text-[10px] font-bold border px-2.5 py-0.5 rounded-full ${
                          k.status === 'active' 
                            ? 'border-green-500/20 text-green-700 dark:text-green-500 bg-green-500/10' 
                            : 'border-srapi-border text-srapi-text-secondary bg-srapi-card-muted'
                        }`}>
                          {k.status === 'active' ? textActiveUpper : textDisabledUpper}
                        </span>
                      </td>
                      <td className="py-4.5 px-6 whitespace-nowrap text-srapi-text-secondary">
                        {new Date(k.created_at).toLocaleDateString()} {new Date(k.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                      </td>
                      <td className="py-4.5 px-6 text-right whitespace-nowrap">
                        <button
                          onClick={() => handleToggleStatus(k.id, k.status)}
                          className={`px-3 py-1.5 border rounded-lg transition-all cursor-pointer font-sans text-xs font-medium inline-flex items-center justify-center gap-1.5 ${
                            k.status === 'active'
                              ? 'border-srapi-error/25 hover:bg-srapi-error/5 text-srapi-error'
                              : 'border-green-500/20 hover:bg-green-500/5 text-green-700'
                          }`}
                        >
                          <Power size={11} />
                          {k.status === 'active' ? t('revoke') : t('activate')}
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>

        {/* Creation Dialog Modal (Large soft rounded-3xl with paper background) */}
        {showCreateModal && (
          <div className="fixed inset-0 flex items-center justify-center bg-black/45 z-50 p-4 backdrop-blur-sm">
            <div className="bg-srapi-card border border-srapi-border rounded-3xl p-8 max-w-md w-full paper-grain space-y-6 shadow-2xl relative">
              <button 
                onClick={() => setShowCreateModal(false)}
                className="absolute top-4 right-4 p-1.5 border border-srapi-border hover:bg-srapi-card-muted rounded-full cursor-pointer"
              >
                <X size={14} />
              </button>

              <div className="space-y-1">
                <h4 className="font-serif font-medium text-lg tracking-tight text-srapi-text-primary">{t('deployTitle')}</h4>
                <p className="text-xs text-srapi-text-secondary font-sans leading-relaxed">{t('deployDesc')}</p>
              </div>

              <form onSubmit={handleCreate} className="space-y-5">
                <div className="space-y-1.5 text-xs">
                  <label className="font-mono uppercase text-[9px] font-bold text-srapi-text-secondary">{t('keyNickname')}</label>
                  <input
                    type="text"
                    required
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    placeholder="e.g. Production Ingress Gateway"
                    className="w-full px-3.5 py-3 border border-srapi-border bg-srapi-bg text-srapi-text-primary rounded-xl text-xs outline-none focus:border-srapi-primary font-sans"
                  />
                </div>

                <div className="space-y-2 text-xs">
                  <label className="font-mono uppercase text-[9px] font-bold text-srapi-text-secondary">{t('allowedTargetModels')}</label>
                  <div className="grid grid-cols-2 gap-2 mt-1">
                    {['gpt-4o-mini', 'claude-3-5-sonnet', 'gemini-1.5-pro', 'gemini-1.5-flash'].map(m => {
                      const isSelected = selectedModels.includes(m);
                      return (
                        <button
                          type="button"
                          key={m}
                          onClick={() => handleModelToggle(m)}
                          className={`p-2.5 border text-left font-mono rounded-lg transition-all text-[10px] flex items-center justify-between cursor-pointer ${
                            isSelected 
                              ? 'border-srapi-primary bg-srapi-primary/5 text-srapi-primary font-bold' 
                              : 'border-srapi-border bg-srapi-bg text-srapi-text-secondary'
                          }`}
                        >
                          {m}
                          {isSelected && <Sparkles size={10} />}
                        </button>
                      );
                    })}
                  </div>
                </div>

                <div className="space-y-1.5 text-xs">
                  <label className="font-mono uppercase text-[9px] font-bold text-srapi-text-secondary">{t('scopeGroupsCsv')}</label>
                  <input
                    type="text"
                    value={groupIds}
                    onChange={(e) => setGroupIds(e.target.value)}
                    placeholder="group-01, group-02"
                    className="w-full px-3.5 py-3 border border-srapi-border bg-srapi-bg text-srapi-text-primary rounded-xl text-xs outline-none focus:border-srapi-primary font-mono"
                  />
                </div>

                <div className="flex gap-4 pt-3 shrink-0">
                  <button
                    type="button"
                    onClick={() => setShowCreateModal(false)}
                    className="flex-1 py-3.5 border border-srapi-border bg-srapi-card-muted hover:bg-srapi-card-muted/80 rounded-full text-xs font-bold font-mono active:scale-[0.97] cursor-pointer"
                  >
                    {t('cancel')}
                  </button>
                  <button
                    type="submit"
                    disabled={isCreating}
                    className="flex-1 py-3.5 bg-srapi-text-primary text-srapi-bg dark:bg-srapi-text-primary dark:text-srapi-bg hover:bg-neutral-800 rounded-full text-xs font-bold font-mono active:scale-[0.97] disabled:opacity-40 disabled:cursor-not-allowed flex items-center justify-center gap-1.5 cursor-pointer"
                  >
                    {isCreating ? t('deploying') : t('deployChannel')}
                  </button>
                </div>
              </form>
            </div>
          </div>
        )}

      </div>
    </DashboardLayout>
  );
}
