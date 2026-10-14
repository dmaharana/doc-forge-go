import type {
  AtlassianPage,
  AtlassianSpace,
  ConfluenceTarget,
  DocumentFeedback,
  DocumentItem,
  DocumentVersion,
  FeedbackCategory,
  FeedbackStatus,
  Generation,
  LLMConfig,
  NotificationPreference,
  Project,
  ProjectMember,
  Source,
  User,
} from '@/types';

const API_BASE = import.meta.env.VITE_API_URL || 'http://localhost:8080/api/v1';

function getToken(): string | null {
  return localStorage.getItem('docforge_token');
}

export function setToken(token: string) {
  localStorage.setItem('docforge_token', token);
}

export function removeToken() {
  localStorage.removeItem('docforge_token');
}

async function request<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
  const token = getToken();
  const headers = new Headers(options.headers || {});

  if (token && !headers.has('Authorization')) {
    headers.set('Authorization', `Bearer ${token}`);
  }

  if (!headers.has('Content-Type') && !(options.body instanceof FormData)) {
    headers.set('Content-Type', 'application/json');
  }

  const response = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    headers,
  });

  if (response.status === 401) {
    // If unauthorized, clear token and notify
    if (!endpoint.includes('/auth/login') && !endpoint.includes('/auth/atlassian')) {
      removeToken();
      window.dispatchEvent(new Event('auth:unauthorized'));
    }
  }

  if (!response.ok) {
    let errMsg = `Request failed (${response.status})`;
    let errData: any = {};
    try {
      errData = await response.json();
      errMsg = errData.error || errMsg;
    } catch {
      // not json
    }
    const err: any = new Error(errMsg);
    Object.assign(err, errData, { status: response.status });
    throw err;
  }

  return response.json();
}

