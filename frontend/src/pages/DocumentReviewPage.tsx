import React, { useEffect, useState, useMemo } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { ArchitecturalDiagram } from '@/components/ArchitecturalDiagram';
import type {
  AtlassianPage,
  AtlassianSpace,
  DocumentItem,
  DocumentVersion,
  FeedbackCategory,
  FeedbackStatus,
  Project,
} from '@/types';
import { api } from '@/lib/api';
import {
  AlertCircle,
  ArrowLeft,
  Check,
  CheckCircle2,
  Clock,
  Download,
  Edit3,
  Eye,
  FileCheck2,
  History,
  MessageSquarePlus,
  RefreshCw,
  Share2,
  ShieldAlert,
  ShieldCheck,
  Sparkles,
  Tag,
  X,
} from 'lucide-react';

const renderWithHighlights = (children: React.ReactNode): React.ReactNode => {
  if (typeof children === 'string') {
    if (!children.includes('[INFERRED]') && !children.includes('[MISSING]')) {
      return children;
    }
    const parts = children.split(/(\[INFERRED\]|\[MISSING\])/g);
    return parts.map((part, i) => {
      if (part === '[INFERRED]') {
        return (
          <span
            key={i}
            className="inline-flex items-center px-1.5 py-0.5 rounded text-[11px] font-mono font-bold bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300 border border-amber-300 mr-1"
          >
            [INFERRED]
          </span>
        );
      }
      if (part === '[MISSING]') {
        return (
          <span
            key={i}
            className="inline-flex items-center px-1.5 py-0.5 rounded text-[11px] font-mono font-bold bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300 border border-red-300 mr-1"
          >
            [MISSING]
          </span>
        );
      }
      return part;
    });
  }
  if (Array.isArray(children)) {
    return children.map((c, i) => <React.Fragment key={i}>{renderWithHighlights(c)}</React.Fragment>);
  }
  return children;
};

interface DocumentReviewPageProps {
  documentId: string;
  onNavigate: (view: string, id?: string) => void;
}

