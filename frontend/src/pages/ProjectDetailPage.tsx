import React, { useEffect, useState } from 'react';
import type {
  AtlassianPage,
  AtlassianSpace,
  CompletenessScorecard,
  ConfluenceTarget,
  DocumentItem,
  Generation,
  GenerationStage,
  LLMConfig,
  Project,
  ProjectMember,
  ProjectMemberRole,
  ProjectTokenSummary,
  Source,
} from '@/types';
import { api } from '@/lib/api';
import { useAuth } from '@/context/AuthContext';
import {
  Activity,
  AlertCircle,
  ArrowLeft,
  CheckCircle,
  CheckCircle2,
  Clock,
  Compass,
  Cpu,
  Download,
  Eye,
  FileCode,
  FileText,
  Globe,
  Info,
  Link as LinkIcon,
  RefreshCw,
  ShieldCheck,
  Sparkles,
  Trash2,
  UploadCloud,
  UserPlus,
  Users,
  X,
  Zap,
} from 'lucide-react';

interface ProjectDetailPageProps {
  projectId: string;
  onNavigate: (view: string, id?: string) => void;
}

export const ProjectDetailPage: React.FC<ProjectDetailPageProps> = ({ projectId, onNavigate }) => {
  const { user, isAdmin } = useAuth();
  const [project, setProject] = useState<Project | null>(null);
  const [sources, setSources] = useState<Source[]>([]);
  const [generations, setGenerations] = useState<Generation[]>([]);
  const [documents, setDocuments] = useState<DocumentItem[]>([]);
  const [confluenceTarget, setConfluenceTarget] = useState<ConfluenceTarget | null>(null);
  const [members, setMembers] = useState<ProjectMember[]>([]);

  const isOwnerOrAdmin = isAdmin || project?.user_role === 'owner' || (Boolean(user) && project?.owner_id === user?.id);

  const [activeTab, setActiveTab] = useState<'overview' | 'sources' | 'generations' | 'documents' | 'confluence' | 'members'>('overview');
  const [loading, setLoading] = useState(true);

  // Confluence Target Management State
  const [confluenceSpaces, setConfluenceSpaces] = useState<AtlassianSpace[]>([]);
  const [loadingSpaces, setLoadingSpaces] = useState(false);
  const [confluencePages, setConfluencePages] = useState<AtlassianPage[]>([]);
  const [loadingPages, setLoadingPages] = useState(false);
  const [selectedSpaceKey, setSelectedSpaceKey] = useState('');
  const [manualSpaceName, setManualSpaceName] = useState('');
  const [useManualSpaceKey, setUseManualSpaceKey] = useState(false);
  const [selectedParentPageId, setSelectedParentPageId] = useState('');
  const [parentPageSearch, setParentPageSearch] = useState('');
  const [manualPageId, setManualPageId] = useState('');
  const [manualPageTitle, setManualPageTitle] = useState('');
  const [useManualPageId, setUseManualPageId] = useState(false);
  const [isEditingConfluenceTarget, setIsEditingConfluenceTarget] = useState(false);
  const [savingConfluenceTarget, setSavingConfluenceTarget] = useState(false);
  const [confluenceMsg, setConfluenceMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  // Members Management State
  const [inviteEmail, setInviteEmail] = useState('');
  const [inviteRole, setInviteRole] = useState<ProjectMemberRole>('member');
  const [invitingMember, setInvitingMember] = useState(false);
  const [memberMsg, setMemberMsg] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  // Modals
  const [showUploadModal, setShowUploadModal] = useState(false);
  const [showRemoteModal, setShowRemoteModal] = useState(false);
  const [showGenModal, setShowGenModal] = useState(false);
  const [previewSource, setPreviewSource] = useState<Source | null>(null);

  // Form states
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [uploading, setUploading] = useState(false);

  const [remoteName, setRemoteName] = useState('');
  const [remoteURL, setRemoteURL] = useState('');
  const [remoteType, setRemoteType] = useState('http');
  const [addingRemote, setAddingRemote] = useState(false);

  // Generation Request Form states
  const [selectedTemplates, setSelectedTemplates] = useState<string[]>([
    'product_spec',
    'adr',
    'release_notes',
    'runbook',
  ]);
  const [selectedSourceIds, setSelectedSourceIds] = useState<string[]>([]);
  const [submittingGen, setSubmittingGen] = useState(false);

  // Completeness & Stage Inspection States
  const [completeness, setCompleteness] = useState<CompletenessScorecard | null>(null);
  const [loadingCompleteness, setLoadingCompleteness] = useState(false);
  const [selectedGenForStages, setSelectedGenForStages] = useState<Generation | null>(null);
  const [genStages, setGenStages] = useState<GenerationStage[]>([]);
  const [loadingStages, setLoadingStages] = useState(false);
  const [showSystemMapModal, setShowSystemMapModal] = useState<string | null>(null);

  // LLM Config association
  const [availableLLMs, setAvailableLLMs] = useState<LLMConfig[]>([]);
  const [showLLMModal, setShowLLMModal] = useState(false);
  const [selectedLLMId, setSelectedLLMId] = useState<string>('');
  const [savingLLM, setSavingLLM] = useState(false);
  const [selectedGenLLMId, setSelectedGenLLMId] = useState<string>('');

  // Token Usage & Chargeback
  const [tokenSummary, setTokenSummary] = useState<ProjectTokenSummary | null>(null);
  const [showTokenLedgerModal, setShowTokenLedgerModal] = useState(false);

  const handleOpenGenModal = async () => {
    setSelectedSourceIds((sources || []).map((s) => s.id));
    setSelectedGenLLMId(project?.llm_config_id || '');
    setShowGenModal(true);
    setLoadingCompleteness(true);
    try {
      const card = await api.getProjectCompleteness(projectId);
      setCompleteness(card);
    } catch (err) {
      console.error('Failed to load completeness:', err);
    } finally {
      setLoadingCompleteness(false);
    }
  };

  const handleViewStages = async (gen: Generation) => {
    setSelectedGenForStages(gen);
    setLoadingStages(true);
    try {
      const stages = await api.getGenerationStages(gen.id);
      setGenStages(stages || []);
    } catch (err) {
      console.error(err);
      setGenStages(gen.stages || []);
    } finally {
      setLoadingStages(false);
    }
  };

  const handleSaveProjectLLM = async (e: React.FormEvent) => {
    e.preventDefault();
    setSavingLLM(true);
    try {
      const updated = await api.updateProject(projectId, {
        llm_config_id: selectedLLMId || null,
      });
      setProject(updated);
      setShowLLMModal(false);
    } catch (err: any) {
      alert(err.message || 'Failed to update associated LLM');
    } finally {
      setSavingLLM(false);
    }
  };

  const loadAll = async () => {
    try {
      const [p, s, g, d, ct, m, llms, tokens] = await Promise.all([
        api.getProject(projectId),
        api.listSources(projectId),
        api.listGenerations(projectId),
        api.listDocuments(projectId),
        api.getProjectConfluenceTarget(projectId).catch(() => null),
        api.getProjectMembers(projectId).catch(() => []),
        api.getLLMConfigs().catch(() => []),
        api.getProjectTokenUsage(projectId).catch(() => null),
      ]);
      setProject(p);
      setSources(s || []);
      setGenerations(g || []);
      setDocuments(d || []);
      setConfluenceTarget(ct);
      setMembers(m || []);
      setAvailableLLMs(llms || []);
      setTokenSummary(tokens);
      setSelectedLLMId(p.llm_config_id || '');
      setSelectedGenLLMId(p.llm_config_id || '');
      if (ct) {
        setSelectedSpaceKey(ct.space_key);
        setSelectedParentPageId(ct.parent_page_id || '');
        setManualPageId(ct.parent_page_id || '');
        setManualPageTitle(ct.parent_page_title || '');
        if (ct.parent_page_id) {
          setUseManualPageId(true);
        }
      }
      loadSpaces();
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const loadSpaces = async () => {
    setLoadingSpaces(true);
    try {
      const sp = await api.getProjectConfluenceSpaces(projectId);
      setConfluenceSpaces(sp || []);
      if (sp?.length && !selectedSpaceKey) {
        setSelectedSpaceKey(sp[0].key);
        loadPages(sp[0].key);
      } else if (selectedSpaceKey) {
        loadPages(selectedSpaceKey);
      } else if (!sp?.length) {
        setUseManualSpaceKey(true);
      }
    } catch (err: any) {
      setUseManualSpaceKey(true);
      const msg = err.message || '';
      if (!msg.includes('401') && !msg.includes('scope') && !msg.includes('Unauthorized')) {
        setConfluenceMsg({ type: 'error', text: msg || 'Failed to load Confluence spaces' });
      }
    } finally {
      setLoadingSpaces(false);
    }
  };

  const loadPages = async (spaceKey: string) => {
    if (!spaceKey) return;
    setLoadingPages(true);
    try {
      const pg = await api.getProjectConfluencePages(projectId, spaceKey);
      setConfluencePages(pg || []);
    } catch {
      setConfluencePages([]);
    } finally {
      setLoadingPages(false);
    }
  };

  const handleSpaceChange = (spaceKey: string) => {
    setSelectedSpaceKey(spaceKey);
    loadPages(spaceKey);
  };

  const handleSaveConfluence = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedSpaceKey) return;
    setSavingConfluenceTarget(true);
    setConfluenceMsg(null);
    try {
      const spaceObj = confluenceSpaces.find((s) => s.key === selectedSpaceKey);

      // Determine parent page ID from manual input or list selection
      let parentId = '';
      if (useManualPageId || confluencePages.length === 0) {
        parentId = manualPageId.trim();
      } else {
        parentId = selectedParentPageId || manualPageId.trim();
      }

      // If user pasted a full Confluence URL, extract the numeric Page ID
      const urlPageMatch = parentId.match(/\/pages\/(\d+)/) || parentId.match(/[?&]pageId=(\d+)/);
      if (urlPageMatch) {
        parentId = urlPageMatch[1];
      }

      const pageObj = confluencePages.find((p) => p.id === parentId);
      const parentTitle = pageObj
        ? pageObj.title
        : (manualPageTitle.trim() || (parentId ? 'Page ' + parentId : undefined));

      const target = await api.setProjectConfluenceTarget(projectId, {
        space_key: selectedSpaceKey,
        space_name: spaceObj ? spaceObj.name : (manualSpaceName.trim() || selectedSpaceKey),
        parent_page_id: parentId || undefined,
        parent_page_title: parentTitle,
      });
      setConfluenceTarget(target);
      setIsEditingConfluenceTarget(false);
      setConfluenceMsg({ type: 'success', text: `Confluence Target saved! Space: ${target.space_key}` });
      setTimeout(() => setConfluenceMsg(null), 4000);
    } catch (err: any) {
      setConfluenceMsg({ type: 'error', text: err.message || 'Failed to save Confluence Target' });
    } finally {
      setSavingConfluenceTarget(false);
    }
  };

  const handleDisconnectConfluence = async () => {
    if (!confirm('Are you sure you want to disconnect the Confluence target? Previously published document links will remain intact.')) return;
    try {
      await api.deleteProjectConfluenceTarget(projectId);
      setConfluenceTarget(null);
      setSelectedSpaceKey('');
      setSelectedParentPageId('');
      setConfluenceMsg({ type: 'success', text: 'Confluence Target disconnected.' });
      loadSpaces();
      setTimeout(() => setConfluenceMsg(null), 4000);
    } catch (err: any) {
      alert(err.message || 'Failed to disconnect');
    }
  };

  const handleInviteMember = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!inviteEmail) return;
    setInvitingMember(true);
    setMemberMsg(null);
    try {
      await api.addProjectMember(projectId, inviteEmail, inviteRole);
      setInviteEmail('');
      setMemberMsg({ type: 'success', text: 'Member invited successfully!' });
      const mList = await api.getProjectMembers(projectId);
      setMembers(mList || []);
      setTimeout(() => setMemberMsg(null), 4000);
    } catch (err: any) {
      setMemberMsg({ type: 'error', text: err.message || 'Failed to add member' });
    } finally {
      setInvitingMember(false);
    }
  };

  const handleRemoveMember = async (memberId: string) => {
    if (!confirm('Remove this member from the project?')) return;
    try {
      await api.deleteProjectMember(projectId, memberId);
      const mList = await api.getProjectMembers(projectId);
      setMembers(mList || []);
    } catch (err: any) {
      alert(err.message || 'Failed to remove member');
    }
  };

  useEffect(() => {
    loadAll();
  }, [projectId]);

  // Live polling for in-flight generations
  useEffect(() => {
    const hasActive = (generations || []).some(
      (g) => g.status === 'queued' || g.status === 'processing'
    );
    if (!hasActive) return;

    const interval = setInterval(async () => {
      try {
        const [gList, dList, tokens] = await Promise.all([
          api.listGenerations(projectId),
          api.listDocuments(projectId),
          api.getProjectTokenUsage(projectId).catch(() => null),
        ]);
        setGenerations(gList || []);
        setDocuments(dList || []);
        if (tokens) setTokenSummary(tokens);
      } catch (err) {
        console.error('Polling error:', err);
      }
    }, 2500);

    return () => clearInterval(interval);
  }, [generations, projectId]);

  // Upload handler
  const handleUpload = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedFile) return;
    setUploading(true);
    try {
      await api.uploadSourceFile(projectId, selectedFile);
      setShowUploadModal(false);
      setSelectedFile(null);
      await loadAll();
    } catch (err: any) {
      alert(err.message || 'Upload failed');
    } finally {
      setUploading(false);
    }
  };

  // Remote source handler
  const handleAddRemote = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!remoteURL.trim()) return;
    setAddingRemote(true);
    try {
      await api.addRemoteSource(projectId, {
        name: remoteName || remoteURL,
        remote_url: remoteURL,
        remote_type: remoteType,
      });
      setShowRemoteModal(false);
      setRemoteName('');
      setRemoteURL('');
      await loadAll();
    } catch (err: any) {
      alert(err.message || 'Remote ingestion failed');
    } finally {
      setAddingRemote(false);
    }
  };

  // Submit Generation
  const handleSubmitGeneration = async (e: React.FormEvent) => {
    e.preventDefault();
    if (selectedTemplates.length === 0) {
      alert('Please select at least one document template');
      return;
    }
    setSubmittingGen(true);
    try {
      await api.submitGeneration(projectId, {
        template_set: selectedTemplates.join(','),
        source_ids: selectedSourceIds,
        llm_config_id: selectedGenLLMId || undefined,
      });
      setShowGenModal(false);
      setActiveTab('generations');
      await loadAll();
    } catch (err: any) {
      alert(err.message || 'Generation submission failed');
    } finally {
      setSubmittingGen(false);
    }
  };

  const handleDeleteSource = async (id: string) => {
    if (!confirm('Are you sure you want to remove this source?')) return;
    try {
      await api.deleteSource(id);
      setSources((prev) => prev.filter((s) => s.id !== id));
    } catch (err: any) {
      alert(err.message || 'Failed to delete source');
    }
  };

  if (loading) {
    return (
      <div className="p-16 text-center text-muted-foreground flex items-center justify-center gap-2">
        <RefreshCw className="h-5 w-5 animate-spin" /> Loading project details...
      </div>
    );
  }

  if (!project) {
    return (
      <div className="p-16 text-center">
        <h2 className="text-lg font-bold">Project not found</h2>
        <button onClick={() => onNavigate('projects')} className="mt-4 text-primary underline">
          Back to Projects
        </button>
      </div>
    );
  }

  return (
    <div className="max-w-7xl mx-auto py-6 px-4 space-y-6">
      {/* Top Header */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 border-b pb-5">
        <div className="space-y-1">
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <button
              onClick={() => onNavigate('projects')}
              className="hover:text-foreground flex items-center gap-1 font-medium"
            >
              <ArrowLeft className="h-3 w-3" /> Projects
            </button>
            <span>/</span>
            <span className="font-semibold text-foreground truncate max-w-xs">{project.name}</span>
          </div>
          <h1 className="text-2xl font-black tracking-tight">{project.name}</h1>
          <p className="text-sm text-muted-foreground">{project.description || 'No description'}</p>
        </div>

        <div className="flex items-center gap-2">
          <button
            onClick={handleOpenGenModal}
            className="inline-flex items-center gap-2 px-4 py-2.5 rounded-lg bg-primary text-primary-foreground font-semibold text-sm shadow hover:opacity-90 transition"
          >
            <Sparkles className="h-4 w-4" /> Request Generation
          </button>
        </div>
      </div>

      {/* Tabs Navigation */}
      <div className="flex border-b text-sm font-medium gap-6">
        <button
          onClick={() => setActiveTab('overview')}
          className={`pb-3 relative transition-colors ${
            activeTab === 'overview'
              ? 'text-primary font-bold border-b-2 border-primary'
              : 'text-muted-foreground hover:text-foreground'
          }`}
        >
          Overview
        </button>

        <button
          onClick={() => setActiveTab('sources')}
          className={`pb-3 relative transition-colors flex items-center gap-1.5 ${
            activeTab === 'sources'
              ? 'text-primary font-bold border-b-2 border-primary'
              : 'text-muted-foreground hover:text-foreground'
          }`}
        >
          Sources ({sources.length})
        </button>

        <button
          onClick={() => setActiveTab('generations')}
          className={`pb-3 relative transition-colors flex items-center gap-1.5 ${
            activeTab === 'generations'
              ? 'text-primary font-bold border-b-2 border-primary'
              : 'text-muted-foreground hover:text-foreground'
          }`}
        >
          Generations ({generations.length})
        </button>

        <button
          onClick={() => setActiveTab('documents')}
          className={`pb-3 relative transition-colors flex items-center gap-1.5 ${
            activeTab === 'documents'
              ? 'text-primary font-bold border-b-2 border-primary'
              : 'text-muted-foreground hover:text-foreground'
          }`}
        >
          Documents ({documents.length})
        </button>

        <button
          onClick={() => {
            setActiveTab('confluence');
            if (confluenceSpaces.length === 0) {
              loadSpaces();
            }
          }}
          className={`pb-3 relative transition-colors flex items-center gap-1.5 ${
            activeTab === 'confluence'
              ? 'text-primary font-bold border-b-2 border-primary'
              : 'text-muted-foreground hover:text-foreground'
          }`}
        >
          <Globe className="h-4 w-4" /> Confluence Target
        </button>

        <button
          onClick={() => setActiveTab('members')}
          className={`pb-3 relative transition-colors flex items-center gap-1.5 ${
            activeTab === 'members'
              ? 'text-primary font-bold border-b-2 border-primary'
              : 'text-muted-foreground hover:text-foreground'
          }`}
        >
          <Users className="h-4 w-4" /> Members ({members.length})
        </button>
      </div>

      {/* TAB 1: OVERVIEW */}
      {activeTab === 'overview' && (
        <div className="space-y-6">
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <div className="p-5 border rounded-xl bg-card space-y-1">
              <span className="text-xs uppercase font-semibold text-muted-foreground">Sources Ingested</span>
              <div className="text-2xl font-bold">{sources.length}</div>
              <p className="text-xs text-muted-foreground">PDFs, Excel, source files, and remote locations</p>
            </div>

            <div className="p-5 border rounded-xl bg-card space-y-1">
              <span className="text-xs uppercase font-semibold text-muted-foreground">Documentation Drafts</span>
              <div className="text-2xl font-bold">{documents.length}</div>
              <p className="text-xs text-muted-foreground">Synthesized technical artifacts ready for review</p>
            </div>

            {/* ASSOCIATED LLM CARD */}
            <div
              onClick={() => {
                if (isOwnerOrAdmin) {
                  setSelectedLLMId(project?.llm_config_id || '');
                  setShowLLMModal(true);
                }
              }}
              className={`p-5 border rounded-xl bg-card space-y-1 ${
                isOwnerOrAdmin ? 'cursor-pointer hover:border-primary/40 hover:shadow-sm transition' : ''
              }`}
            >
              <div className="flex items-center justify-between">
                <span className="text-xs uppercase font-semibold text-muted-foreground">Associated LLM</span>
                {isOwnerOrAdmin && (
                  <span className="text-[10px] text-primary font-semibold hover:underline">
                    Configure
                  </span>
                )}
              </div>
              <div className="text-base font-bold flex items-center gap-1.5 truncate">
                <Cpu className="h-4 w-4 text-indigo-500 shrink-0" />
                <span className="truncate">
                  {project?.llm_model ? project.llm_model : 'System Default'}
                </span>
              </div>
              <p className="text-xs text-muted-foreground truncate">
                {project?.llm_provider
                  ? `Provider: ${project.llm_provider.toUpperCase()}`
                  : 'Uses system active default LLM'}
              </p>
            </div>

            <div
              onClick={() => {
                setActiveTab('confluence');
                if (confluenceSpaces.length === 0) {
                  loadSpaces();
                }
              }}
              className="p-5 border rounded-xl bg-card space-y-1 cursor-pointer hover:border-primary/40 hover:shadow-sm transition"
            >
              <span className="text-xs uppercase font-semibold text-muted-foreground">Confluence Target</span>
              <div className="text-base font-bold flex items-center gap-1.5 truncate">
                {confluenceTarget ? (
                  <>
                    <Globe className="h-4 w-4 text-primary shrink-0" />
                    <span>Space: {confluenceTarget.space_key}</span>
                  </>
                ) : (
                  <span className="text-muted-foreground text-sm font-normal">Not configured</span>
                )}
              </div>
              <p className="text-xs text-muted-foreground truncate">
                {confluenceTarget ? confluenceTarget.space_name : 'Click to configure target'}
              </p>
            </div>
          </div>

          {/* TOKEN CONSUMPTION & CHARGEBACK SUMMARY */}
          <div className="p-6 border rounded-xl bg-card space-y-4">
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-2 border-b pb-4">
              <div>
                <h3 className="font-bold text-base flex items-center gap-2">
                  <Zap className="h-5 w-5 text-amber-500" /> Token Usage & Chargeback
                </h3>
                <p className="text-xs text-muted-foreground mt-0.5">
                  Cumulative LLM token consumption across all multi-stage synthesis and feedback revisions for project chargeback
                </p>
              </div>
              <div className="flex items-center gap-3">
                <span className="text-xs text-muted-foreground">
                  Events: <strong className="text-foreground">{tokenSummary?.events_count || 0}</strong>
                </span>
                {tokenSummary && tokenSummary.events_count > 0 && (
                  <button
                    onClick={() => setShowTokenLedgerModal(true)}
                    className="text-xs px-3 py-1.5 rounded-lg border font-medium hover:bg-muted transition flex items-center gap-1.5"
                  >
                    View Audit Ledger
                  </button>
                )}
              </div>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
              <div className="p-4 rounded-xl bg-muted/40 border space-y-1">
                <span className="text-xs font-semibold uppercase text-muted-foreground">Total Tokens Used</span>
                <div className="text-2xl font-black text-foreground">
                  {(tokenSummary?.total_tokens || project?.total_tokens || 0).toLocaleString()}
                </div>
                <div className="text-[11px] text-muted-foreground flex items-center gap-2">
                  <span>Input: <strong>{(tokenSummary?.total_input_tokens || 0).toLocaleString()}</strong></span>
                  <span>•</span>
                  <span>Output: <strong>{(tokenSummary?.total_output_tokens || 0).toLocaleString()}</strong></span>
                </div>
              </div>

              <div className="p-4 rounded-xl bg-muted/40 border space-y-2">
                <span className="text-xs font-semibold uppercase text-muted-foreground">Usage by Operation</span>
                {tokenSummary && Object.keys(tokenSummary.by_operation || {}).length > 0 ? (
                  <div className="space-y-1 text-xs">
                    {Object.entries(tokenSummary.by_operation).map(([op, tok]) => (
                      <div key={op} className="flex items-center justify-between">
                        <span className="text-muted-foreground capitalize">
                          {op.replace(/_/g, ' ')}
                        </span>
                        <span className="font-mono font-medium">{tok.toLocaleString()}</span>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="text-xs text-muted-foreground">No generation activity yet.</p>
                )}
              </div>

              <div className="p-4 rounded-xl bg-muted/40 border space-y-2">
                <span className="text-xs font-semibold uppercase text-muted-foreground">Usage by Model</span>
                {tokenSummary && Object.keys(tokenSummary.by_model || {}).length > 0 ? (
                  <div className="space-y-1 text-xs">
                    {Object.entries(tokenSummary.by_model).map(([model, tok]) => (
                      <div key={model} className="flex items-center justify-between">
                        <span className="text-muted-foreground truncate max-w-[140px]" title={model}>
                          {model}
                        </span>
                        <span className="font-mono font-medium">{tok.toLocaleString()}</span>
                      </div>
                    ))}
                  </div>
                ) : (
                  <p className="text-xs text-muted-foreground">No model activity yet.</p>
                )}
              </div>
            </div>
          </div>

          <div className="p-6 border rounded-xl bg-card space-y-4">
            <h3 className="font-bold text-base">Quick Start Guide</h3>
            <div className="grid grid-cols-1 md:grid-cols-4 gap-4 text-sm">
              <div className="p-4 rounded-lg bg-secondary/50 space-y-2">
                <div className="font-semibold flex items-center gap-1.5 text-primary">
                  1. Ingest Sources
                </div>
                <p className="text-xs text-muted-foreground">
                  Upload API specs, code archives, PDFs, or connect remote Git repositories.
                </p>
              </div>

              <div className="p-4 rounded-lg bg-secondary/50 space-y-2">
                <div className="font-semibold flex items-center gap-1.5 text-primary">
                  2. Run Generation
                </div>
                <p className="text-xs text-muted-foreground">
                  Select templates (Spec, ADR, Release Notes, Runbook) and trigger AI synthesis.
                </p>
              </div>

              <div className="p-4 rounded-lg bg-secondary/50 space-y-2">
                <div className="font-semibold flex items-center gap-1.5 text-primary">
                  3. Human Review
                </div>
                <p className="text-xs text-muted-foreground">
                  Capture structured feedback, flag blocking items, and trigger AI revision.
                </p>
              </div>

              <div className="p-4 rounded-lg bg-secondary/50 space-y-2">
                <div className="font-semibold flex items-center gap-1.5 text-primary">
                  4. Sign-Off & Export
                </div>
                <p className="text-xs text-muted-foreground">
                  Approve once blocking items are resolved, then publish to Confluence or download Markdown.
                </p>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* TAB 2: SOURCES */}
      {activeTab === 'sources' && (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-bold">Project Ingested Sources</h2>
            <div className="flex items-center gap-2">
              <button
                onClick={() => setShowUploadModal(true)}
                className="inline-flex items-center gap-1.5 px-3 py-2 rounded-lg bg-primary text-primary-foreground text-xs font-semibold shadow hover:opacity-90"
              >
                <UploadCloud className="h-4 w-4" /> Upload File
              </button>
              <button
                onClick={() => setShowRemoteModal(true)}
                className="inline-flex items-center gap-1.5 px-3 py-2 rounded-lg border text-xs font-semibold hover:bg-muted"
              >
                <LinkIcon className="h-4 w-4" /> Add Remote Location
              </button>
            </div>
          </div>

          {sources.length === 0 ? (
            <div className="p-12 text-center border rounded-xl bg-card space-y-3">
              <UploadCloud className="h-8 w-8 text-muted-foreground mx-auto" />
              <h3 className="font-semibold text-sm">No sources added yet</h3>
              <p className="text-xs text-muted-foreground max-w-sm mx-auto">
                Add technical files (PDF, Excel, Markdown, Code, ZIP) or remote URLs so the AI has grounding evidence.
              </p>
            </div>
          ) : (
            <div className="border rounded-xl bg-card overflow-hidden">
              <table className="w-full text-left text-sm">
                <thead className="bg-muted/50 border-b text-xs uppercase font-semibold text-muted-foreground">
                  <tr>
                    <th className="py-3 px-4">Name / Filename</th>
                    <th className="py-3 px-4">Type</th>
                    <th className="py-3 px-4">MIME / Details</th>
                    <th className="py-3 px-4">Size</th>
                    <th className="py-3 px-4">Status</th>
                    <th className="py-3 px-4 text-right">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y text-xs">
                  {sources.map((s) => (
                    <tr key={s.id} className="hover:bg-muted/30">
                      <td className="py-3 px-4 font-semibold text-foreground flex items-center gap-2">
                        {s.type === 'upload' ? (
                          <FileCode className="h-4 w-4 text-blue-500" />
                        ) : (
                          <Globe className="h-4 w-4 text-emerald-500" />
                        )}
                        <span className="truncate max-w-xs">{s.name}</span>
                      </td>
                      <td className="py-3 px-4 uppercase text-[11px] font-mono text-muted-foreground">
                        {s.type} {s.remote_type ? `(${s.remote_type})` : ''}
                      </td>
                      <td className="py-3 px-4 text-muted-foreground truncate max-w-xs">
                        {s.mime_type || 'text/plain'}
                      </td>
                      <td className="py-3 px-4 text-muted-foreground">
                        {s.size_bytes ? `${Math.round(s.size_bytes / 1024)} KB` : 'N/A'}
                      </td>
                      <td className="py-3 px-4">
                        <span className="px-2 py-0.5 rounded-full text-[10px] font-bold uppercase bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300">
                          {s.status}
                        </span>
                      </td>
                      <td className="py-3 px-4 text-right space-x-2">
                        <button
                          onClick={() => setPreviewSource(s)}
                          className="px-2 py-1 rounded border text-[11px] font-medium hover:bg-muted"
                        >
                          View Extracted Text
                        </button>
                        <button
                          onClick={() => handleDeleteSource(s.id)}
                          className="px-2 py-1 rounded border border-destructive/20 text-destructive text-[11px] font-medium hover:bg-destructive/10"
                        >
                          Delete
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* TAB 3: GENERATIONS */}
      {activeTab === 'generations' && (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-bold">Generation Run History</h2>
            <button
              onClick={handleOpenGenModal}
              className="inline-flex items-center gap-1.5 px-3 py-2 rounded-lg bg-primary text-primary-foreground text-xs font-semibold shadow hover:opacity-90"
            >
              <Sparkles className="h-4 w-4" /> Request Generation
            </button>
          </div>

          {generations.length === 0 ? (
            <div className="p-12 text-center border rounded-xl bg-card space-y-3">
              <Sparkles className="h-8 w-8 text-muted-foreground mx-auto" />
              <h3 className="font-semibold text-sm">No documentation generations run yet</h3>
              <p className="text-xs text-muted-foreground max-w-sm mx-auto">
                Submit a generation request to have the LLM synthesize Feature Specs, ADRs, Release Notes, and Runbooks from your sources.
              </p>
            </div>
          ) : (
            <div className="border rounded-xl bg-card overflow-hidden">
              <table className="w-full text-left text-sm">
                <thead className="bg-muted/50 border-b text-xs uppercase font-semibold text-muted-foreground">
                  <tr>
                    <th className="py-3 px-4">Status</th>
                    <th className="py-3 px-4">Template Set</th>
                    <th className="py-3 px-4">Model Used</th>
                    <th className="py-3 px-4">Tokens Used</th>
                    <th className="py-3 px-4">Requested By</th>
                    <th className="py-3 px-4">Requested At</th>
                    <th className="py-3 px-4">Completed At</th>
                    <th className="py-3 px-4 text-right">Pipeline</th>
                  </tr>
                </thead>
                <tbody className="divide-y text-xs">
                  {generations.map((g) => (
                    <tr key={g.id} className="hover:bg-muted/30">
                      <td className="py-3 px-4">
                        <span
                          className={`px-2 py-0.5 rounded-full text-[10px] font-bold uppercase ${
                            g.status === 'completed'
                              ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300'
                              : g.status === 'processing'
                              ? 'bg-blue-100 text-blue-800 animate-pulse'
                              : g.status === 'failed'
                              ? 'bg-red-100 text-red-800'
                              : 'bg-amber-100 text-amber-800'
                          }`}
                        >
                          {g.status}
                        </span>
                      </td>
                      <td className="py-3 px-4 font-mono text-[11px] font-semibold text-foreground">
                        {g.template_set}
                      </td>
                      <td className="py-3 px-4 text-muted-foreground">
                        {g.model_used || 'Pending...'}
                      </td>
                      <td className="py-3 px-4 font-mono text-muted-foreground">
                        {g.total_tokens ? `${g.total_tokens.toLocaleString()} tok` : (g.status === 'completed' ? '—' : '...')}
                      </td>
                      <td className="py-3 px-4 text-muted-foreground">
                        {g.requester_name || 'User'}
                      </td>
                      <td className="py-3 px-4 text-muted-foreground">
                        {new Date(g.created_at).toLocaleTimeString()}
                      </td>
                      <td className="py-3 px-4 text-muted-foreground">
                        {g.completed_at ? new Date(g.completed_at).toLocaleTimeString() : '—'}
                      </td>
                      <td className="py-3 px-4 text-right space-x-1.5 whitespace-nowrap">
                        <button
                          onClick={() => handleViewStages(g)}
                          className="px-2 py-1 rounded border text-[11px] font-medium hover:bg-muted inline-flex items-center gap-1"
                          title="Inspect Multi-Stage Pipeline Stages"
                        >
                          <Activity className="h-3 w-3" /> Stages
                        </button>
                        {g.system_map_md && (
                          <button
                            onClick={() => setShowSystemMapModal(g.system_map_md || null)}
                            className="px-2 py-1 rounded border border-primary/30 text-primary text-[11px] font-medium hover:bg-primary/10 inline-flex items-center gap-1"
                            title="View Stage 1 Synthesized Architecture System Map"
                          >
                            <Compass className="h-3 w-3" /> System Map
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {/* TAB 4: DOCUMENTS */}
      {activeTab === 'documents' && (
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h2 className="text-lg font-bold">Synthesized Documents</h2>
            <button
              onClick={handleOpenGenModal}
              className="inline-flex items-center gap-1.5 px-3 py-2 rounded-lg bg-primary text-primary-foreground text-xs font-semibold shadow hover:opacity-90"
            >
              <Sparkles className="h-4 w-4" /> Synthesize New Drafts
            </button>
          </div>

          {documents.length === 0 ? (
            <div className="p-12 text-center border rounded-xl bg-card space-y-3">
              <FileText className="h-8 w-8 text-muted-foreground mx-auto" />
              <h3 className="font-semibold text-sm">No documents generated yet</h3>
              <p className="text-xs text-muted-foreground max-w-sm mx-auto">
                Once a generation job completes, your synthesized draft documents will appear here for human review.
              </p>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {documents.map((doc) => (
                <div
                  key={doc.id}
                  className="p-5 border rounded-xl bg-card hover:border-primary/40 hover:shadow-md transition flex flex-col justify-between space-y-4"
                >
                  <div className="space-y-2">
                    <div className="flex items-center justify-between">
                      <span className="text-[10px] uppercase font-bold tracking-wider px-2 py-0.5 rounded-full bg-secondary text-secondary-foreground font-mono">
                        {doc.template_type}
                      </span>
                      <span
                        className={`text-[10px] uppercase font-bold px-2 py-0.5 rounded-full ${
                          doc.status === 'approved'
                            ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300'
                            : 'bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300'
                        }`}
                      >
                        {doc.status}
                      </span>
                    </div>

                    <h3 className="font-bold text-base leading-snug">{doc.title}</h3>
                    <p className="text-xs text-muted-foreground line-clamp-3">
                      {doc.content_md.slice(0, 200)}...
                    </p>
                  </div>

                  <div className="pt-3 border-t flex items-center justify-between text-xs">
                    <span className="text-muted-foreground">
                      Updated: {new Date(doc.updated_at).toLocaleDateString()}
                    </span>
                    <div className="flex items-center gap-2">
                      <a
                        href={api.getDownloadURL(doc.id)}
                        target="_blank"
                        rel="noreferrer"
                        title="Download Markdown"
                        className="p-1.5 rounded border hover:bg-muted text-muted-foreground hover:text-foreground"
                      >
                        <Download className="h-3.5 w-3.5" />
                      </a>
                      <button
                        onClick={() => onNavigate('document-review', doc.id)}
                        className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg bg-primary text-primary-foreground font-semibold text-xs shadow hover:opacity-90"
                      >
                        <Eye className="h-3.5 w-3.5" /> Review Document
                      </button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* TAB 5: CONFLUENCE TARGET */}
      {activeTab === 'confluence' && (
        <div className="space-y-6">
          <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
            <div>
              <h2 className="text-lg font-bold flex items-center gap-2">
                <Globe className="h-5 w-5 text-primary" /> Project Confluence Target
              </h2>
              <p className="text-xs text-muted-foreground mt-0.5">
                Configure the Confluence Cloud space and parent page where approved documentation is published.
                All publications use the exporting reviewer's authenticated Atlassian SSO session.
              </p>
            </div>
            {isOwnerOrAdmin && confluenceTarget && !isEditingConfluenceTarget && (
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={() => {
                    setIsEditingConfluenceTarget(true);
                    if (confluenceTarget) {
                      setSelectedSpaceKey(confluenceTarget.space_key);
                      setSelectedParentPageId(confluenceTarget.parent_page_id || '');
                      setManualPageId(confluenceTarget.parent_page_id || '');
                      setManualPageTitle(confluenceTarget.parent_page_title || '');
                      if (confluenceTarget.parent_page_id) {
                        setUseManualPageId(true);
                      }
                    }
                    if (confluenceSpaces.length === 0) loadSpaces();
                  }}
                  className="px-3 py-1.5 rounded-lg border text-xs font-semibold hover:bg-muted"
                >
                  Change Target
                </button>
                <button
                  type="button"
                  onClick={handleDisconnectConfluence}
                  className="px-3 py-1.5 rounded-lg border border-red-200 text-red-600 hover:bg-red-50 dark:hover:bg-red-950/30 text-xs font-semibold flex items-center gap-1.5"
                >
                  <Trash2 className="h-3.5 w-3.5" /> Disconnect
                </button>
              </div>
            )}
          </div>

          {confluenceMsg && (
            <div
              className={`p-3 rounded-lg text-xs flex items-center gap-2 border ${
                confluenceMsg.type === 'success'
                  ? 'bg-emerald-50 dark:bg-emerald-950/40 text-emerald-800 dark:text-emerald-300 border-emerald-200 dark:border-emerald-900'
                  : 'bg-red-50 dark:bg-red-950/40 text-red-800 dark:text-red-300 border-red-200 dark:border-red-900'
              }`}
            >
              {confluenceMsg.type === 'success' ? (
                <CheckCircle2 className="h-4 w-4 shrink-0" />
              ) : (
                <AlertCircle className="h-4 w-4 shrink-0" />
              )}
              <span>{confluenceMsg.text}</span>
            </div>
          )}

          {confluenceTarget && !isEditingConfluenceTarget ? (
            <div className="border rounded-xl bg-card p-6 space-y-4">
              <div className="flex items-center justify-between pb-4 border-b">
                <div className="flex items-center gap-2">
                  <span className="h-2.5 w-2.5 rounded-full bg-emerald-500" />
                  <span className="font-semibold text-sm">Target Connected</span>
                </div>
                <span className="text-[11px] text-muted-foreground font-mono">
                  Space Key: {confluenceTarget.space_key}
                </span>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-3 gap-4 pt-1">
                <div className="space-y-1">
                  <span className="text-xs uppercase font-semibold text-muted-foreground">Target Space</span>
                  <div className="font-medium text-sm">{confluenceTarget.space_name}</div>
                  <div className="text-xs text-muted-foreground font-mono">Key: {confluenceTarget.space_key}</div>
                </div>

                <div className="space-y-1">
                  <span className="text-xs uppercase font-semibold text-muted-foreground">Parent Page</span>
                  <div className="font-medium text-sm">
                    {confluenceTarget.parent_page_title ||
                      (confluenceTarget.parent_page_id ? `Page ID: ${confluenceTarget.parent_page_id}` : 'Root of Space (No Parent)')}
                  </div>
                  {confluenceTarget.parent_page_id && (
                    <div className="text-xs text-muted-foreground font-mono">ID: {confluenceTarget.parent_page_id}</div>
                  )}
                </div>

                <div className="space-y-1">
                  <span className="text-xs uppercase font-semibold text-muted-foreground">Last Updated</span>
                  <div className="font-medium text-sm">
                    {confluenceTarget.updated_at ? new Date(confluenceTarget.updated_at).toLocaleString() : 'Recently'}
                  </div>
                  <div className="text-xs text-muted-foreground">Project Default Target</div>
                </div>
              </div>
            </div>
          ) : !isOwnerOrAdmin ? (
            <div className="border rounded-xl bg-card p-6 text-center space-y-2">
              <Globe className="h-8 w-8 text-muted-foreground mx-auto" />
              <h3 className="font-semibold text-sm">No Confluence Target Configured</h3>
              <p className="text-xs text-muted-foreground max-w-md mx-auto">
                Only the project owner or system administrators can configure the team's default Confluence target space.
              </p>
            </div>
          ) : (
            <div className="border rounded-xl bg-card p-6 space-y-6">
              <div className="flex items-center justify-between pb-3 border-b">
                <div>
                  <h3 className="font-semibold text-sm">
                    {confluenceTarget ? 'Update Confluence Target' : 'Configure Confluence Target'}
                  </h3>
                  <p className="text-xs text-muted-foreground">
                    Select the space and optional parent page from your Atlassian Cloud organization.
                  </p>
                </div>
                {confluenceTarget && (
                  <button
                    type="button"
                    onClick={() => setIsEditingConfluenceTarget(false)}
                    className="text-xs text-muted-foreground hover:text-foreground"
                  >
                    Cancel
                  </button>
                )}
              </div>

              <form onSubmit={handleSaveConfluence} className="space-y-5">
                {/* Space Picker */}
                <div className="space-y-2">
                  <div className="flex items-center justify-between">
                    <label className="text-xs font-semibold uppercase text-muted-foreground">
                      Confluence Space <span className="text-red-500">*</span>
                    </label>
                    <div className="flex items-center gap-3">
                      {confluenceSpaces.length > 0 && (
                        <button
                          type="button"
                          onClick={() => setUseManualSpaceKey(!useManualSpaceKey)}
                          className="text-xs text-primary hover:underline font-medium"
                        >
                          {useManualSpaceKey ? 'Choose from Space List' : 'Enter Space Key Manually'}
                        </button>
                      )}
                      <button
                        type="button"
                        onClick={loadSpaces}
                        disabled={loadingSpaces}
                        className="text-xs text-primary hover:underline flex items-center gap-1 font-medium"
                      >
                        <RefreshCw className={`h-3 w-3 ${loadingSpaces ? 'animate-spin' : ''}`} /> Reload Spaces
                      </button>
                    </div>
                  </div>

                  {loadingSpaces ? (
                    <div className="p-3 text-xs text-muted-foreground flex items-center gap-2 border rounded-lg">
                      <RefreshCw className="h-3.5 w-3.5 animate-spin" /> Loading accessible Confluence spaces...
                    </div>
                  ) : useManualSpaceKey || confluenceSpaces.length === 0 ? (
                    <div className="space-y-2">
                      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                        <div className="space-y-1">
                          <label className="text-[11px] font-medium text-muted-foreground">
                            Space Key <span className="text-red-500">*</span>
                          </label>
                          <input
                            type="text"
                            value={selectedSpaceKey}
                            onChange={(e) => {
                              const val = e.target.value.toUpperCase();
                              setSelectedSpaceKey(val);
                              handleSpaceChange(val);
                            }}
                            placeholder="e.g. ENG, DOCS, PROD"
                            required
                            className="w-full px-3 py-2 rounded-lg border bg-background text-xs font-mono uppercase focus:outline-none focus:ring-1 focus:ring-primary"
                          />
                        </div>
                        <div className="space-y-1">
                          <label className="text-[11px] font-medium text-muted-foreground">Space Name (Optional)</label>
                          <input
                            type="text"
                            value={manualSpaceName}
                            onChange={(e) => setManualSpaceName(e.target.value)}
                            placeholder="e.g. Engineering & Architecture"
                            className="w-full px-3 py-2 rounded-lg border bg-background text-xs focus:outline-none focus:ring-1 focus:ring-primary"
                          />
                        </div>
                      </div>
                      {confluenceSpaces.length === 0 && (
                        <div className="p-2.5 rounded-lg border bg-muted/40 text-[11px] text-muted-foreground space-y-2">
                          <div className="font-semibold text-foreground flex items-center justify-between">
                            <span className="flex items-center gap-1.5">
                              <Info className="h-3.5 w-3.5 text-blue-500" />
                              Space List Not Available
                            </span>
                            <div className="flex items-center gap-2">
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
                                className="text-primary hover:underline font-semibold text-[11px] flex items-center gap-1"
                              >
                                <RefreshCw className="h-3 w-3" /> Reconnect Session
                              </button>
                              <button
                                type="button"
                                onClick={loadSpaces}
                                className="text-muted-foreground hover:underline font-medium text-[11px]"
                              >
                                Reload
                              </button>
                            </div>
                          </div>
                          <p>
                            Enter your Confluence Space Key (e.g. <code className="font-mono bg-muted px-1 py-0.5 rounded">ProjectDoc</code>). If your Atlassian session expired or was disconnected, click <strong>Reconnect Session</strong> to re-authenticate with granted scopes.
                          </p>
                        </div>
                      )}
                    </div>
                  ) : (
                    <select
                      value={selectedSpaceKey}
                      onChange={(e) => handleSpaceChange(e.target.value)}
                      required
                      className="w-full px-3 py-2 rounded-lg border bg-background text-xs focus:outline-none focus:ring-1 focus:ring-primary"
                    >
                      <option value="">-- Select a Confluence Space --</option>
                      {confluenceSpaces.map((s) => (
                        <option key={s.key} value={s.key}>
                          {s.name} ({s.key})
                        </option>
                      ))}
                    </select>
                  )}
                </div>

                {/* Parent Page Picker */}
                {selectedSpaceKey && (
                  <div className="space-y-3 pt-2 border-t">
                    <div className="flex items-center justify-between">
                      <label className="text-xs font-semibold uppercase text-muted-foreground">
                        Parent Page in Space
                      </label>
                      <button
                        type="button"
                        onClick={() => setUseManualPageId(!useManualPageId)}
                        className="text-xs text-primary hover:underline font-medium"
                      >
                        {useManualPageId ? 'Choose from Page List' : 'Enter Page ID Manually'}
                      </button>
                    </div>

                    {useManualPageId || confluencePages.length === 0 ? (
                      <div className="space-y-2">
                        <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                          <div className="space-y-1">
                            <label className="text-[11px] font-medium text-muted-foreground">
                              Parent Page ID or Confluence URL
                            </label>
                            <input
                              type="text"
                              value={manualPageId}
                              onChange={(e) => {
                                const val = e.target.value;
                                const urlMatch = val.match(/\/pages\/(\d+)(?:\/([^/?#]+))?/);
                                if (urlMatch) {
                                  setManualPageId(urlMatch[1]);
                                  if (urlMatch[2] && !manualPageTitle) {
                                    setManualPageTitle(decodeURIComponent(urlMatch[2].replace(/\+/g, ' ')));
                                  }
                                } else {
                                  setManualPageId(val);
                                }
                              }}
                              placeholder="e.g. 25001985 or paste page URL"
                              className="w-full px-3 py-2 rounded-lg border bg-background text-xs font-mono focus:outline-none focus:ring-1 focus:ring-primary"
                            />
                          </div>
                          <div className="space-y-1">
                            <label className="text-[11px] font-medium text-muted-foreground">
                              Parent Page Title (Optional)
                            </label>
                            <input
                              type="text"
                              value={manualPageTitle}
                              onChange={(e) => setManualPageTitle(e.target.value)}
                              placeholder="e.g. Product Architecture"
                              className="w-full px-3 py-2 rounded-lg border bg-background text-xs focus:outline-none focus:ring-1 focus:ring-primary"
                            />
                          </div>
                        </div>
                        <p className="text-[11px] text-muted-foreground">
                          Enter the numeric Page ID (or paste the Confluence page URL) to publish underneath an existing page. Leave blank to publish at the root of the space.
                        </p>
                      </div>
                    ) : (
                      <div className="space-y-2">
                        {loadingPages ? (
                          <div className="p-3 text-xs text-muted-foreground flex items-center gap-2 border rounded-lg">
                            <RefreshCw className="h-3.5 w-3.5 animate-spin" /> Fetching pages for space {selectedSpaceKey}...
                          </div>
                        ) : (
                          <>
                            <div className="flex items-center gap-2">
                              <input
                                type="text"
                                value={parentPageSearch}
                                onChange={(e) => setParentPageSearch(e.target.value)}
                                placeholder="Search pages in this space..."
                                className="w-full px-3 py-1.5 rounded-lg border bg-background text-xs focus:outline-none focus:ring-1 focus:ring-primary"
                              />
                            </div>

                            <div className="max-h-48 overflow-y-auto space-y-1 border rounded-lg p-2 bg-background">
                              <label className="flex items-center gap-2 p-2 rounded hover:bg-muted cursor-pointer text-xs">
                                <input
                                  type="radio"
                                  name="parentPage"
                                  checked={!selectedParentPageId}
                                  onChange={() => setSelectedParentPageId('')}
                                  className="rounded-full"
                                />
                                <span className="font-semibold text-foreground">
                                  Root of Space (No Parent Page)
                                </span>
                              </label>

                              {confluencePages
                                .filter((p) =>
                                  !parentPageSearch || p.title.toLowerCase().includes(parentPageSearch.toLowerCase())
                                )
                                .map((p) => (
                                  <label
                                    key={p.id}
                                    className="flex items-center gap-2 p-2 rounded hover:bg-muted cursor-pointer text-xs"
                                  >
                                    <input
                                      type="radio"
                                      name="parentPage"
                                      checked={selectedParentPageId === p.id}
                                      onChange={() => {
                                        setSelectedParentPageId(p.id);
                                        setManualPageId(p.id);
                                        setManualPageTitle(p.title);
                                      }}
                                      className="rounded-full"
                                    />
                                    <span className="truncate flex-1">{p.title}</span>
                                    <span className="text-[10px] text-muted-foreground font-mono">ID: {p.id}</span>
                                  </label>
                                ))}

                              {confluencePages.length === 0 && (
                                <div className="p-2 text-xs text-muted-foreground">
                                  No pages found in this space. Documents will be published at the space root.
                                </div>
                              )}
                            </div>
                          </>
                        )}
                      </div>
                    )}
                  </div>
                )}

                <div className="flex items-center justify-end gap-2 pt-4 border-t">
                  {confluenceTarget && (
                    <button
                      type="button"
                      onClick={() => setIsEditingConfluenceTarget(false)}
                      className="px-4 py-2 rounded-lg border text-xs font-semibold hover:bg-muted"
                    >
                      Cancel
                    </button>
                  )}
                  <button
                    type="submit"
                    disabled={savingConfluenceTarget || !selectedSpaceKey}
                    className="px-4 py-2 rounded-lg bg-primary text-primary-foreground text-xs font-semibold shadow hover:opacity-90 disabled:opacity-50 flex items-center gap-1.5"
                  >
                    {savingConfluenceTarget ? (
                      <>
                        <RefreshCw className="h-3.5 w-3.5 animate-spin" /> Saving Target...
                      </>
                    ) : (
                      'Save Confluence Target'
                    )}
                  </button>
                </div>
              </form>
            </div>
          )}
        </div>
      )}

      {/* TAB 6: MEMBERS */}
      {activeTab === 'members' && (
        <div className="space-y-6">
          <div className="flex flex-col md:flex-row md:items-center justify-between gap-4">
            <div>
              <h2 className="text-lg font-bold flex items-center gap-2">
                <Users className="h-5 w-5 text-primary" /> Project Members & Access
              </h2>
              <p className="text-xs text-muted-foreground mt-0.5">
                Manage team members who can view, generate, review, and export documentation for this project.
              </p>
            </div>
          </div>

          {memberMsg && (
            <div
              className={`p-3 rounded-lg text-xs flex items-center gap-2 border ${
                memberMsg.type === 'success'
                  ? 'bg-emerald-50 dark:bg-emerald-950/40 text-emerald-800 dark:text-emerald-300 border-emerald-200 dark:border-emerald-900'
                  : 'bg-red-50 dark:bg-red-950/40 text-red-800 dark:text-red-300 border-red-200 dark:border-red-900'
              }`}
            >
              {memberMsg.type === 'success' ? (
                <CheckCircle2 className="h-4 w-4 shrink-0" />
              ) : (
                <AlertCircle className="h-4 w-4 shrink-0" />
              )}
              <span>{memberMsg.text}</span>
            </div>
          )}

          {/* Invite Member Card (Owners / Admins only) */}
          {isOwnerOrAdmin && (
            <div className="border rounded-xl bg-card p-5 space-y-4">
              <h3 className="font-semibold text-sm flex items-center gap-2">
                <UserPlus className="h-4 w-4 text-primary" /> Invite Colleague
              </h3>
              <form onSubmit={handleInviteMember} className="flex flex-col sm:flex-row gap-3 items-end">
                <div className="flex-1 space-y-1 w-full">
                  <label className="text-xs font-semibold uppercase text-muted-foreground">Email Address</label>
                  <input
                    type="email"
                    required
                    value={inviteEmail}
                    onChange={(e) => setInviteEmail(e.target.value)}
                    placeholder="colleague@example.com"
                    className="w-full px-3 py-2 rounded-lg border bg-background text-xs focus:outline-none focus:ring-1 focus:ring-primary"
                  />
                </div>

                <div className="w-full sm:w-40 space-y-1">
                  <label className="text-xs font-semibold uppercase text-muted-foreground">Project Role</label>
                  <select
                    value={inviteRole}
                    onChange={(e) => setInviteRole(e.target.value as ProjectMemberRole)}
                    className="w-full px-3 py-2 rounded-lg border bg-background text-xs focus:outline-none focus:ring-1 focus:ring-primary"
                  >
                    <option value="member">Member</option>
                    <option value="owner">Owner</option>
                  </select>
                </div>

                <button
                  type="submit"
                  disabled={invitingMember || !inviteEmail.trim()}
                  className="px-4 py-2 rounded-lg bg-primary text-primary-foreground text-xs font-semibold shadow hover:opacity-90 disabled:opacity-50 flex items-center gap-1.5 shrink-0"
                >
                  {invitingMember ? (
                    <>
                      <RefreshCw className="h-3.5 w-3.5 animate-spin" /> Inviting...
                    </>
                  ) : (
                    <>
                      <UserPlus className="h-3.5 w-3.5" /> Add Member
                    </>
                  )}
                </button>
              </form>
              <p className="text-[11px] text-muted-foreground">
                Colleagues who have not yet logged in will be added with pending status and automatically linked upon signing in with their Atlassian SSO account.
              </p>
            </div>
          )}

          {/* Members Table */}
          <div className="border rounded-xl bg-card overflow-hidden">
            <div className="p-4 border-b bg-muted/20 flex items-center justify-between">
              <span className="font-semibold text-xs uppercase tracking-wider text-muted-foreground">
                Team Members ({members.length})
              </span>
            </div>

            <div className="divide-y text-xs">
              {members.map((m) => {
                const isProjectCreator = m.user_id === project.owner_id;
                const isCurrentUser = user && m.user_id === user.id;

                return (
                  <div key={m.id} className="p-4 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                    <div className="space-y-1">
                      <div className="flex items-center gap-2">
                        <span className="font-semibold text-foreground text-sm">
                          {m.user_name || m.user_email}
                        </span>
                        {isCurrentUser && (
                          <span className="text-[10px] px-1.5 py-0.5 rounded bg-muted text-muted-foreground">
                            You
                          </span>
                        )}
                        {isProjectCreator && (
                          <span className="text-[10px] px-1.5 py-0.5 rounded bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300 font-medium">
                            Project Creator
                          </span>
                        )}
                      </div>
                      <div className="text-muted-foreground text-xs">{m.user_email}</div>
                    </div>

                    <div className="flex items-center gap-3">
                      <span
                        className={`text-[10px] uppercase font-bold px-2 py-0.5 rounded-full ${
                          m.role === 'owner'
                            ? 'bg-purple-100 text-purple-800 dark:bg-purple-950 dark:text-purple-300'
                            : 'bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-300'
                        }`}
                      >
                        {m.role}
                      </span>

                      <span
                        className={`text-[10px] uppercase font-bold px-2 py-0.5 rounded-full ${
                          m.status === 'active'
                            ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300'
                            : 'bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300'
                        }`}
                      >
                        {m.status}
                      </span>

                      {isOwnerOrAdmin && !isProjectCreator && (
                        <button
                          type="button"
                          onClick={() => handleRemoveMember(m.id)}
                          className="p-1.5 rounded hover:bg-red-50 text-muted-foreground hover:text-red-600 dark:hover:bg-red-950/40 transition"
                          title="Remove member"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </button>
                      )}
                    </div>
                  </div>
                );
              })}

              {members.length === 0 && (
                <div className="p-8 text-center text-muted-foreground">
                  No members added yet.
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* MODAL: UPLOAD FILE */}
      {showUploadModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm">
          <div className="bg-card w-full max-w-md border rounded-2xl shadow-2xl p-6 space-y-4">
            <h2 className="text-lg font-bold">Upload Source File</h2>
            <form onSubmit={handleUpload} className="space-y-4">
              <div className="border-2 border-dashed rounded-xl p-6 text-center space-y-2 hover:border-primary/50 transition">
                <UploadCloud className="h-8 w-8 text-muted-foreground mx-auto" />
                <div className="text-xs text-muted-foreground">
                  Select a file: PDF, Excel (.xlsx), Markdown (.md), Plain Text, Source Code, ZIP Archive, or Images.
                </div>
                <input
                  type="file"
                  required
                  onChange={(e) => setSelectedFile(e.target.files?.[0] || null)}
                  className="text-xs text-muted-foreground file:mr-2 file:py-1 file:px-3 file:rounded file:border file:text-xs file:font-semibold file:bg-primary file:text-primary-foreground cursor-pointer"
                />
              </div>

              <div className="flex items-center justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowUploadModal(false)}
                  className="px-3.5 py-1.5 rounded-lg border text-xs font-medium hover:bg-muted"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={uploading || !selectedFile}
                  className="px-4 py-1.5 rounded-lg bg-primary text-primary-foreground text-xs font-semibold shadow hover:opacity-90 disabled:opacity-50"
                >
                  {uploading ? 'Extracting & Saving...' : 'Upload & Extract'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* MODAL: ADD REMOTE SOURCE */}
      {showRemoteModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm">
          <div className="bg-card w-full max-w-md border rounded-2xl shadow-2xl p-6 space-y-4">
            <h2 className="text-lg font-bold">Add Remote Source Location</h2>
            <form onSubmit={handleAddRemote} className="space-y-3">
              <div className="space-y-1">
                <label className="text-xs font-semibold uppercase text-muted-foreground">Source Label / Name</label>
                <input
                  type="text"
                  value={remoteName}
                  onChange={(e) => setRemoteName(e.target.value)}
                  placeholder="e.g. Core API Specification"
                  className="w-full px-3 py-2 rounded-lg border bg-background text-xs focus:outline-none focus:ring-1 focus:ring-primary"
                />
              </div>

              <div className="space-y-1">
                <label className="text-xs font-semibold uppercase text-muted-foreground">Location Type</label>
                <select
                  value={remoteType}
                  onChange={(e) => setRemoteType(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg border bg-background text-xs focus:outline-none focus:ring-1 focus:ring-primary"
                >
                  <option value="http">HTTP / HTTPS URL</option>
                  <option value="git">Git Repository URL</option>
                  <option value="local">Server Local Filesystem Path</option>
                  <option value="s3">Amazon S3 URI</option>
                  <option value="azure">Azure Blob URI</option>
                </select>
              </div>

              <div className="space-y-1">
                <label className="text-xs font-semibold uppercase text-muted-foreground">Remote URL / Path</label>
                <input
                  type="text"
                  required
                  value={remoteURL}
                  onChange={(e) => setRemoteURL(e.target.value)}
                  placeholder="https://raw.githubusercontent.com/... or https://..."
                  className="w-full px-3 py-2 rounded-lg border bg-background text-xs focus:outline-none focus:ring-1 focus:ring-primary"
                />
              </div>

              <div className="flex items-center justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowRemoteModal(false)}
                  className="px-3.5 py-1.5 rounded-lg border text-xs font-medium hover:bg-muted"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={addingRemote}
                  className="px-4 py-1.5 rounded-lg bg-primary text-primary-foreground text-xs font-semibold shadow hover:opacity-90 disabled:opacity-50"
                >
                  {addingRemote ? 'Fetching & Extracting...' : 'Add Source'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* MODAL: REQUEST GENERATION */}
      {showGenModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm">
          <div className="bg-card w-full max-w-lg border rounded-2xl shadow-2xl p-6 space-y-4 max-h-[90vh] overflow-y-auto">
            <div className="flex items-center justify-between border-b pb-3">
              <h2 className="text-base font-bold flex items-center gap-2">
                <Sparkles className="h-5 w-5 text-indigo-500" /> Request AI Documentation Generation
              </h2>
              <button
                type="button"
                onClick={() => setShowGenModal(false)}
                className="text-muted-foreground hover:text-foreground p-1 rounded hover:bg-muted"
              >
                <X className="h-4 w-4" />
              </button>
            </div>

            {/* Pre-Flight Architecture Completeness Scorecard */}
            <div className="p-3.5 rounded-xl border bg-muted/30 space-y-2.5">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <ShieldCheck className="h-4 w-4 text-primary" />
                  <span className="font-bold text-xs uppercase tracking-wider text-foreground">
                    Pre-Flight Completeness Check
                  </span>
                </div>
                {loadingCompleteness ? (
                  <span className="text-[11px] text-muted-foreground flex items-center gap-1">
                    <RefreshCw className="h-3 w-3 animate-spin" /> Scanning...
                  </span>
                ) : completeness ? (
                  <span
                    className={`px-2 py-0.5 rounded-full text-[10px] font-bold ${
                      completeness.coverage_score >= 80
                        ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300'
                        : completeness.coverage_score >= 50
                        ? 'bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300'
                        : 'bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300'
                    }`}
                  >
                    {completeness.coverage_score}% Grounding Score
                  </span>
                ) : null}
              </div>

              {completeness && (
                <div className="space-y-2 text-[11px]">
                  <div className="grid grid-cols-2 sm:grid-cols-3 gap-2 pt-1">
                    <div className="flex items-center gap-1.5">
                      {completeness.has_entrypoint ? (
                        <CheckCircle className="h-3.5 w-3.5 text-emerald-600" />
                      ) : (
                        <AlertCircle className="h-3.5 w-3.5 text-amber-600" />
                      )}
                      <span>Entrypoints</span>
                    </div>
                    <div className="flex items-center gap-1.5">
                      {completeness.has_config ? (
                        <CheckCircle className="h-3.5 w-3.5 text-emerald-600" />
                      ) : (
                        <AlertCircle className="h-3.5 w-3.5 text-amber-600" />
                      )}
                      <span>Configuration</span>
                    </div>
                    <div className="flex items-center gap-1.5">
                      {completeness.has_tests ? (
                        <CheckCircle className="h-3.5 w-3.5 text-emerald-600" />
                      ) : (
                        <AlertCircle className="h-3.5 w-3.5 text-amber-600" />
                      )}
                      <span>Tests</span>
                    </div>
                    <div className="flex items-center gap-1.5">
                      {completeness.has_api_endpoints ? (
                        <CheckCircle className="h-3.5 w-3.5 text-emerald-600" />
                      ) : (
                        <AlertCircle className="h-3.5 w-3.5 text-amber-600" />
                      )}
                      <span>Endpoints</span>
                    </div>
                    <div className="flex items-center gap-1.5">
                      {completeness.has_data_models ? (
                        <CheckCircle className="h-3.5 w-3.5 text-emerald-600" />
                      ) : (
                        <AlertCircle className="h-3.5 w-3.5 text-amber-600" />
                      )}
                      <span>Models</span>
                    </div>
                    <div className="flex items-center gap-1.5 text-muted-foreground">
                      <Compass className="h-3.5 w-3.5 text-indigo-500" />
                      <span>{completeness.total_symbols} AST Symbols</span>
                    </div>
                  </div>

                  {completeness.missing_areas && completeness.missing_areas.length > 0 && (
                    <div className="p-2 rounded-lg bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-900 text-amber-800 dark:text-amber-300">
                      <div className="font-semibold flex items-center gap-1">
                        <AlertCircle className="h-3 w-3" /> Flagged Missing Areas:
                      </div>
                      <ul className="list-disc list-inside mt-0.5 space-y-0.5 text-[10px]">
                        {completeness.missing_areas.map((area, idx) => (
                          <li key={idx}>{area}</li>
                        ))}
                      </ul>
                    </div>
                  )}
                </div>
              )}
            </div>

            <form onSubmit={handleSubmitGeneration} className="space-y-4 text-xs">
              {/* LLM Selection for this run */}
              <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <label className="font-semibold uppercase text-muted-foreground block text-[11px]">
                    AI Model for this Run:
                  </label>
                  {project?.llm_model && (
                    <span className="text-[10px] text-muted-foreground">
                      Project default: <strong className="text-foreground">{project.llm_model}</strong>
                    </span>
                  )}
                </div>
                <select
                  value={selectedGenLLMId}
                  onChange={(e) => setSelectedGenLLMId(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg border bg-background text-xs focus:ring-1 focus:ring-primary"
                >
                  <option value="">
                    {project?.llm_model
                      ? `Project Associated: ${project.llm_model} (${project.llm_provider?.toUpperCase()})`
                      : '-- Use System Active Default LLM --'}
                  </option>
                  {availableLLMs.map((cfg) => (
                    <option key={cfg.id} value={cfg.id}>
                      {cfg.provider.toUpperCase()} — {cfg.default_model} {cfg.is_active ? '(Global Default)' : ''}
                    </option>
                  ))}
                </select>
              </div>

              <div className="space-y-2">
                <label className="font-semibold uppercase text-muted-foreground block">
                  Select Document Templates to Synthesize:
                </label>
                <div className="grid grid-cols-2 gap-2">
                  {[
                    { id: 'product_spec', label: 'Feature Specification' },
                    { id: 'adr', label: 'Architecture Decision Record' },
                    { id: 'release_notes', label: 'Release Notes' },
                    { id: 'runbook', label: 'Operations Runbook' },
                  ].map((tmpl) => (
                    <label
                      key={tmpl.id}
                      className="flex items-center gap-2 p-2.5 rounded-lg border bg-background hover:bg-muted/50 cursor-pointer"
                    >
                      <input
                        type="checkbox"
                        checked={selectedTemplates.includes(tmpl.id)}
                        onChange={(e) => {
                          if (e.target.checked) {
                            setSelectedTemplates([...selectedTemplates, tmpl.id]);
                          } else {
                            setSelectedTemplates(selectedTemplates.filter((t) => t !== tmpl.id));
                          }
                        }}
                        className="rounded"
                      />
                      <span className="font-medium text-foreground">{tmpl.label}</span>
                    </label>
                  ))}
                </div>
              </div>

              <div className="space-y-2">
                <div className="flex items-center justify-between">
                  <label className="font-semibold uppercase text-muted-foreground">
                    Included Sources ({selectedSourceIds.length}/{sources.length}):
                  </label>
                  <button
                    type="button"
                    onClick={() => {
                      if (selectedSourceIds.length === (sources || []).length) {
                        setSelectedSourceIds([]);
                      } else {
                        setSelectedSourceIds((sources || []).map((s) => s.id));
                      }
                    }}
                    className="text-primary hover:underline font-medium"
                  >
                    {selectedSourceIds.length === sources.length ? 'Deselect All' : 'Select All'}
                  </button>
                </div>

                <div className="max-h-40 overflow-y-auto space-y-1.5 border rounded-lg p-2 bg-background">
                  {sources.length === 0 ? (
                    <div className="text-muted-foreground p-2">No sources uploaded yet. All project files will be inferred.</div>
                  ) : (
                    sources.map((s) => (
                      <label
                        key={s.id}
                        className="flex items-center gap-2 p-1.5 rounded hover:bg-muted cursor-pointer"
                      >
                        <input
                          type="checkbox"
                          checked={selectedSourceIds.includes(s.id)}
                          onChange={(e) => {
                            if (e.target.checked) {
                              setSelectedSourceIds([...selectedSourceIds, s.id]);
                            } else {
                              setSelectedSourceIds(selectedSourceIds.filter((id) => id !== s.id));
                            }
                          }}
                          className="rounded"
                        />
                        <span className="truncate">{s.name}</span>
                      </label>
                    ))
                  )}
                </div>
              </div>

              <div className="p-3 rounded-lg bg-blue-50 dark:bg-blue-950/40 text-blue-800 dark:text-blue-300 border border-blue-200 dark:border-blue-900">
                Synthesis runs as an asynchronous background worker job. You will receive an audit notification upon completion.
              </div>

              <div className="flex items-center justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowGenModal(false)}
                  className="px-3.5 py-1.5 rounded-lg border text-xs font-medium hover:bg-muted"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={submittingGen || selectedTemplates.length === 0}
                  className="px-4 py-1.5 rounded-lg bg-primary text-primary-foreground text-xs font-semibold shadow hover:opacity-90 disabled:opacity-50"
                >
                  {submittingGen ? 'Queueing Generation...' : 'Submit Generation Request'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* MODAL: PREVIEW EXTRACTED TEXT */}
      {previewSource && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm">
          <div className="bg-card w-full max-w-2xl border rounded-2xl shadow-2xl p-6 space-y-4 max-h-[85vh] flex flex-col">
            <div className="flex items-center justify-between border-b pb-3">
              <h2 className="text-base font-bold truncate">Extracted Text: {previewSource.name}</h2>
              <button
                onClick={() => setPreviewSource(null)}
                className="text-muted-foreground hover:text-foreground text-sm font-semibold"
              >
                ✕
              </button>
            </div>
            <div className="flex-1 overflow-y-auto bg-muted/40 p-4 rounded-xl border text-xs font-mono whitespace-pre-wrap">
              {previewSource.extracted_text || 'No extracted text available for this source.'}
            </div>
            <div className="flex justify-end pt-2">
              <button
                onClick={() => setPreviewSource(null)}
                className="px-4 py-1.5 rounded-lg bg-primary text-primary-foreground text-xs font-semibold"
              >
                Close Preview
              </button>
            </div>
          </div>
        </div>
      )}

      {/* MODAL: GENERATION STAGES INSPECTOR */}
      {selectedGenForStages && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm">
          <div className="bg-card w-full max-w-xl border rounded-2xl shadow-2xl p-6 space-y-4 max-h-[85vh] flex flex-col">
            <div className="flex items-center justify-between border-b pb-3">
              <div>
                <h2 className="text-base font-bold flex items-center gap-2">
                  <Activity className="h-5 w-5 text-primary" /> Pipeline Execution Stages
                </h2>
                <div className="text-xs text-muted-foreground font-mono">
                  Job: {selectedGenForStages.id.slice(0, 8)}... • Template: {selectedGenForStages.template_set}
                </div>
              </div>
              <button
                onClick={() => setSelectedGenForStages(null)}
                className="text-muted-foreground hover:text-foreground p-1 rounded hover:bg-muted"
              >
                <X className="h-4 w-4" />
              </button>
            </div>

            <div className="flex-1 overflow-y-auto space-y-3">
              {loadingStages ? (
                <div className="p-8 text-center text-muted-foreground flex items-center justify-center gap-2 text-xs">
                  <RefreshCw className="h-4 w-4 animate-spin" /> Loading stage execution log...
                </div>
              ) : genStages.length === 0 ? (
                <div className="p-8 text-center text-muted-foreground text-xs">
                  No individual stage execution metrics recorded for this run.
                </div>
              ) : (
                <div className="space-y-2">
                  {genStages.map((st, idx) => (
                    <div
                      key={st.id || st.stage_name}
                      className="p-3.5 rounded-xl border bg-muted/20 flex flex-col gap-2"
                    >
                      <div className="flex items-center justify-between">
                        <span className="font-semibold text-xs capitalize flex items-center gap-2">
                          <span className="font-mono text-[10px] text-muted-foreground">
                            #{idx + 1}
                          </span>
                          {st.stage_name.replace(/_/g, ' ')}
                        </span>
                        <span
                          className={`px-2 py-0.5 rounded-full text-[10px] font-bold uppercase ${
                            st.status === 'completed'
                              ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300'
                              : st.status === 'processing'
                              ? 'bg-blue-100 text-blue-800 animate-pulse'
                              : st.status === 'failed'
                              ? 'bg-red-100 text-red-800'
                              : 'bg-muted text-muted-foreground'
                          }`}
                        >
                          {st.status}
                        </span>
                      </div>

                      <div className="flex items-center justify-between text-[11px] text-muted-foreground">
                        <span className="flex items-center gap-1">
                          <Clock className="h-3 w-3" /> Duration: {st.duration_ms ? `${st.duration_ms} ms` : '—'}
                        </span>
                        {st.tokens_used ? <span>{st.tokens_used.toLocaleString()} tokens</span> : null}
                      </div>

                      {st.error_message && (
                        <div className="p-2 rounded bg-red-50 dark:bg-red-950/40 text-red-700 dark:text-red-400 text-[11px] font-mono">
                          {st.error_message}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>

            <div className="flex justify-end pt-2 border-t">
              <button
                onClick={() => setSelectedGenForStages(null)}
                className="px-4 py-1.5 rounded-lg bg-primary text-primary-foreground text-xs font-semibold"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}

      {/* MODAL: SYSTEM ARCHITECTURE MAP */}
      {showSystemMapModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm">
          <div className="bg-card w-full max-w-2xl border rounded-2xl shadow-2xl p-6 space-y-4 max-h-[85vh] flex flex-col">
            <div className="flex items-center justify-between border-b pb-3">
              <h2 className="text-base font-bold flex items-center gap-2">
                <Compass className="h-5 w-5 text-primary" /> Synthesized Architecture System Map (Stage 1)
              </h2>
              <button
                onClick={() => setShowSystemMapModal(null)}
                className="text-muted-foreground hover:text-foreground p-1 rounded hover:bg-muted"
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <div className="flex-1 overflow-y-auto bg-muted/40 p-4 rounded-xl border text-xs font-mono whitespace-pre-wrap leading-relaxed">
              {showSystemMapModal}
            </div>
            <div className="flex justify-end pt-2 border-t">
              <button
                onClick={() => setShowSystemMapModal(null)}
                className="px-4 py-1.5 rounded-lg bg-primary text-primary-foreground text-xs font-semibold"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}

      {/* MODAL: TOKEN USAGE AUDIT LEDGER */}
      {showTokenLedgerModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm">
          <div className="bg-card w-full max-w-3xl border rounded-2xl shadow-2xl p-6 space-y-4 max-h-[90vh] flex flex-col">
            <div className="flex items-center justify-between border-b pb-3">
              <div>
                <h2 className="text-base font-bold flex items-center gap-2">
                  <Zap className="h-5 w-5 text-amber-500" /> Token Usage Audit Ledger
                </h2>
                <p className="text-xs text-muted-foreground mt-0.5">
                  Detailed event-by-event token ledger for <strong>{project.name}</strong> for financial chargeback and quota reconciliation.
                </p>
              </div>
              <button
                onClick={() => setShowTokenLedgerModal(false)}
                className="text-muted-foreground hover:text-foreground p-1 rounded hover:bg-muted"
              >
                <X className="h-4 w-4" />
              </button>
            </div>

            <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 bg-muted/30 p-3 rounded-xl border text-xs">
              <div>
                <span className="text-muted-foreground text-[10px] uppercase font-semibold block">Total Tokens</span>
                <span className="text-base font-bold text-foreground">{(tokenSummary?.total_tokens || 0).toLocaleString()}</span>
              </div>
              <div>
                <span className="text-muted-foreground text-[10px] uppercase font-semibold block">Input / Prompt</span>
                <span className="text-sm font-semibold text-foreground">{(tokenSummary?.total_input_tokens || 0).toLocaleString()}</span>
              </div>
              <div>
                <span className="text-muted-foreground text-[10px] uppercase font-semibold block">Output / Completion</span>
                <span className="text-sm font-semibold text-foreground">{(tokenSummary?.total_output_tokens || 0).toLocaleString()}</span>
              </div>
              <div>
                <span className="text-muted-foreground text-[10px] uppercase font-semibold block">Total Events</span>
                <span className="text-sm font-semibold text-foreground">{(tokenSummary?.events_count || 0).toLocaleString()}</span>
              </div>
            </div>

            <div className="flex-1 overflow-y-auto border rounded-xl">
              <table className="w-full text-left text-xs">
                <thead className="bg-muted/60 border-b text-[10px] uppercase font-semibold text-muted-foreground sticky top-0 bg-card">
                  <tr>
                    <th className="py-2.5 px-3">Timestamp</th>
                    <th className="py-2.5 px-3">Operation</th>
                    <th className="py-2.5 px-3">Model</th>
                    <th className="py-2.5 px-3 text-right">Input</th>
                    <th className="py-2.5 px-3 text-right">Output</th>
                    <th className="py-2.5 px-3 text-right">Total Tokens</th>
                  </tr>
                </thead>
                <tbody className="divide-y font-mono">
                  {tokenSummary?.recent_usage && tokenSummary.recent_usage.length > 0 ? (
                    tokenSummary.recent_usage.map((u) => (
                      <tr key={u.id} className="hover:bg-muted/30 font-sans">
                        <td className="py-2.5 px-3 text-muted-foreground text-[11px] whitespace-nowrap">
                          {new Date(u.created_at).toLocaleString()}
                        </td>
                        <td className="py-2.5 px-3">
                          <span className="px-2 py-0.5 rounded text-[10px] font-medium bg-secondary text-secondary-foreground uppercase">
                            {u.operation.replace(/_/g, ' ')}
                          </span>
                        </td>
                        <td className="py-2.5 px-3 text-muted-foreground text-[11px]">
                          {u.model_used}
                        </td>
                        <td className="py-2.5 px-3 text-right font-mono text-[11px] text-muted-foreground">
                          {u.input_tokens.toLocaleString()}
                        </td>
                        <td className="py-2.5 px-3 text-right font-mono text-[11px] text-muted-foreground">
                          {u.output_tokens.toLocaleString()}
                        </td>
                        <td className="py-2.5 px-3 text-right font-mono text-[11px] font-semibold text-foreground">
                          {u.total_tokens.toLocaleString()}
                        </td>
                      </tr>
                    ))
                  ) : (
                    <tr>
                      <td colSpan={6} className="py-8 text-center text-muted-foreground font-sans text-xs">
                        No ledger entries found.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>

            <div className="flex justify-end pt-2 border-t">
              <button
                onClick={() => setShowTokenLedgerModal(false)}
                className="px-4 py-1.5 rounded-lg bg-primary text-primary-foreground text-xs font-semibold"
              >
                Close
              </button>
            </div>
          </div>
        </div>
      )}

      {/* MODAL: ASSOCIATE LLM WITH PROJECT */}
      {showLLMModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm">
          <div className="bg-card w-full max-w-md border rounded-2xl shadow-2xl p-6 space-y-4">
            <div className="flex items-center justify-between border-b pb-3">
              <h2 className="text-base font-bold flex items-center gap-2">
                <Cpu className="h-5 w-5 text-indigo-500" /> Associate LLM Configuration
              </h2>
              <button
                onClick={() => setShowLLMModal(false)}
                className="text-muted-foreground hover:text-foreground p-1 rounded hover:bg-muted"
              >
                <X className="h-4 w-4" />
              </button>
            </div>

            <form onSubmit={handleSaveProjectLLM} className="space-y-4 text-xs">
              <p className="text-muted-foreground leading-relaxed">
                Associate a dedicated AI model with <strong>{project?.name}</strong>. All documentation generation,
                system architecture mapping, and automated pre-review runs for this project will use this configuration by default.
              </p>

              <div className="space-y-1.5">
                <label className="font-semibold uppercase text-muted-foreground block text-[11px]">
                  Select Configured LLM Provider & Model:
                </label>
                <select
                  value={selectedLLMId}
                  onChange={(e) => setSelectedLLMId(e.target.value)}
                  className="w-full px-3 py-2.5 rounded-lg border bg-background text-xs focus:ring-1 focus:ring-primary"
                >
                  <option value="">-- Use System Default (Global Active LLM) --</option>
                  {availableLLMs.map((cfg) => (
                    <option key={cfg.id} value={cfg.id}>
                      {cfg.provider.toUpperCase()} — {cfg.default_model} {cfg.is_active ? '★ (Global Default)' : ''}
                    </option>
                  ))}
                </select>
              </div>

              {availableLLMs.length === 0 && (
                <div className="p-3 rounded-lg bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-900 text-amber-800 dark:text-amber-300">
                  No LLM configs found. As an administrator, you can configure providers in <strong>Admin &gt; LLM Configuration</strong>.
                </div>
              )}

              <div className="flex items-center justify-end gap-2 pt-2 border-t">
                <button
                  type="button"
                  onClick={() => setShowLLMModal(false)}
                  className="px-3.5 py-1.5 rounded-lg border text-xs font-medium hover:bg-muted"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={savingLLM}
                  className="px-4 py-1.5 rounded-lg bg-primary text-primary-foreground text-xs font-semibold shadow hover:opacity-90 disabled:opacity-50"
                >
                  {savingLLM ? 'Saving...' : 'Save LLM Association'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
