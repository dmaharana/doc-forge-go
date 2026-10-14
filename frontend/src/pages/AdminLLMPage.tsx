import React, { useEffect, useState } from 'react';
import type { LLMConfig } from '@/types';
import { api } from '@/lib/api';
import { CheckCircle2, Cpu, Key, RefreshCw, Save, ShieldCheck } from 'lucide-react';

export const AdminLLMPage: React.FC = () => {
  const [, setConfigs] = useState<LLMConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [successMsg, setSuccessMsg] = useState<string | null>(null);

  // Form states
  const [provider, setProvider] = useState('openai');
  const [apiKey, setApiKey] = useState('');
  const [baseURL, setBaseURL] = useState('');
  const [defaultModel, setDefaultModel] = useState('gpt-4o');
  const [maxTokens, setMaxTokens] = useState(4096);
  const [temperature, setTemperature] = useState(0.2);

  const loadConfigs = async () => {
    setLoading(true);
    try {
      const list = await api.adminListLLM();
      setConfigs(list || []);
      const active = (list || []).find((c) => c.is_active);
      if (active) {
        setProvider(active.provider);
        setBaseURL(active.base_url || '');
        setDefaultModel(active.default_model);
        setMaxTokens(active.max_tokens);
        setTemperature(active.temperature);
      }
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadConfigs();
  }, []);

  const handleProviderPreset = (p: string) => {
    setProvider(p);
    if (p === 'openai') {
      setBaseURL('https://api.openai.com/v1');
      setDefaultModel('gpt-4o');
    } else if (p === 'anthropic') {
      setBaseURL('https://api.anthropic.com/v1');
      setDefaultModel('claude-3-5-sonnet-20241022');
    } else if (p === 'azure') {
      setBaseURL('https://<your-resource>.openai.azure.com/openai/deployments/<model>');
      setDefaultModel('gpt-4o');
    } else if (p === 'mock') {
      setBaseURL('');
      setDefaultModel('docforge-builtin-synthesizer');
    }
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setSuccessMsg(null);
    try {
      await api.adminSaveLLM({
        provider,
        api_key: apiKey || undefined,
        base_url: baseURL,
        default_model: defaultModel,
        max_tokens: Number(maxTokens),
        temperature: Number(temperature),
        is_active: true,
      });
      setSuccessMsg('LLM Provider configuration saved and activated! Keys encrypted with AES-GCM.');
      setApiKey('');
      await loadConfigs();
    } catch (err: any) {
      alert(err.message || 'Failed to save LLM configuration');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="max-w-4xl mx-auto py-6 px-4 space-y-6">
      <div className="border-b pb-5">
        <h1 className="text-2xl font-black tracking-tight flex items-center gap-2">
          <Cpu className="h-6 w-6 text-primary" /> LLM Provider Configuration
        </h1>
        <p className="text-sm text-muted-foreground">
          Configure model parameters and API credentials. All secrets are encrypted at rest using AES-GCM.
        </p>
      </div>

      {successMsg && (
        <div className="p-4 rounded-xl bg-emerald-50 dark:bg-emerald-950/40 text-emerald-800 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800 text-xs font-semibold flex items-center gap-2">
          <CheckCircle2 className="h-4 w-4" /> {successMsg}
        </div>
      )}

      {loading ? (
        <div className="p-16 text-center text-muted-foreground flex items-center justify-center gap-2">
          <RefreshCw className="h-5 w-5 animate-spin" /> Loading configurations...
        </div>
      ) : (
        <form onSubmit={handleSave} className="space-y-6 bg-card border rounded-2xl p-6 shadow-sm">
          {/* Provider Selection */}
          <div className="space-y-2">
            <label className="text-xs font-bold uppercase text-muted-foreground">Select Provider</label>
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-2.5">
              {[
                { id: 'openai', name: 'OpenAI' },
                { id: 'anthropic', name: 'Anthropic' },
                { id: 'azure', name: 'Azure OpenAI / Custom' },
                { id: 'mock', name: 'Offline Mock' },
              ].map((p) => (
                <button
                  key={p.id}
                  type="button"
                  onClick={() => handleProviderPreset(p.id)}
                  className={`p-3 rounded-xl border text-xs font-bold text-center transition ${
                    provider === p.id
                      ? 'bg-primary text-primary-foreground border-primary shadow-sm'
                      : 'hover:bg-muted text-muted-foreground'
                  }`}
                >
                  {p.name}
                </button>
              ))}
            </div>
          </div>

          {/* API Key */}
          <div className="space-y-1.5">
            <label className="text-xs font-bold uppercase text-muted-foreground flex items-center justify-between">
              <span className="flex items-center gap-1.5">
                <Key className="h-3.5 w-3.5" /> API Key
              </span>
              <span className="text-[10px] text-emerald-600 font-mono font-medium flex items-center gap-1">
                <ShieldCheck className="h-3 w-3" /> AES-256 GCM Encrypted
              </span>
            </label>
            <input
              type="password"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder={provider === 'mock' ? 'Not required for offline mock' : 'sk-... (Leave empty to keep existing encrypted key)'}
              className="w-full px-3.5 py-2.5 rounded-lg border bg-background text-xs font-mono focus:ring-1 focus:ring-primary"
            />
          </div>

          {/* Base URL */}
          <div className="space-y-1.5">
            <label className="text-xs font-bold uppercase text-muted-foreground">Base URL (API Endpoint)</label>
            <input
              type="text"
              value={baseURL}
              onChange={(e) => setBaseURL(e.target.value)}
              placeholder="https://api.openai.com/v1"
              className="w-full px-3.5 py-2.5 rounded-lg border bg-background text-xs font-mono focus:ring-1 focus:ring-primary"
            />
          </div>

          {/* Model Name */}
          <div className="space-y-1.5">
            <label className="text-xs font-bold uppercase text-muted-foreground">Default Model</label>
            <input
              type="text"
              required
              value={defaultModel}
              onChange={(e) => setDefaultModel(e.target.value)}
              placeholder="e.g. gpt-4o, claude-3-5-sonnet-20241022"
              className="w-full px-3.5 py-2.5 rounded-lg border bg-background text-xs font-mono focus:ring-1 focus:ring-primary"
            />
          </div>

          {/* Max Tokens & Temperature */}
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <div className="space-y-1.5">
              <label className="text-xs font-bold uppercase text-muted-foreground">Max Output Tokens</label>
              <input
                type="number"
                min={256}
                max={16384}
                value={maxTokens}
                onChange={(e) => setMaxTokens(Number(e.target.value))}
                className="w-full px-3.5 py-2.5 rounded-lg border bg-background text-xs focus:ring-1 focus:ring-primary"
              />
            </div>

            <div className="space-y-1.5">
              <div className="flex justify-between text-xs font-bold uppercase text-muted-foreground">
                <span>Temperature</span>
                <span>{temperature}</span>
              </div>
              <input
                type="range"
                min={0}
                max={1}
                step={0.05}
                value={temperature}
                onChange={(e) => setTemperature(parseFloat(e.target.value))}
                className="w-full mt-2 cursor-pointer"
              />
              <span className="text-[10px] text-muted-foreground">Lower values guarantee factual determinism</span>
            </div>
          </div>

          <div className="pt-4 border-t flex justify-end">
            <button
              type="submit"
              disabled={saving}
              className="inline-flex items-center gap-2 px-6 py-2.5 rounded-lg bg-primary text-primary-foreground font-semibold text-xs shadow hover:opacity-90 disabled:opacity-50"
            >
              {saving ? (
                <>
                  <RefreshCw className="h-4 w-4 animate-spin" /> Saving...
                </>
              ) : (
                <>
                  <Save className="h-4 w-4" /> Save & Activate Configuration
                </>
              )}
            </button>
          </div>
        </form>
      )}
    </div>
  );
};