export const DocumentReviewPage: React.FC<DocumentReviewPageProps> = ({
  documentId,
  onNavigate,
}) => {
  const [doc, setDoc] = useState<DocumentItem | null>(null);
  const [project, setProject] = useState<Project | null>(null);
  const [allDocs, setAllDocs] = useState<DocumentItem[]>([]);
  const [loading, setLoading] = useState(true);

  // View state
  const [mode, setMode] = useState<'preview' | 'edit'>('preview');
  const [editedContent, setEditedContent] = useState('');
  const [editedTitle, setEditedTitle] = useState('');
  const [savingEdit, setSavingEdit] = useState(false);

  // Version viewing
  const [selectedVersion, setSelectedVersion] = useState<DocumentVersion | null>(null);
  const [showHistoryDropdown, setShowHistoryDropdown] = useState(false);

  // Feedback panel states
  const [showAddFeedbackModal, setShowAddFeedbackModal] = useState(false);
  const [fbCategory, setFbCategory] = useState<FeedbackCategory>('correction');
  const [fbSection, setFbSection] = useState('');
  const [fbContent, setFbContent] = useState('');
  const [fbIsBlocking, setFbIsBlocking] = useState(false);
  const [submittingFeedback, setSubmittingFeedback] = useState(false);

  // Actions states
  const [updatingWithFeedback, setUpdatingWithFeedback] = useState(false);
  const [showConfirmUpdateModal, setShowConfirmUpdateModal] = useState(false);
  const [showBlockingGateAlert, setShowBlockingGateAlert] = useState(false);
  const [approving, setApproving] = useState(false);
  const [exportingConfluence, setExportingConfluence] = useState(false);
  const [toastMessage, setToastMessage] = useState<string | null>(null);
  const [exportError, setExportError] = useState<string | null>(null);

  // Space Access Denied Fallback Modal
  const [spaceAccessDeniedData, setSpaceAccessDeniedData] = useState<{
    targetSpaceKey: string;
    accessibleSpaces: AtlassianSpace[];
    canSetProjectDefault: boolean;
  } | null>(null);
  const [overrideSpaceKey, setOverrideSpaceKey] = useState('');
  const [overrideParentPageId, setOverrideParentPageId] = useState('');
  const [overrideSetDefault, setOverrideSetDefault] = useState(false);
  const [overridePages, setOverridePages] = useState<AtlassianPage[]>([]);
  const [loadingOverridePages, setLoadingOverridePages] = useState(false);

  const loadDocumentData = async (id: string) => {
    setLoading(true);
    try {
      const documentData = await api.getDocument(id);
      const safeDoc = documentData
        ? {
            ...documentData,
            feedback: documentData.feedback || [],
            versions: documentData.versions || [],
          }
        : null;
      setDoc(safeDoc);
      setEditedContent(documentData?.content_md || '');
      setEditedTitle(documentData?.title || '');
      setSelectedVersion(null);

      // Load project & siblings
      if (documentData?.project_id) {
        const [projData, projectDocs] = await Promise.all([
          api.getProject(documentData.project_id),
          api.listDocuments(documentData.project_id),
        ]);
        setProject(projData);
        setAllDocs(projectDocs || []);
      }
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadDocumentData(documentId);
  }, [documentId]);

  // Toast auto-dismiss
  useEffect(() => {
    if (toastMessage) {
      const timer = setTimeout(() => setToastMessage(null), 5000);
      return () => clearTimeout(timer);
    }
  }, [toastMessage]);

  // Extract sections/headings from markdown for section anchor dropdown
  const headings = useMemo(() => {
    if (!doc?.content_md) return [];
    const lines = doc.content_md.split('\n');
    const list: string[] = [];
    for (const l of lines) {
      if (l.startsWith('#')) {
        const h = l.replace(/^#+\s*/, '').trim();
        if (h && !list.includes(h)) {
          list.push(h);
        }
      }
    }
    return list;
  }, [doc?.content_md]);

  // Filter feedback
  const feedbackList = doc?.feedback || [];
  const openFeedback = feedbackList.filter((f) => f.status === 'open');
  const openBlockingFeedback = openFeedback.filter((f) => f.is_blocking);

  // Current display content (either active doc or selected historical version)
  const displayContent = selectedVersion ? selectedVersion.content_md : (mode === 'edit' ? editedContent : doc?.content_md || '');

  // Handle Manual Save Edit
  const handleSaveEdit = async () => {
    if (!doc) return;
    setSavingEdit(true);
    try {
      const res = await api.updateDocument(doc.id, {
        title: editedTitle,
        content_md: editedContent,
        change_summary: 'Manual revision by reviewer',
      });
      setDoc(res.document);
      setToastMessage('Document content updated manually.');
      setMode('preview');
    } catch (err: any) {
      alert(err.message || 'Failed to update document');
    } finally {
      setSavingEdit(false);
    }
  };

  // Handle Add Feedback
  const handleAddFeedbackSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!doc || !fbContent.trim()) return;
    setSubmittingFeedback(true);
    try {
      const created = await api.addFeedback(doc.id, {
        section_anchor: fbSection || undefined,
        category: fbCategory,
        content: fbContent,
        is_blocking: fbIsBlocking,
      });

      setDoc((prev) => {
        if (!prev) return prev;
        return {
          ...prev,
          feedback: [created, ...(prev.feedback || [])],
        };
      });

      setShowAddFeedbackModal(false);
      setFbContent('');
      setFbSection('');
      setFbIsBlocking(false);
      setToastMessage('Feedback recorded successfully.');
    } catch (err: any) {
      alert(err.message || 'Failed to submit feedback');
    } finally {
      setSubmittingFeedback(false);
    }
  };

  // Handle Update Feedback Status
  const handleFeedbackStatusToggle = async (fbId: string, newStatus: FeedbackStatus) => {
    try {
      await api.updateFeedbackStatus(fbId, newStatus);
      setDoc((prev) => {
        if (!prev) return prev;
        return {
          ...prev,
          feedback: (prev.feedback || []).map((f) =>
            f.id === fbId ? { ...f, status: newStatus } : f
          ),
        };
      });
    } catch (err: any) {
      alert(err.message || 'Failed to update feedback status');
    }
  };

  // Handle "Update with Feedback" (AI Revision Loop)
  const handleTriggerUpdateWithFeedback = async () => {
    if (!doc) return;
    setShowConfirmUpdateModal(false);
    setUpdatingWithFeedback(true);
    try {
      const res = await api.updateWithFeedback(doc.id);
      setDoc(res.document);
      setEditedContent(res.document.content_md);
      setSelectedVersion(null);
      setToastMessage(
        `Version ${res.new_version.version_number} created – ${openFeedback.length} feedback items addressed.`
      );
    } catch (err: any) {
      alert(err.message || 'Failed to revise document with feedback');
    } finally {
      setUpdatingWithFeedback(false);
    }
  };

  // Handle Approval with Hard Gate check
  const handleApprove = async () => {
    if (!doc) return;

    // Hard Gate Check: If any blocking feedback remains open, block approval!
    if (openBlockingFeedback.length > 0) {
      setShowBlockingGateAlert(true);
      return;
    }

    setApproving(true);
    try {
      const approvedDoc = await api.approveDocument(doc.id);
      setDoc(approvedDoc);
      setToastMessage('Document successfully approved!');
    } catch (err: any) {
      alert(err.message || 'Failed to approve document');
    } finally {
      setApproving(false);
    }
  };

  const loadOverridePages = async (spaceKey: string) => {
    if (!doc?.project_id || !spaceKey) return;
    setLoadingOverridePages(true);
    try {
      const pg = await api.getProjectConfluencePages(doc.project_id, spaceKey);
      setOverridePages(pg || []);
    } catch {
      setOverridePages([]);
    } finally {
      setLoadingOverridePages(false);
    }
  };

  // Handle Confluence Export
  const handleExportConfluence = async (overrideOptions?: {
    spaceKey: string;
    spaceName?: string;
    parentPageId?: string;
    setDefault?: boolean;
  }) => {
    if (!doc) return;
    setExportingConfluence(true);
    setExportError(null);
    try {
      const res = await api.exportToConfluence(
        doc.id,
        overrideOptions
          ? {
              override_space_key: overrideOptions.spaceKey,
              override_space_name: overrideOptions.spaceName,
              override_parent_page_id: overrideOptions.parentPageId || undefined,
              set_as_project_default: overrideOptions.setDefault,
            }
          : undefined
      );
      setDoc((prev) => (prev ? { ...prev, confluence_page_id: res.confluence_page_id } : prev));
      setToastMessage(`Exported to Confluence! Page ID: ${res.confluence_page_id}`);
      setSpaceAccessDeniedData(null);
    } catch (err: any) {
      if (err.code === 'SPACE_ACCESS_DENIED') {
        const spaces: AtlassianSpace[] = err.accessible_spaces || [];
        setSpaceAccessDeniedData({
          targetSpaceKey: err.target_space_key || '',
          accessibleSpaces: spaces,
          canSetProjectDefault: Boolean(err.can_set_project_default),
        });
        if (spaces.length > 0) {
          const firstKey = spaces[0].key;
          setOverrideSpaceKey(firstKey);
          setOverrideParentPageId('');
          loadOverridePages(firstKey);
        }
      } else {
        setExportError(err.message || 'Confluence export failed. You can download the markdown file directly.');
      }
    } finally {
      setExportingConfluence(false);
    }
  };

  const handleSubmitOverrideExport = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!overrideSpaceKey) return;
    const spaceObj = spaceAccessDeniedData?.accessibleSpaces.find((s) => s.key === overrideSpaceKey);
    await handleExportConfluence({
      spaceKey: overrideSpaceKey,
      spaceName: spaceObj ? spaceObj.name : overrideSpaceKey,
      parentPageId: overrideParentPageId || undefined,
      setDefault: overrideSetDefault,
    });
  };

  const getCategoryBadgeClass = (category: FeedbackCategory) => {
    switch (category) {
      case 'correction':
        return 'bg-red-100 text-red-800 dark:bg-red-950 dark:text-red-300 border-red-300';
      case 'missing_info':
        return 'bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300 border-amber-300';
      case 'additional_fact':
        return 'bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-300 border-blue-300';
      case 'clarification':
        return 'bg-purple-100 text-purple-800 dark:bg-purple-950 dark:text-purple-300 border-purple-300';
      case 'style':
        return 'bg-indigo-100 text-indigo-800 dark:bg-indigo-950 dark:text-indigo-300 border-indigo-300';
      default:
        return 'bg-slate-100 text-slate-800 dark:bg-slate-900 dark:text-slate-300 border-slate-300';
    }
  };

  if (loading) {
    return (
      <div className="p-20 text-center text-muted-foreground flex items-center justify-center gap-2">
        <RefreshCw className="h-5 w-5 animate-spin" /> Loading document review workspace...
      </div>
    );
  }

  if (!doc) {
    return (
      <div className="p-16 text-center">
        <h2 className="text-lg font-bold">Document not found</h2>
        <button onClick={() => onNavigate('projects')} className="mt-4 text-primary underline">
          Back to Projects
        </button>
      </div>
    );
  }

  const currentVersionNumber = doc.versions?.length || 1;

  return (
    <div className="flex flex-col h-[calc(100vh-4rem)] bg-background overflow-hidden">
      {/* Toast Notification */}
      {toastMessage && (
        <div className="fixed top-20 right-6 z-50 flex items-center gap-2 bg-foreground text-background text-xs font-semibold px-4 py-3 rounded-xl shadow-2xl animate-in fade-in slide-in-from-top-4">
          <CheckCircle2 className="h-4 w-4 text-emerald-400" />
          <span>{toastMessage}</span>
          <button onClick={() => setToastMessage(null)} className="ml-2 hover:opacity-75">
            <X className="h-3 w-3" />
          </button>
        </div>
      )}

      {/* Main 3-Column Layout */}
      <div className="flex flex-1 overflow-hidden">
        {/* ================= COLUMN 1: LEFT SIDEBAR (DOCUMENTS LIST) ================= */}
        <aside className="w-64 border-r bg-muted/20 flex flex-col hidden lg:flex">
          <div className="p-3.5 border-b flex items-center justify-between">
            <button
              onClick={() => onNavigate('project-detail', doc.project_id)}
              className="text-xs font-semibold text-muted-foreground hover:text-foreground flex items-center gap-1.5"
            >
              <ArrowLeft className="h-3.5 w-3.5" /> Back to Project
            </button>
            <span className="text-[11px] font-mono font-medium text-muted-foreground">
              {allDocs.length} Docs
            </span>
          </div>

          <div className="p-3 border-b bg-card">
            <div className="text-[11px] uppercase font-bold text-muted-foreground tracking-wider">
              Project Context
            </div>
            <div className="text-xs font-bold text-foreground truncate mt-0.5">{project?.name}</div>
          </div>

          <div className="flex-1 overflow-y-auto p-2 space-y-1">
            {allDocs.map((d) => (
              <button
                key={d.id}
                onClick={() => {
                  if (d.id !== doc.id) {
                    onNavigate('document-review', d.id);
                  }
                }}
                className={`w-full text-left p-2.5 rounded-lg text-xs transition flex flex-col gap-1 ${
                  d.id === doc.id
                    ? 'bg-primary text-primary-foreground font-semibold shadow-sm'
                    : 'hover:bg-muted text-muted-foreground hover:text-foreground'
                }`}
              >
                <div className="flex items-center justify-between">
                  <span className="font-mono text-[10px] uppercase opacity-75">{d.template_type}</span>
                  <span
                    className={`text-[9px] uppercase px-1.5 py-0.2 rounded font-bold ${
                      d.status === 'approved'
                        ? 'bg-emerald-500/20 text-emerald-300'
                        : 'bg-amber-500/20 text-amber-300'
                    }`}
                  >
                    {d.status}
                  </span>
                </div>
                <span className="truncate leading-tight">{d.title}</span>
              </button>
            ))}
          </div>
        </aside>

        {/* ================= COLUMN 2: MAIN CONTENT AREA ================= */}
        <main className="flex-1 flex flex-col overflow-hidden bg-background">
          {/* Document Header Bar */}
          <div className="p-4 border-b bg-card/50 flex flex-wrap items-center justify-between gap-3">
            <div className="space-y-1 min-w-0">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="font-mono text-xs uppercase px-2 py-0.5 rounded-md bg-secondary text-secondary-foreground font-semibold">
                  {doc.template_type}
                </span>
                <span
                  className={`text-xs uppercase font-bold px-2 py-0.5 rounded-md ${
                    doc.status === 'approved'
                      ? 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300 border border-emerald-300'
                      : 'bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300 border border-amber-300'
                  }`}
                >
                  Status: {doc.status.replace('_', ' ')}
                </span>
                <span className="text-xs font-semibold px-2 py-0.5 rounded-md bg-muted text-foreground border font-mono">
                  Version: {selectedVersion ? `v${selectedVersion.version_number} (Historical)` : `v${currentVersionNumber}`}
                </span>
                {doc.confluence_page_id && (
                  <span className="text-xs font-semibold px-2 py-0.5 rounded-md bg-blue-100 text-blue-800 dark:bg-blue-950 dark:text-blue-300 border border-blue-300 flex items-center gap-1">
                    <Share2 className="h-3 w-3" /> Confluence Page: {doc.confluence_page_id}
                  </span>
                )}
              </div>
              <h1 className="text-xl font-black text-foreground truncate">{doc.title}</h1>
            </div>

            {/* Controls: Mode Toggle & Version History */}
            <div className="flex items-center gap-2">
              <div className="flex items-center rounded-lg border bg-muted p-0.5 text-xs font-medium">
                <button
                  onClick={() => setMode('preview')}
                  className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md transition ${
                    mode === 'preview' ? 'bg-card text-foreground shadow-sm font-semibold' : 'text-muted-foreground hover:text-foreground'
                  }`}
                >
                  <Eye className="h-3.5 w-3.5" /> Preview
                </button>
                <button
                  onClick={() => setMode('edit')}
                  className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md transition ${
                    mode === 'edit' ? 'bg-card text-foreground shadow-sm font-semibold' : 'text-muted-foreground hover:text-foreground'
                  }`}
                >
                  <Edit3 className="h-3.5 w-3.5" /> Edit Raw
                </button>
              </div>

              {/* Version History Dropdown */}
              <div className="relative">
                <button
                  onClick={() => setShowHistoryDropdown(!showHistoryDropdown)}
                  className="flex items-center gap-1 px-3 py-1.5 rounded-lg border bg-card text-xs font-medium hover:bg-muted"
                >
                  <History className="h-3.5 w-3.5" />
                  <span>History ({doc.versions?.length || 1}) ▾</span>
                </button>

                {showHistoryDropdown && (
                  <div className="absolute right-0 mt-2 w-72 rounded-xl border bg-card shadow-xl z-50 p-2 space-y-1 text-xs">
                    <div className="font-bold px-2 py-1 border-b text-muted-foreground">Version History</div>
                    <button
                      onClick={() => {
                        setSelectedVersion(null);
                        setShowHistoryDropdown(false);
                      }}
                      className={`w-full text-left p-2 rounded-lg hover:bg-muted transition flex flex-col ${
                        selectedVersion === null ? 'bg-secondary font-semibold' : ''
                      }`}
                    >
                      <div className="flex justify-between items-center font-bold">
                        <span>Current Active Version (v{currentVersionNumber})</span>
                        <span className="text-[10px] text-emerald-600 font-bold">LATEST</span>
                      </div>
                      <span className="text-[11px] text-muted-foreground">Active review draft</span>
                    </button>

                    {doc.versions?.map((v) => (
                      <button
                        key={v.id}
                        onClick={() => {
                          setSelectedVersion(v);
                          setShowHistoryDropdown(false);
                        }}
                        className={`w-full text-left p-2 rounded-lg hover:bg-muted transition flex flex-col ${
                          selectedVersion?.id === v.id ? 'bg-secondary font-semibold' : ''
                        }`}
                      >
                        <div className="flex justify-between items-center font-bold">
                          <span>Version {v.version_number}</span>
                          <span className="text-[10px] text-muted-foreground">
                            {new Date(v.created_at).toLocaleTimeString()}
                          </span>
                        </div>
                        <span className="text-[11px] text-muted-foreground truncate">
                          {v.change_summary || 'Generated draft'}
                        </span>
                      </button>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </div>

          {/* Historical Version Notice Banner */}
          {selectedVersion && (
            <div className="bg-amber-50 dark:bg-amber-950/50 border-b border-amber-200 px-4 py-2 text-xs text-amber-800 dark:text-amber-200 flex items-center justify-between">
              <span className="flex items-center gap-1.5 font-medium">
                <Clock className="h-3.5 w-3.5" /> Viewing historical Version {selectedVersion.version_number} created on{' '}
                {new Date(selectedVersion.created_at).toLocaleString()} ({selectedVersion.change_summary})
              </span>
              <button
                onClick={() => setSelectedVersion(null)}
                className="font-bold underline hover:opacity-80"
              >
                Return to Latest Version
              </button>
            </div>
          )}

          {/* Content Area: Editor or Markdown Preview */}
          <div className="flex-1 overflow-y-auto p-6 md:p-8">
            {mode === 'edit' && !selectedVersion ? (
              <div className="space-y-4 max-w-4xl mx-auto">
                <div className="space-y-1">
                  <label className="text-xs font-semibold text-muted-foreground uppercase">Document Title</label>
                  <input
                    type="text"
                    value={editedTitle}
                    onChange={(e) => setEditedTitle(e.target.value)}
                    className="w-full px-3.5 py-2 rounded-lg border bg-background font-bold text-sm focus:ring-1 focus:ring-primary"
                  />
                </div>
                <div className="space-y-1">
                  <label className="text-xs font-semibold text-muted-foreground uppercase">Markdown Content</label>
                  <textarea
                    rows={28}
                    value={editedContent}
                    onChange={(e) => setEditedContent(e.target.value)}
                    className="w-full p-4 rounded-xl border bg-background font-mono text-xs leading-relaxed focus:ring-1 focus:ring-primary resize-y"
                  />
                </div>
                <div className="flex justify-end gap-2">
                  <button
                    onClick={() => {
                      setEditedContent(doc.content_md);
                      setMode('preview');
                    }}
                    className="px-4 py-2 rounded-lg border text-xs font-medium hover:bg-muted"
                  >
                    Cancel Edit
                  </button>
                  <button
                    onClick={handleSaveEdit}
                    disabled={savingEdit}
                    className="px-5 py-2 rounded-lg bg-primary text-primary-foreground text-xs font-semibold shadow hover:opacity-90 disabled:opacity-50"
                  >
                    {savingEdit ? 'Saving...' : 'Save Manual Changes'}
                  </button>
                </div>
              </div>
            ) : (
              <div className="max-w-4xl mx-auto prose prose-slate dark:prose-invert prose-headings:font-bold prose-h1:text-2xl prose-h2:text-xl prose-h3:text-lg prose-table:border prose-th:bg-muted/50 prose-th:p-2 prose-td:p-2 prose-code:font-mono prose-code:bg-muted prose-code:px-1 prose-code:py-0.5 prose-code:rounded">
                <ReactMarkdown
                  remarkPlugins={[remarkGfm]}
                  components={{
                    p: ({ children }) => <p className="mb-4">{renderWithHighlights(children)}</p>,
                    li: ({ children }) => <li>{renderWithHighlights(children)}</li>,
                    td: ({ children }) => <td className="p-2">{renderWithHighlights(children)}</td>,
                    th: ({ children }) => <th className="p-2">{renderWithHighlights(children)}</th>,
                    pre: ({ children, ...props }) => {
                      if (
                        React.isValidElement(children) &&
                        typeof (children.props as any)?.className === 'string' &&
                        (children.props as any).className.includes('language-mermaid')
                      ) {
                        return <>{children}</>;
                      }
                      return <pre {...props}>{children}</pre>;
                    },
                    code: ({ className, children, ...props }) => {
                      const isMermaid = /language-mermaid/.test(className || '');
                      if (isMermaid) {
                        return <ArchitecturalDiagram code={String(children).replace(/\n$/, '')} />;
                      }
                      return (
                        <code className={className} {...props}>
                          {children}
                        </code>
                      );
                    },
                  }}
                >
                  {displayContent}
                </ReactMarkdown>
              </div>
            )}
          </div>

          {/* ================= STICKY BOTTOM ACTION BAR ================= */}
          <footer className="border-t bg-card px-4 py-3 flex flex-wrap items-center justify-between gap-3 shadow-lg z-30">
            <div className="flex items-center gap-2">
              <button
                onClick={() => onNavigate('project-detail', doc.project_id)}
                className="px-3.5 py-2 rounded-lg border text-xs font-medium hover:bg-muted flex items-center gap-1.5"
              >
                <ArrowLeft className="h-3.5 w-3.5" /> Back to Project
              </button>

              <a
                href={api.getDownloadURL(doc.id)}
                target="_blank"
                rel="noreferrer"
                className="px-3.5 py-2 rounded-lg border text-xs font-medium hover:bg-muted flex items-center gap-1.5 text-foreground"
              >
                <Download className="h-3.5 w-3.5" /> Download Markdown
              </a>
            </div>

            {/* Inline Export Error Banner */}
            {exportError && (
              <div className="w-full flex flex-col gap-2 p-3 rounded-lg bg-red-50 dark:bg-red-950/40 border border-red-200 dark:border-red-800 text-red-700 dark:text-red-400 text-xs">
                <div className="flex items-start gap-2">
                  <AlertCircle className="h-4 w-4 mt-0.5 shrink-0" />
                  <span className="flex-1 leading-relaxed">{exportError}</span>
                  <button onClick={() => setExportError(null)} className="shrink-0 hover:opacity-70 p-0.5">
                    <X className="h-3.5 w-3.5" />
                  </button>
                </div>
                {exportError.includes('reconnect') && (
                  <div className="pl-6 pt-1">
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
                      className="px-3 py-1.5 rounded-lg bg-red-600 text-white font-semibold hover:bg-red-700 flex items-center gap-1.5 text-xs shadow-sm"
                    >
                      <RefreshCw className="h-3.5 w-3.5" /> Reconnect Atlassian Session
                    </button>
                  </div>
                )}
              </div>
            )}

            <div className="flex items-center gap-2.5">
              {/* Primary Action: Update with Feedback */}
              <button
                onClick={() => setShowConfirmUpdateModal(true)}
                disabled={updatingWithFeedback || openFeedback.length === 0}
                className="px-4 py-2 rounded-lg bg-indigo-600 text-white font-semibold text-xs shadow hover:bg-indigo-700 disabled:opacity-40 disabled:cursor-not-allowed flex items-center gap-2 transition"
                title={openFeedback.length === 0 ? 'No open feedback items to address' : ''}
              >
                {updatingWithFeedback ? (
                  <>
                    <RefreshCw className="h-3.5 w-3.5 animate-spin" />
                    <span>AI Revising Document...</span>
                  </>
                ) : (
                  <>
                    <Sparkles className="h-3.5 w-3.5" />
                    <span>Update with Feedback ({openFeedback.length})</span>
                  </>
                )}
              </button>

              {/* Approve Button (with blocking gate guard) */}
              <button
                onClick={handleApprove}
                disabled={approving || doc.status === 'approved'}
                className={`px-4 py-2 rounded-lg font-semibold text-xs shadow flex items-center gap-1.5 transition ${
                  doc.status === 'approved'
                    ? 'bg-emerald-600 text-white cursor-default'
                    : openBlockingFeedback.length > 0
                    ? 'bg-amber-500 text-white hover:bg-amber-600'
                    : 'bg-emerald-600 text-white hover:bg-emerald-700'
                }`}
              >
                <Check className="h-3.5 w-3.5" />
                {doc.status === 'approved' ? 'Approved' : 'Approve Document'}
              </button>

              {/* Export to Confluence */}
              <button
                onClick={() => handleExportConfluence()}
                disabled={exportingConfluence || doc.status !== 'approved'}
                className="px-3.5 py-2 rounded-lg border text-xs font-semibold hover:bg-muted flex items-center gap-1.5 disabled:opacity-40"
                title={doc.status !== 'approved' ? 'Approve document first before publishing' : ''}
              >
                <Share2 className="h-3.5 w-3.5" />
                {exportingConfluence ? 'Publishing...' : 'Export to Confluence'}
              </button>
            </div>
          </footer>
        </main>

        {/* ================= COLUMN 3: RIGHT FEEDBACK PANEL ================= */}
        <aside className="w-80 md:w-96 border-l bg-card flex flex-col z-20">
          {/* Panel Header */}
          <div className="p-4 border-b flex items-center justify-between">
            <div>
              <h2 className="font-extrabold text-sm flex items-center gap-2">
                Structured Feedback ({feedbackList.length})
              </h2>
              <span className="text-[11px] text-muted-foreground font-medium">
                {openFeedback.length} Open ({openBlockingFeedback.length} Blocking)
              </span>
            </div>

            <button
              onClick={() => setShowAddFeedbackModal(true)}
              className="inline-flex items-center gap-1 px-3 py-1.5 rounded-lg bg-primary text-primary-foreground font-semibold text-xs shadow hover:opacity-90"
            >
              <MessageSquarePlus className="h-3.5 w-3.5" /> Add Feedback
            </button>
          </div>

          {/* Feedback Cards List */}
          <div className="flex-1 overflow-y-auto p-3 space-y-3">
            {feedbackList.length === 0 ? (
              <div className="p-8 text-center text-muted-foreground space-y-2">
                <FileCheck2 className="h-8 w-8 text-muted-foreground/60 mx-auto" />
                <div className="text-xs font-semibold">No feedback recorded yet</div>
                <p className="text-[11px]">
                  Add corrections, missing information, or clarifications to refine the document.
                </p>
              </div>
            ) : (
              feedbackList.map((fb) => (
                <div
                  key={fb.id}
                  className={`p-3.5 rounded-xl border text-xs space-y-2.5 transition shadow-sm ${
                    fb.status === 'addressed'
                      ? 'bg-muted/30 opacity-75 border-dashed'
                      : fb.is_blocking
                      ? 'bg-red-50/40 dark:bg-red-950/20 border-red-200 dark:border-red-900'
                      : 'bg-card'
                  }`}
                >
                  <div className="flex items-center justify-between gap-2">
                    <span
                      className={`text-[10px] uppercase font-bold px-2 py-0.5 rounded-full border ${getCategoryBadgeClass(
                        fb.category
                      )}`}
                    >
                      {fb.category.replace('_', ' ')}
                    </span>

                    {fb.is_blocking && (
                      <span className="flex items-center gap-1 text-[10px] font-black uppercase text-red-600 dark:text-red-400 bg-red-100 dark:bg-red-950 px-1.5 py-0.5 rounded border border-red-300">
                        <ShieldAlert className="h-3 w-3" /> Blocking
                      </span>
                    )}

                    <span
                      className={`text-[10px] font-semibold uppercase px-1.5 py-0.5 rounded ${
                        fb.status === 'open'
                          ? 'bg-blue-100 text-blue-800'
                          : 'bg-emerald-100 text-emerald-800'
                      }`}
                    >
                      {fb.status}
                    </span>
                  </div>

                  {fb.section_anchor && (
                    <div className="text-[11px] font-mono font-medium text-muted-foreground flex items-center gap-1 truncate">
                      <Tag className="h-3 w-3" /> {fb.section_anchor}
                    </div>
                  )}

                  <p className="text-foreground leading-relaxed whitespace-pre-wrap">{fb.content}</p>

                  <div className="pt-2 border-t flex items-center justify-between text-[10px] text-muted-foreground">
                    <span className="flex items-center gap-1">
                      {fb.author_name === 'System: Pre-Review Gate' ? (
                        <span className="inline-flex items-center gap-1 font-semibold text-purple-600 dark:text-purple-400 bg-purple-100 dark:bg-purple-950/60 px-1.5 py-0.5 rounded text-[10px]">
                          <ShieldCheck className="h-3 w-3" /> Pre-Review Gate
                        </span>
                      ) : (
                        fb.author_name || 'Reviewer'
                      )}{' '}
                      • {new Date(fb.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                    </span>

                    <div className="flex items-center gap-1.5">
                      {fb.status === 'open' ? (
                        <>
                          <button
                            onClick={() => handleFeedbackStatusToggle(fb.id, 'addressed')}
                            className="font-medium text-emerald-600 hover:underline"
                          >
                            Mark Addressed
                          </button>
                          <span>•</span>
                          <button
                            onClick={() => handleFeedbackStatusToggle(fb.id, 'dismissed')}
                            className="font-medium text-muted-foreground hover:underline"
                          >
                            Dismiss
                          </button>
                        </>
                      ) : (
                        <button
                          onClick={() => handleFeedbackStatusToggle(fb.id, 'open')}
                          className="font-medium text-primary hover:underline"
                        >
                          Reopen
                        </button>
                      )}
                    </div>
                  </div>
                </div>
              ))
            )}
          </div>

          {/* Panel Footer Summary */}
          <div className="p-3 border-t bg-muted/40 flex items-center justify-between text-xs font-semibold text-muted-foreground">
            <span>Open Items: {openFeedback.length}</span>
            <span className={openBlockingFeedback.length > 0 ? 'text-red-600 dark:text-red-400 font-bold' : ''}>
              Blocking: {openBlockingFeedback.length}
            </span>
          </div>
        </aside>
      </div>

      {/* ================= MODAL: ADD FEEDBACK ================= */}
      {showAddFeedbackModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm">
          <div className="bg-card w-full max-w-lg border rounded-2xl shadow-2xl p-6 space-y-4">
            <h2 className="text-lg font-bold flex items-center gap-2">
              <MessageSquarePlus className="h-5 w-5 text-primary" /> Add Review Feedback
            </h2>
            <form onSubmit={handleAddFeedbackSubmit} className="space-y-4 text-xs">
              <div className="space-y-1">
                <label className="font-semibold uppercase text-muted-foreground">Category</label>
                <select
                  value={fbCategory}
                  onChange={(e) => setFbCategory(e.target.value as FeedbackCategory)}
                  className="w-full px-3 py-2 rounded-lg border bg-background text-xs focus:ring-1 focus:ring-primary font-medium"
                >
                  <option value="correction">Correction (Factual or technical error)</option>
                  <option value="missing_info">Missing Information (Omitted required detail)</option>
                  <option value="additional_fact">Additional Fact (New technical context)</option>
                  <option value="clarification">Clarification (Ambiguity or imprecise term)</option>
                  <option value="style">Style (Formatting, grammar, or tone)</option>
                  <option value="other">Other</option>
                </select>
              </div>

              <div className="space-y-1">
                <label className="font-semibold uppercase text-muted-foreground">
                  Link to Section (Optional)
                </label>
                <select
                  value={fbSection}
                  onChange={(e) => setFbSection(e.target.value)}
                  className="w-full px-3 py-2 rounded-lg border bg-background text-xs focus:ring-1 focus:ring-primary"
                >
                  <option value="">-- General Document Feedback --</option>
                  {headings.map((h, i) => (
                    <option key={i} value={h}>
                      {h}
                    </option>
                  ))}
                </select>
              </div>

              <div className="space-y-1">
                <label className="font-semibold uppercase text-muted-foreground">Feedback Comment</label>
                <textarea
                  rows={4}
                  required
                  value={fbContent}
                  onChange={(e) => setFbContent(e.target.value)}
                  placeholder="Describe the technical correction, missing endpoint, or clarification required..."
                  className="w-full p-3 rounded-lg border bg-background text-xs focus:ring-1 focus:ring-primary leading-relaxed"
                />
              </div>

              {/* Blocking Flag Checkbox */}
              <label className="flex items-center gap-2.5 p-3 rounded-xl border bg-red-50/30 dark:bg-red-950/20 border-red-200 cursor-pointer">
                <input
                  type="checkbox"
                  checked={fbIsBlocking}
                  onChange={(e) => setFbIsBlocking(e.target.checked)}
                  className="rounded h-4 w-4 text-red-600 focus:ring-red-500"
                />
                <div>
                  <span className="font-bold text-red-700 dark:text-red-300">
                    Blocking – must be resolved before approval
                  </span>
                  <p className="text-[11px] text-muted-foreground">
                    Enforces strict governance gate. Document cannot transition to Approved while open.
                  </p>
                </div>
              </label>

              <div className="flex items-center justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => setShowAddFeedbackModal(false)}
                  className="px-3.5 py-1.5 rounded-lg border font-medium hover:bg-muted"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={submittingFeedback || !fbContent.trim()}
                  className="px-4 py-1.5 rounded-lg bg-primary text-primary-foreground font-semibold shadow hover:opacity-90 disabled:opacity-50"
                >
                  {submittingFeedback ? 'Saving...' : 'Save Feedback'}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* ================= MODAL: CONFIRM "UPDATE WITH FEEDBACK" ================= */}
      {showConfirmUpdateModal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm">
          <div className="bg-card w-full max-w-md border rounded-2xl shadow-2xl p-6 space-y-4">
            <h2 className="text-base font-bold flex items-center gap-2">
              <Sparkles className="h-5 w-5 text-indigo-500" /> Confirm AI Document Revision
            </h2>
            <p className="text-xs text-muted-foreground leading-relaxed">
              This will submit the current document draft alongside all{' '}
              <strong>{openFeedback.length} open feedback item(s)</strong> and project sources to the LLM. 
              A new immutable version (v{currentVersionNumber + 1}) will be synthesized and feedback items will automatically be marked addressed.
            </p>
            <div className="flex justify-end gap-2 pt-2">
              <button
                onClick={() => setShowConfirmUpdateModal(false)}
                className="px-3.5 py-1.5 rounded-lg border text-xs font-medium hover:bg-muted"
              >
                Cancel
              </button>
              <button
                onClick={handleTriggerUpdateWithFeedback}
                className="px-4 py-1.5 rounded-lg bg-indigo-600 text-white text-xs font-semibold shadow hover:bg-indigo-700"
              >
                Continue & Revise
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ================= MODAL: HARD GATE BLOCKING WARNING ================= */}
      {showBlockingGateAlert && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm">
          <div className="bg-card w-full max-w-md border border-red-300 rounded-2xl shadow-2xl p-6 space-y-4">
            <div className="flex items-center gap-3 text-red-600">
              <ShieldAlert className="h-7 w-7 flex-shrink-0" />
              <div>
                <h3 className="font-extrabold text-base text-foreground">Document Sign-off Blocked</h3>
                <span className="text-xs text-red-600 font-semibold">Strict Hard Gate Enforcement</span>
              </div>
            </div>

            <p className="text-xs text-muted-foreground leading-relaxed">
              This document cannot be approved because{' '}
              <strong className="text-foreground">{openBlockingFeedback.length} blocking feedback item(s)</strong> are currently open:
            </p>

            <div className="max-h-36 overflow-y-auto space-y-1.5 border rounded-lg p-2 bg-muted/40 text-xs">
              {openBlockingFeedback.map((b) => (
                <div key={b.id} className="p-1.5 rounded bg-card border border-red-200">
                  <span className="font-bold text-red-600 mr-1">[{b.category}]</span>
                  <span className="text-foreground">{b.content}</span>
                </div>
              ))}
            </div>

            <p className="text-[11px] text-muted-foreground">
              Please use <strong>"Update with Feedback"</strong> to have the AI revise the document, or resolve/dismiss the blocking items in the feedback panel.
            </p>

            <div className="flex justify-end pt-2">
              <button
                onClick={() => setShowBlockingGateAlert(false)}
                className="px-4 py-1.5 rounded-lg bg-primary text-primary-foreground text-xs font-semibold shadow"
              >
                Understood
              </button>
            </div>
          </div>
        </div>
      )}

      {/* MODAL: SPACE ACCESS DENIED / OVERRIDE SPACE PICKER */}
      {spaceAccessDeniedData && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm">
          <div className="bg-card w-full max-w-lg border rounded-2xl shadow-2xl p-6 space-y-4">
            <div className="flex items-start gap-3">
              <div className="p-2 rounded-full bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300 shrink-0">
                <AlertCircle className="h-5 w-5" />
              </div>
              <div>
                <h2 className="text-base font-bold">Space Access Restricted</h2>
                <p className="text-xs text-muted-foreground mt-1">
                  You do not have write access to the project's configured Confluence space{' '}
                  <span className="font-semibold text-foreground">'{spaceAccessDeniedData.targetSpaceKey}'</span>.
                  Atlassian requires publishing using your own authenticated credentials.
                </p>
              </div>
            </div>

            <form onSubmit={handleSubmitOverrideExport} className="space-y-4 text-xs">
              <div className="space-y-1">
                <label className="font-semibold uppercase text-muted-foreground block">
                  Select Accessible Confluence Space:
                </label>
                {spaceAccessDeniedData.accessibleSpaces.length === 0 ? (
                  <div className="p-3 border rounded-lg text-muted-foreground">
                    No accessible Confluence spaces found for your account. Please check your Confluence space permissions.
                  </div>
                ) : (
                  <select
                    value={overrideSpaceKey}
                    onChange={(e) => {
                      setOverrideSpaceKey(e.target.value);
                      setOverrideParentPageId('');
                      loadOverridePages(e.target.value);
                    }}
                    required
                    className="w-full px-3 py-2 rounded-lg border bg-background text-xs focus:outline-none focus:ring-1 focus:ring-primary"
                  >
                    {spaceAccessDeniedData.accessibleSpaces.map((s) => (
                      <option key={s.key} value={s.key}>
                        {s.name} ({s.key})
                      </option>
                    ))}
                  </select>
                )}
              </div>

              {overrideSpaceKey && (
                <div className="space-y-1">
                  <label className="font-semibold uppercase text-muted-foreground block">
                    Parent Page in Space:
                  </label>
                  {loadingOverridePages ? (
                    <div className="p-2 text-muted-foreground flex items-center gap-1.5">
                      <RefreshCw className="h-3 w-3 animate-spin" /> Loading pages...
                    </div>
                  ) : (
                    <select
                      value={overrideParentPageId}
                      onChange={(e) => setOverrideParentPageId(e.target.value)}
                      className="w-full px-3 py-2 rounded-lg border bg-background text-xs focus:outline-none focus:ring-1 focus:ring-primary"
                    >
                      <option value="">Root of space (No parent page)</option>
                      {overridePages.map((p) => (
                        <option key={p.id} value={p.id}>
                          {p.title}
                        </option>
                      ))}
                    </select>
                  )}
                </div>
              )}

              {spaceAccessDeniedData.canSetProjectDefault && (
                <label className="flex items-center gap-2 p-3 rounded-lg border bg-secondary/30 cursor-pointer">
                  <input
                    type="checkbox"
                    checked={overrideSetDefault}
                    onChange={(e) => setOverrideSetDefault(e.target.checked)}
                    className="rounded"
                  />
                  <div>
                    <span className="font-semibold text-foreground">Set as project default target</span>
                    <p className="text-[11px] text-muted-foreground">
                      As a project owner or administrator, update this project to use this space for future exports.
                    </p>
                  </div>
                </label>
              )}

              <div className="flex items-center justify-end gap-2 pt-2 border-t">
                <button
                  type="button"
                  onClick={() => setSpaceAccessDeniedData(null)}
                  className="px-3.5 py-1.5 rounded-lg border text-xs font-medium hover:bg-muted"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={exportingConfluence || !overrideSpaceKey}
                  className="px-4 py-1.5 rounded-lg bg-primary text-primary-foreground text-xs font-semibold shadow hover:opacity-90 disabled:opacity-50 flex items-center gap-1.5"
                >
                  {exportingConfluence ? (
                    <>
                      <RefreshCw className="h-3.5 w-3.5 animate-spin" /> Exporting...
                    </>
                  ) : (
                    'Export to Selected Space'
                  )}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
};
