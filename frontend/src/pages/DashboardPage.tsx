import React, { useEffect, useState } from 'react';
import type { LLMConfig, Project } from '@/types';
import { api } from '@/lib/api';
import { useAuth } from '@/context/AuthContext';
import {
  ArrowRight,
  BookOpen,
  Cpu,
  FileCheck2,
  FolderGit2,
  Plus,
  RefreshCw,
  Sparkles,
  Layers,
} from 'lucide-react';

interface DashboardPageProps {
  onNavigate: (view: string, id?: string) => void;
}

export const DashboardPage: React.FC<DashboardPageProps> = ({ onNavigate }) => {
  const { user, isAdmin } = useAuth();
  const [projects, setProjects] = useState<Project[]>([]);
  const [llmConfigs, setLlmConfigs] = useState<LLMConfig[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [newProjectName, setNewProjectName] = useState('');
  const [newProjectDesc, setNewProjectDesc] = useState('');
  const [creating, setCreating] = useState(false);

  const loadData = async () => {
    setLoading(true);
    try {
      const projList = await api.listProjects();
      setProjects(projList || []);

      if (isAdmin) {
        const llms = await api.adminListLLM();
        setLlmConfigs(llms || []);
      }
    } catch (err) {
      console.error('Failed to load dashboard data:', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
  }, [isAdmin]);

  const handleCreateProject = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newProjectName.trim()) return;
    setCreating(true);
    try {
      const created = await api.createProject({
        name: newProjectName,
        description: newProjectDesc,
      });
      setShowCreateModal(false);
      setNewProjectName('');
      setNewProjectDesc('');
      onNavigate('project-detail', created.id);
    } catch (err: any) {
      alert(err.message || 'Failed to create project');
    } finally {
      setCreating(false);
    }
  };

  const activeLLM = llmConfigs.find((c) => c.is_active);

  return (
    <div className="space-y-8 max-w-7xl mx-auto py-6 px-4">
      {/* Welcome Banner */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 border-b pb-6">
        <div>
          <h1 className="text-3xl font-extrabold tracking-tight">Welcome back, {user?.name}</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            DocForge technical documentation workspace • AI Synthesis & Human Review Pipeline
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={() => setShowCreateModal(true)}
            className="inline-flex items-center gap-2 px-4 py-2.5 rounded-lg bg-primary text-primary-foreground font-semibold text-sm shadow hover:opacity-90 transition"
          >
            <Plus className="h-4 w-4" />
            New Project
          </button>
        </div>
      </div>

      {/* Metric Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="p-5 rounded-xl border bg-card shadow-sm space-y-2">
          <div className="flex items-center justify-between text-muted-foreground">
            <span className="text-xs font-semibold uppercase tracking-wider">Total Projects</span>
            <FolderGit2 className="h-5 w-5 text-primary" />
          </div>
          <div className="text-2xl font-bold">{projects.length}</div>
          <div className="text-xs text-muted-foreground">Active documentation workspaces</div>
        </div>

        <div className="p-5 rounded-xl border bg-card shadow-sm space-y-2">
          <div className="flex items-center justify-between text-muted-foreground">
            <span className="text-xs font-semibold uppercase tracking-wider">AI Engine</span>
            <Cpu className="h-5 w-5 text-indigo-500" />
          </div>
          <div className="text-xl font-bold truncate">
            {activeLLM ? activeLLM.provider.toUpperCase() : 'Mock / Local'}
          </div>
          <div className="text-xs text-muted-foreground truncate">
            Model: {activeLLM ? activeLLM.default_model : 'Builtin Synthesizer'}
          </div>
        </div>

        <div className="p-5 rounded-xl border bg-card shadow-sm space-y-2">
          <div className="flex items-center justify-between text-muted-foreground">
            <span className="text-xs font-semibold uppercase tracking-wider">Templates</span>
            <Layers className="h-5 w-5 text-emerald-500" />
          </div>
          <div className="text-2xl font-bold">4 Built-in</div>
          <div className="text-xs text-muted-foreground">Spec, ADR, Release Notes, Runbook</div>
        </div>

        <div className="p-5 rounded-xl border bg-card shadow-sm space-y-2">
          <div className="flex items-center justify-between text-muted-foreground">
            <span className="text-xs font-semibold uppercase tracking-wider">Governance</span>
            <FileCheck2 className="h-5 w-5 text-amber-500" />
          </div>
          <div className="text-2xl font-bold">Hard Gated</div>
          <div className="text-xs text-muted-foreground">Blocking feedback enforces review sign-off</div>
        </div>
      </div>

      {/* Recent Projects Section */}
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-bold tracking-tight flex items-center gap-2">
            <BookOpen className="h-5 w-5" /> Recent Projects
          </h2>
          <button
            onClick={() => onNavigate('projects')}
            className="text-sm font-medium text-primary hover:underline flex items-center gap-1"
          >
            View all projects <ArrowRight className="h-3.5 w-3.5" />
          </button>
        </div>

        {loading ? (
          <div className="p-12 text-center text-muted-foreground flex items-center justify-center gap-2">
            <RefreshCw className="h-4 w-4 animate-spin" /> Loading projects...
          </div>
        ) : projects.length === 0 ? (
          <div className="p-12 text-center border rounded-xl bg-card/50 space-y-4">
            <FolderGit2 className="h-10 w-10 text-muted-foreground mx-auto" />
            <div>
              <h3 className="font-semibold text-base">No documentation projects yet</h3>
              <p className="text-sm text-muted-foreground max-w-md mx-auto mt-1">
                Create your first project, upload heterogeneous source files (PDFs, Excel, source code, images), and generate documentation drafts.
              </p>
            </div>
            <button
              onClick={() => setShowCreateModal(true)}
              className="inline-flex items-center gap-2 px-4 py-2 rounded-lg bg-primary text-primary-foreground font-semibold text-sm shadow hover:opacity-90"
            >
              <Plus className="h-4 w-4" /> Create First Project
            </button>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {projects.slice(0, 6).map((p) => (
              <div
                key={p.id}
                onClick={() => onNavigate('project-detail', p.id)}
                className="group p-5 rounded-xl border bg-card hover:border-primary/50 hover:shadow-md transition cursor-pointer flex flex-col justify-between"
              >
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="text-xs font-semibold px-2 py-0.5 rounded-full bg-secondary text-secondary-foreground uppercase">
                      {p.status}
                    </span>
                    <span className="text-[11px] text-muted-foreground">
                      {new Date(p.updated_at).toLocaleDateString()}
                    </span>
                  </div>
                  <h3 className="font-bold text-base text-foreground group-hover:text-primary transition line-clamp-1">
                    {p.name}
                  </h3>
                  <p className="text-xs text-muted-foreground line-clamp-2">
                    {p.description || 'No description provided.'}
                  </p>
                </div>

                <div className="pt-4 mt-4 border-t flex items-center justify-between text-xs text-muted-foreground">
                  <span>Owner: {p.owner_name || 'System User'}</span>
                  <span className="font-medium text-primary flex items-center gap-1 group-hover:translate-x-1 transition-transform">
                    Explore <ArrowRight className="h-3 w-3" />
                  </span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Guided Review Pipeline Info Card */}
      <div className="p-6 rounded-2xl border bg-gradient-to-r from-blue-50 to-indigo-50 dark:from-slate-900 dark:to-indigo-950/40 space-y-3">
        <div className="flex items-center gap-2 font-bold text-blue-900 dark:text-blue-200">
          <Sparkles className="h-5 w-5 text-indigo-600 dark:text-indigo-400" />
          The DocForge One-Time Baseline Pipeline
        </div>
        <p className="text-sm text-slate-700 dark:text-slate-300 leading-relaxed">
          DocForge treats technical documentation as a controlled, auditable pipeline: 
          <strong> Ingest heterogeneous sources</strong> (Code, PDFs, Excel, ZIPs, Images) → 
          <strong> Structured AI Draft Synthesis</strong> conforming to embedded engineering templates → 
          <strong> Human Review & Structured Feedback</strong> (with blocking item gates) → 
          <strong> AI Revision Loop</strong> → 
          <strong> Approval & Confluence Publication</strong>.
        </p>
      </div>

      {/* Create Project Modal */}
      {showCreateModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm">
          <div className="bg-card w-full max-w-lg border rounded-2xl shadow-2xl p-6 space-y-4">
            <h2 className="text-xl font-bold">Create New Documentation Project</h2>
            <form onSubmit={handleCreateProject} className="space-y-4">
              <div className="space-y-1">
                <label className="text-xs font-semibold uppercase text-muted-foreground">
                  Project Name
                </label>
                <input
                  type="text"
                  required
                  value={newProjectName}
                  onChange={(e) => setNewProjectName(e.target.value)}
                  className="w-full px-3.5 py-2.5 rounded-lg border bg-background text-sm focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary"
                  placeholder="e.g. Identity & Access Orchestration Service"
                />
              </div>

              <div className="space-y-1">
                <label className="text-xs font-semibold uppercase text-muted-foreground">
                  Description & Context
                </label>
                <textarea
                  rows={3}
                  value={newProjectDesc}
                  onChange={(e) => setNewProjectDesc(e.target.value)}
                  className="w-full px-3.5 py-2.5 rounded-lg border bg-background text-sm focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary"
                  placeholder="Explain what this product/service does to guide LLM synthesis..."
                />
              </div>

              <div className="flex items-center justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowCreateModal(false)}
                  className="px-4 py-2 rounded-lg border font-medium text-sm hover:bg-muted"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={creating}
                  className="px-4 py-2 rounded-lg bg-primary text-primary-foreground font-semibold text-sm shadow hover:opacity-90 disabled:opacity-50"
                >
                  {creating ? 'Creating...' : 'Create Project'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
