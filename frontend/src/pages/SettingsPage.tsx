import React, { useEffect, useState } from 'react';
import { api } from '@/lib/api';
import { useAuth } from '@/context/AuthContext';
import {
  Bell,
  CheckCircle2,
  Mail,
  MessageSquare,
  RefreshCw,
  Save,
  Share2,
  ShieldCheck,
} from 'lucide-react';

export const SettingsPage: React.FC = () => {
  const { user } = useAuth();

  // Notification Preferences
  const [emailEnabled, setEmailEnabled] = useState(true);
  const [teamsEnabled, setTeamsEnabled] = useState(false);
  const [teamsWebhookURL, setTeamsWebhookURL] = useState('');
  const [savingPrefs, setSavingPrefs] = useState(false);
  const [savedSuccess, setSavedSuccess] = useState(false);

  const loadData = async () => {
    try {
      const prefs = await api.getNotificationPreferences();
      for (const p of (prefs || [])) {
        if (p.channel === 'email') {
          setEmailEnabled(p.enabled);
        } else if (p.channel === 'teams') {
          setTeamsEnabled(p.enabled);
          if (p.teams_webhook_masked) {
            setTeamsWebhookURL(p.teams_webhook_masked);
          }
        }
      }
    } catch (err) {
      console.error(err);
    }
  };

  useEffect(() => {
    loadData();
  }, []);

  const handleSaveNotifications = async (e: React.FormEvent) => {
    e.preventDefault();
    setSavingPrefs(true);
    setSavedSuccess(false);
    try {
      await Promise.all([
        api.saveNotificationPreference({
          channel: 'email',
          enabled: emailEnabled,
        }),
        api.saveNotificationPreference({
          channel: 'teams',
          enabled: teamsEnabled,
          teams_webhook_url: teamsWebhookURL.includes('••••') ? undefined : teamsWebhookURL,
        }),
      ]);
      setSavedSuccess(true);
      setTimeout(() => setSavedSuccess(false), 4000);
    } catch (err: any) {
      alert(err.message || 'Failed to save notification preferences');
    } finally {
      setSavingPrefs(false);
    }
  };

  return (
    <div className="max-w-4xl mx-auto py-6 px-4 space-y-8">
      <div className="border-b pb-5">
        <h1 className="text-2xl font-black tracking-tight">Personal Preferences & Alerts</h1>
        <p className="text-sm text-muted-foreground">
          Manage your delivery channels and generation completion alerts
        </p>
      </div>

      {/* Information: Project-Scoped Confluence Integration */}
      <div className="bg-card border rounded-2xl p-6 shadow-sm flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div className="flex items-start gap-4">
          <div className="h-10 w-10 rounded-xl bg-blue-100 dark:bg-blue-950 text-blue-600 flex items-center justify-center shrink-0">
            <Share2 className="h-5 w-5" />
          </div>
          <div className="space-y-1">
            <h2 className="text-base font-bold">Atlassian Confluence Integration</h2>
            <p className="text-xs text-muted-foreground leading-relaxed max-w-xl">
              Confluence publishing targets (target space and parent page) are configured directly inside each specific <strong>Project</strong>. DocForge uses your active Atlassian session to verify permissions and publish approved documents.
            </p>
          </div>
        </div>
        <button
          type="button"
          onClick={async () => {
            try {
              const res = await api.atlassianSSOStart();
              if (res.authorization_url) window.location.href = res.authorization_url;
            } catch (err: any) {
              alert(err.message || 'Failed to start authorization');
            }
          }}
          className="shrink-0 px-4 py-2 rounded-xl bg-primary text-primary-foreground text-xs font-semibold hover:opacity-90 transition-opacity flex items-center gap-2 self-start sm:self-center"
        >
          <RefreshCw className="h-3.5 w-3.5" /> Reconnect Atlassian Session
        </button>
      </div>

      {/* SECTION 2: NOTIFICATION PREFERENCES */}
      <form onSubmit={handleSaveNotifications} className="bg-card border rounded-2xl p-6 shadow-sm space-y-6">
        <div className="flex items-center gap-3 border-b pb-4">
          <div className="h-10 w-10 rounded-xl bg-purple-100 dark:bg-purple-950 text-purple-600 flex items-center justify-center">
            <Bell className="h-5 w-5" />
          </div>
          <div>
            <h2 className="text-base font-bold">Generation Notifications</h2>
            <p className="text-xs text-muted-foreground">
              Choose how you want to be notified when background generation jobs complete.
            </p>
          </div>
        </div>

        {savedSuccess && (
          <div className="p-3.5 rounded-xl bg-emerald-50 dark:bg-emerald-950/40 text-emerald-800 dark:text-emerald-300 border border-emerald-200 text-xs font-semibold flex items-center gap-2">
            <CheckCircle2 className="h-4 w-4" /> Notification preferences saved successfully!
          </div>
        )}

        {/* Channel 1: Email */}
        <div className="flex items-start justify-between gap-4 p-4 rounded-xl border bg-muted/20">
          <div className="space-y-1">
            <div className="flex items-center gap-2 font-bold text-xs">
              <Mail className="h-4 w-4 text-blue-500" />
              <span>Email Notifications (SSO Identity)</span>
            </div>
            <p className="text-xs text-muted-foreground">
              Sends an audit summary to your registered account email (<strong>{user?.email}</strong>) upon job completion.
            </p>
          </div>

          <label className="relative inline-flex items-center cursor-pointer">
            <input
              type="checkbox"
              checked={emailEnabled}
              onChange={(e) => setEmailEnabled(e.target.checked)}
              className="sr-only peer"
            />
            <div className="w-10 h-6 bg-muted peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary"></div>
          </label>
        </div>

        {/* Channel 2: Microsoft Teams Webhook */}
        <div className="space-y-3 p-4 rounded-xl border bg-muted/20">
          <div className="flex items-start justify-between gap-4">
            <div className="space-y-1">
              <div className="flex items-center gap-2 font-bold text-xs">
                <MessageSquare className="h-4 w-4 text-indigo-500" />
                <span>Microsoft Teams Channel Webhook</span>
              </div>
              <p className="text-xs text-muted-foreground">
                Posts an interactive Adaptive Card to your Microsoft Teams channel when generation finishes.
              </p>
            </div>

            <label className="relative inline-flex items-center cursor-pointer">
              <input
                type="checkbox"
                checked={teamsEnabled}
                onChange={(e) => setTeamsEnabled(e.target.checked)}
                className="sr-only peer"
              />
              <div className="w-10 h-6 bg-muted peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary"></div>
            </label>
          </div>

          {teamsEnabled && (
            <div className="space-y-1.5 pt-2">
              <label className="text-xs font-semibold text-muted-foreground flex items-center justify-between">
                <span>Teams Incoming Webhook URL</span>
                <span className="text-[10px] text-emerald-600 flex items-center gap-1 font-mono">
                  <ShieldCheck className="h-3 w-3" /> AES-256 GCM Encrypted
                </span>
              </label>
              <input
                type="text"
                value={teamsWebhookURL}
                onChange={(e) => setTeamsWebhookURL(e.target.value)}
                placeholder="https://outlook.office.com/webhook/..."
                className="w-full px-3.5 py-2.5 rounded-lg border bg-background text-xs font-mono focus:ring-1 focus:ring-primary"
              />
            </div>
          )}
        </div>

        <div className="pt-2 flex justify-end">
          <button
            type="submit"
            disabled={savingPrefs}
            className="inline-flex items-center gap-2 px-5 py-2.5 rounded-lg bg-primary text-primary-foreground font-semibold text-xs shadow hover:opacity-90 disabled:opacity-50"
          >
            {savingPrefs ? (
              <>
                <RefreshCw className="h-3.5 w-3.5 animate-spin" /> Saving...
              </>
            ) : (
              <>
                <Save className="h-3.5 w-3.5" /> Save Notification Preferences
              </>
            )}
          </button>
        </div>
      </form>
    </div>
  );
};
