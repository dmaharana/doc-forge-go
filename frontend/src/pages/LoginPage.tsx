import React, { useEffect, useState } from 'react';
import { useAuth } from '@/context/AuthContext';
import { api } from '@/lib/api';
import { FileText, KeyRound, Lock, Mail, ShieldCheck, ChevronDown, ChevronUp } from 'lucide-react';

export const LoginPage: React.FC = () => {
  const { login, loginWithAtlassian } = useAuth();
  const [email, setEmail] = useState('admin@docforge.local');
  const [password, setPassword] = useState('Admin123!');
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [showPasswordLogin, setShowPasswordLogin] = useState(false);

  useEffect(() => {
    // Check if redirected from Atlassian SSO with code
    const params = new URLSearchParams(window.location.search);
    const code = params.get('code');
    if (code) {
      setLoading(true);
      loginWithAtlassian(code).catch((err: any) => {
        setError(err.message || 'Atlassian SSO failed');
        setLoading(false);
      });
    }
  }, [loginWithAtlassian]);

  const handleAtlassianSSO = async () => {
    setError(null);
    setLoading(true);
    try {
      const res = await api.getAtlassianSSOUrl();
      if (res.is_mock_mode === 'true') {
        await loginWithAtlassian('mock_atlassian_code');
      } else {
        window.location.href = res.authorization_url;
      }
    } catch (err: any) {
      setError(err.message || 'Failed to initiate Atlassian SSO');
      setLoading(false);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      await login(email, password);
    } catch (err: any) {
      setError(err.message || 'Login failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen w-full flex items-center justify-center bg-slate-50 dark:bg-slate-950 p-4">
      <div className="w-full max-w-md bg-card border rounded-2xl shadow-xl p-8 space-y-6">
        {/* Brand Header */}
        <div className="text-center space-y-2">
          <div className="inline-flex h-12 w-12 items-center justify-center rounded-xl bg-primary text-primary-foreground shadow-md mb-2">
            <FileText className="h-6 w-6" />
          </div>
          <h1 className="text-2xl font-bold tracking-tight">Welcome to DocForge</h1>
          <p className="text-sm text-muted-foreground">
            Enterprise Technical Documentation with Confluence Integration
          </p>
        </div>

        {error && (
          <div className="p-3 text-sm rounded-lg bg-destructive/15 text-destructive border border-destructive/20 font-medium">
            {error}
          </div>
        )}

        {/* PRIMARY: Atlassian SSO Enforced Login */}
        <div className="space-y-3">
          <button
            type="button"
            onClick={handleAtlassianSSO}
            disabled={loading}
            className="w-full py-3 px-4 rounded-xl bg-blue-600 hover:bg-blue-700 text-white font-semibold text-sm shadow-md hover:shadow-lg disabled:opacity-50 transition flex items-center justify-center gap-2.5"
          >
            {loading ? (
              <span className="animate-spin rounded-full h-4 w-4 border-2 border-white border-t-transparent" />
            ) : (
              <svg className="h-5 w-5 fill-current" viewBox="0 0 24 24">
                <path d="M11.53 2c0 2.4-1.97 4.35-4.4 4.35H2.8a.8.8 0 0 0-.8.8v4.33c0 2.4 1.97 4.35 4.4 4.35h4.33c.44 0 .8-.36.8-.8V2.8a.8.8 0 0 0-.8-.8h.8zm9.67 9.67c0-2.4-1.97-4.35-4.4-4.35h-4.33a.8.8 0 0 0-.8.8v4.33c0 2.4 1.97 4.35 4.4 4.35h4.33c.44 0 .8-.36.8-.8v-4.33z"/>
              </svg>
            )}
            Sign in with Atlassian SSO
          </button>
          <p className="text-xs text-center text-muted-foreground">
            Sign in with your company Atlassian Cloud identity to access assigned projects and export documentation directly to Confluence.
          </p>
        </div>

        {/* Expandable Fallback for Local Dev / Admin Password Login */}
        <div className="pt-2 border-t">
          <button
            type="button"
            onClick={() => setShowPasswordLogin(!showPasswordLogin)}
            className="w-full py-1.5 text-xs text-muted-foreground hover:text-foreground flex items-center justify-center gap-1 transition"
          >
            <span>Developer / Seed Admin Login</span>
            {showPasswordLogin ? <ChevronUp className="h-3.5 w-3.5" /> : <ChevronDown className="h-3.5 w-3.5" />}
          </button>

          {showPasswordLogin && (
            <form onSubmit={handleSubmit} className="mt-3 space-y-3 p-4 rounded-xl border bg-muted/30">
              <div className="space-y-1">
                <label className="text-xs font-semibold text-muted-foreground flex items-center gap-1.5">
                  <Mail className="h-3.5 w-3.5" /> Email Address
                </label>
                <input
                  type="email"
                  required
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg border bg-background text-sm focus:outline-none focus:ring-2 focus:ring-primary/20"
                  placeholder="admin@docforge.local"
                />
              </div>

              <div className="space-y-1">
                <label className="text-xs font-semibold text-muted-foreground flex items-center gap-1.5">
                  <Lock className="h-3.5 w-3.5" /> Password
                </label>
                <input
                  type="password"
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg border bg-background text-sm focus:outline-none focus:ring-2 focus:ring-primary/20"
                  placeholder="••••••••"
                />
              </div>

              <button
                type="submit"
                disabled={loading}
                className="w-full py-2 px-3 rounded-lg bg-secondary text-secondary-foreground font-medium text-xs hover:bg-secondary/80 disabled:opacity-50 transition flex items-center justify-center gap-1.5"
              >
                <KeyRound className="h-3.5 w-3.5" />
                Sign In with Credentials
              </button>
            </form>
          )}
        </div>

        <div className="pt-2 text-center text-xs text-muted-foreground border-t flex items-center justify-center gap-1.5">
          <ShieldCheck className="h-3.5 w-3.5 text-emerald-500" />
          <span>DocForge • Project-Scoped Confluence & Atlassian SSO</span>
        </div>
      </div>
    </div>
  );
};
