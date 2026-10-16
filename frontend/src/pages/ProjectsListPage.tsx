import React, { useEffect, useState } from 'react';
import type { Project } from '@/types';
import { api } from '@/lib/api';
import { ArrowRight, FolderGit2, Plus, Search, Zap } from 'lucide-react';

interface ProjectsListPageProps {
  onNavigate: (view: string, id?: string) => void;
}

export const ProjectsListPage: React.FC<ProjectsListPageProps> = ({ onNavigate }) => {
  const [projects, setProjects] = useState<Project[]>([]);
  const [search, setSearch] = useState('');
  const [loading, setLoading] = useState(true);
  const [showCreateModal, setShowCreateModal] = useState(false);
  const [newName, setNewName] = useState('');
  const [newDesc, setNewDesc] = useState('');
  const [creating, setCreating] = useState(false);

  const loadProjects = async () => {
    setLoading(true);
    try {
      const list = await api.listProjects();
      setProjects(list || []);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadProjects();
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!newName.trim()) return;
    setCreating(true);
    try {
      const p = await api.createProject({ name: newName, description: newDesc });
      setShowCreateModal(false);
      setNewName('');
      setNewDesc('');
      onNavigate('project-detail', p.id);
    } catch (err: any) {
      alert(err.message || 'Failed to create project');
    } finally {
      setCreating(false);
    }
  };

  const filtered = (projects || []).filter(
    (p) =>
      p.name.toLowerCase().includes(search.toLowerCase()) ||
      (p.description && p.description.toLowerCase().includes(search.toLowerCase()))
  );

  return (
    <div className="space-y-6 max-w-7xl mx-auto py-6 px-4">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Documentation Projects</h1>
          <p className="text-sm text-muted-foreground">
            Manage your product documentation workspaces, sources, and generation baselines
          </p>
        </div>
        <button
          onClick={() => setShowCreateModal(true)}
          className="inline-flex items-center gap-2 px-4 py-2.5 rounded-lg bg-primary text-primary-foreground font-semibold text-sm shadow hover:opacity-90"
        >
          <Plus className="h-4 w-4" /> New Project
        </button>
      </div>

      {/* Search Input */}
      <div className="relative max-w-md">
        <Search className="absolute left-3.5 top-3 h-4 w-4 text-muted-foreground" />
        <input
          type="text"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search projects..."
          className="w-full pl-10 pr-4 py-2 rounded-lg border bg-background text-sm focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary"
        />
      </div>

      {/* Projects Grid */}
      {loading ? (
        <div className="p-12 text-center text-muted-foreground">Loading projects...</div>
      ) : filtered.length === 0 ? (
        <div className="p-12 text-center border rounded-xl bg-card space-y-3">
          <FolderGit2 className="h-10 w-10 text-muted-foreground mx-auto" />
          <h3 className="font-semibold text-base">No matching projects found</h3>
          <p className="text-sm text-muted-foreground">Try adjusting your search criteria or create a new project.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
          {filtered.map((p) => (
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
                  <div className="flex items-center gap-1.5">
                    {p.total_tokens ? (
                      <span
                        className="text-[10px] font-mono px-1.5 py-0.5 rounded bg-amber-50 dark:bg-amber-950/60 text-amber-700 dark:text-amber-300 border border-amber-200 dark:border-amber-900 flex items-center gap-1"
                        title="Cumulative Tokens Consumed"
                      >
                        <Zap className="h-2.5 w-2.5" />
                        {p.total_tokens.toLocaleString()}
                      </span>
                    ) : null}
                    <span className="text-[11px] text-muted-foreground">
                      {new Date(p.updated_at).toLocaleDateString()}
                    </span>
                  </div>
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
                  View Detail <ArrowRight className="h-3 w-3" />
                </span>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Modal */}
      {showCreateModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm">
          <div className="bg-card w-full max-w-lg border rounded-2xl shadow-2xl p-6 space-y-4">
            <h2 className="text-xl font-bold">Create New Project</h2>
            <form onSubmit={handleCreate} className="space-y-4">
              <div className="space-y-1">
                <label className="text-xs font-semibold uppercase text-muted-foreground">Project Name</label>
                <input
                  type="text"
                  required
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  className="w-full px-3.5 py-2.5 rounded-lg border bg-background text-sm focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary"
                  placeholder="e.g. Order Settlement & Ledger"
                />
              </div>

              <div className="space-y-1">
                <label className="text-xs font-semibold uppercase text-muted-foreground">Description & Context</label>
                <textarea
                  rows={3}
                  value={newDesc}
                  onChange={(e) => setNewDesc(e.target.value)}
                  className="w-full px-3.5 py-2.5 rounded-lg border bg-background text-sm focus:outline-none focus:ring-2 focus:ring-primary/20 focus:border-primary"
                  placeholder="Summary of product domain, architecture and scope..."
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