export const api = {
  // Auth
  async login(email: string, password: string):Promise<{ token: string; user: User }> {
    const res = await request<{ token: string; user: User }>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    });
    setToken(res.token);
    return res;
  },

  async getMe(): Promise<User> {
    return request<User>('/auth/me');
  },

  // Admin Users
  async adminListUsers(): Promise<User[]> {
    return request<User[]>('/admin/users');
  },

  async adminCreateUser(data: { name: string; email: string; role: string; password: string }): Promise<User> {
    return request<User>('/admin/users', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  },

  async adminUpdateUser(id: string, data: { name: string; role: string; is_active: boolean }): Promise<User> {
    return request<User>(`/admin/users/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  },

  // Admin LLM Config
  async adminListLLM(): Promise<LLMConfig[]> {
    return request<LLMConfig[]>('/admin/llm');
  },

  async adminSaveLLM(data: Partial<LLMConfig> & { api_key?: string }): Promise<LLMConfig> {
    return request<LLMConfig>('/admin/llm', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  },

  // Projects
  async listProjects(): Promise<Project[]> {
    return request<Project[]>('/projects');
  },

  async createProject(data: { name: string; description: string }): Promise<Project> {
    return request<Project>('/projects', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  },

  async getProject(id: string): Promise<Project> {
    return request<Project>(`/projects/${id}`);
  },

  async updateProject(id: string, data: { name: string; description: string; status: string }): Promise<Project> {
    return request<Project>(`/projects/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  },

  async deleteProject(id: string): Promise<{ status: string }> {
    return request<{ status: string }>(`/projects/${id}`, {
      method: 'DELETE',
    });
  },

  // Sources
  async listSources(projectId: string): Promise<Source[]> {
    return request<Source[]>(`/projects/${projectId}/sources`);
  },

  async uploadSourceFile(projectId: string, file: File): Promise<Source> {
    const formData = new FormData();
    formData.append('file', file);
    return request<Source>(`/projects/${projectId}/sources/upload`, {
      method: 'POST',
      body: formData,
    });
  },

  async addRemoteSource(projectId: string, data: { name: string; remote_url: string; remote_type: string }): Promise<Source> {
    return request<Source>(`/projects/${projectId}/sources/remote`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  },

  async deleteSource(sourceId: string): Promise<{ status: string }> {
    return request<{ status: string }>(`/sources/${sourceId}`, {
      method: 'DELETE',
    });
  },

  // Generations
  async listGenerations(projectId: string): Promise<Generation[]> {
    return request<Generation[]>(`/projects/${projectId}/generations`);
  },

  async submitGeneration(projectId: string, data: { template_set: string; source_ids: string[]; llm_config_id?: string }): Promise<Generation> {
    return request<Generation>(`/projects/${projectId}/generations`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  },

  async getGeneration(id: string): Promise<Generation> {
    return request<Generation>(`/generations/${id}`);
  },

  // Documents
  async listDocuments(projectId: string): Promise<DocumentItem[]> {
    return request<DocumentItem[]>(`/projects/${projectId}/documents`);
  },

  async getDocument(id: string): Promise<DocumentItem> {
    return request<DocumentItem>(`/documents/${id}`);
  },

  async updateDocument(id: string, data: { title: string; content_md: string; change_summary: string }): Promise<{ document: DocumentItem; new_version: DocumentVersion }> {
    return request<{ document: DocumentItem; new_version: DocumentVersion }>(`/documents/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    });
  },

  async approveDocument(id: string): Promise<DocumentItem> {
    return request<DocumentItem>(`/documents/${id}/approve`, {
      method: 'POST',
    });
  },

  getDownloadURL(id: string): string {
    const token = getToken();
    return `${API_BASE}/documents/${id}/download?token=${token || ''}`;
  },

  async exportToConfluence(
    id: string,
    options?: {
      override_space_key?: string;
      override_space_name?: string;
      override_parent_page_id?: string;
      set_as_project_default?: boolean;
    }
  ): Promise<{ confluence_page_id: string; status: string }> {
    return request<{ confluence_page_id: string; status: string }>(`/documents/${id}/export-confluence`, {
      method: 'POST',
      body: options ? JSON.stringify(options) : undefined,
    });
  },

  // Feedback
  async addFeedback(docId: string, data: { section_anchor?: string; category: FeedbackCategory; content: string; is_blocking: boolean }): Promise<DocumentFeedback> {
    return request<DocumentFeedback>(`/documents/${docId}/feedback`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  },

  async updateFeedbackStatus(feedbackId: string, status: FeedbackStatus): Promise<{ status: string }> {
    return request<{ status: string }>(`/feedback/${feedbackId}`, {
      method: 'PUT',
      body: JSON.stringify({ status }),
    });
  },

  async updateWithFeedback(docId: string): Promise<{ message: string; document: DocumentItem; new_version: DocumentVersion }> {
    return request<{ message: string; document: DocumentItem; new_version: DocumentVersion }>(`/documents/${docId}/update-with-feedback`, {
      method: 'POST',
    });
  },

  // Atlassian SSO & Connect
  async getAtlassianSSOUrl(): Promise<{ authorization_url: string; is_mock_mode: string }> {
    return request<{ authorization_url: string; is_mock_mode: string }>('/auth/atlassian/start');
  },
  async atlassianSSOStart(): Promise<{ authorization_url: string; is_mock_mode: string }> {
    return this.getAtlassianSSOUrl();
  },

  async atlassianSSOCallback(code: string): Promise<{ token: string; user: User }> {
    const res = await request<{ token: string; user: User }>(`/auth/atlassian/callback?code=${encodeURIComponent(code)}`);
    setToken(res.token);
    return res;
  },

  async atlassianConnect(): Promise<{ authorization_url: string; is_mock_mode: string }> {
    return request<{ authorization_url: string; is_mock_mode: string }>('/atlassian/connect');
  },

  async atlassianCallback(code: string): Promise<{ status: string; site_url: string }> {
    return request<{ status: string; site_url: string }>(`/atlassian/callback?code=${encodeURIComponent(code)}`);
  },

  async atlassianListSpaces(): Promise<AtlassianSpace[]> {
    return request<AtlassianSpace[]>('/atlassian/spaces');
  },

  // Project Members
  async getProjectMembers(projectId: string): Promise<ProjectMember[]> {
    return request<ProjectMember[]>(`/projects/${projectId}/members`);
  },

  async addProjectMember(projectId: string, email: string, role: string = 'member'): Promise<ProjectMember> {
    return request<ProjectMember>(`/projects/${projectId}/members`, {
      method: 'POST',
      body: JSON.stringify({ email, role }),
    });
  },

  async deleteProjectMember(projectId: string, memberId: string): Promise<{ status: string }> {
    return request<{ status: string }>(`/projects/${projectId}/members/${memberId}`, {
      method: 'DELETE',
    });
  },

  // Project-Scoped Confluence Target & Hierarchy
  async getProjectConfluenceTarget(projectId: string): Promise<ConfluenceTarget | null> {
    return request<ConfluenceTarget | null>(`/projects/${projectId}/confluence-target`);
  },

  async setProjectConfluenceTarget(
    projectId: string,
    data: { space_key: string; space_name: string; parent_page_id?: string; parent_page_title?: string }
  ): Promise<ConfluenceTarget> {
    return request<ConfluenceTarget>(`/projects/${projectId}/confluence-target`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
  },

  async deleteProjectConfluenceTarget(projectId: string): Promise<{ status: string }> {
    return request<{ status: string }>(`/projects/${projectId}/confluence-target`, {
      method: 'DELETE',
    });
  },

  async getProjectConfluenceSpaces(projectId: string): Promise<AtlassianSpace[]> {
    return request<AtlassianSpace[]>(`/projects/${projectId}/confluence/spaces`);
  },

  async getProjectConfluencePages(projectId: string, spaceKey?: string): Promise<AtlassianPage[]> {
    const q = spaceKey ? `?space_key=${encodeURIComponent(spaceKey)}` : '';
    return request<AtlassianPage[]>(`/projects/${projectId}/confluence/pages${q}`);
  },

  // Notifications
  async getNotificationPreferences(projectId?: string): Promise<NotificationPreference[]> {
    const q = projectId ? `?project_id=${projectId}` : '';
    return request<NotificationPreference[]>(`/notifications/preferences${q}`);
  },

  async saveNotificationPreference(data: { project_id?: string; channel: 'email' | 'teams'; enabled: boolean; teams_webhook_url?: string }): Promise<NotificationPreference> {
    return request<NotificationPreference>('/notifications/preferences', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  },
};
