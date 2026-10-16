export type UserRole = 'admin' | 'user';

export interface User {
  id: string;
  email: string;
  name: string;
  role: UserRole;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface LLMConfig {
  id: string;
  provider: string;
  api_key_masked?: string;
  base_url?: string;
  default_model: string;
  max_tokens: number;
  temperature: number;
  is_active: boolean;
  updated_at: string;
}

export interface Project {
  id: string;
  name: string;
  description: string;
  owner_id: string;
  owner_name?: string;
  status: string;
  llm_config_id?: string;
  llm_model?: string;
  llm_provider?: string;
  user_role?: 'owner' | 'member' | 'admin';
  total_tokens?: number;
  created_at: string;
  updated_at: string;
}

export type ProjectMemberRole = 'owner' | 'member';
export type ProjectMemberStatus = 'active' | 'pending';

export interface ProjectMember {
  id: string;
  project_id: string;
  user_id?: string;
  user_name?: string;
  user_email: string;
  role: ProjectMemberRole;
  status: ProjectMemberStatus;
  created_at: string;
}

export type SourceType = 'upload' | 'remote';

export interface Source {
  id: string;
  project_id: string;
  name: string;
  type: SourceType;
  file_path?: string;
  original_filename?: string;
  mime_type?: string;
  size_bytes?: number;
  remote_url?: string;
  remote_type?: string;
  extracted_text?: string;
  metadata_json?: string;
  status: string;
  created_at: string;
}

export type GenerationStatus = 'queued' | 'processing' | 'completed' | 'failed';

export interface Generation {
  id: string;
  project_id: string;
  requested_by: string;
  requester_name?: string;
  status: GenerationStatus;
  template_set: string;
  llm_config_id?: string;
  model_used?: string;
  error_message?: string;
  notification_sent_at?: string;
  system_map_md?: string;
  completeness_scorecard_json?: string;
  created_at: string;
  started_at?: string;
  completed_at?: string;
  source_ids?: string[];
  total_tokens?: number;
  stages?: GenerationStage[];
}

export interface CompletenessScorecard {
  coverage_score: number;
  has_entrypoint: boolean;
  has_config: boolean;
  has_api_endpoints: boolean;
  has_data_models: boolean;
  has_tests: boolean;
  total_chunks: number;
  total_symbols: number;
  missing_areas: string[];
}

export type GenerationStageStatus = 'pending' | 'processing' | 'completed' | 'failed' | 'skipped';

export interface GenerationStage {
  id: string;
  generation_id: string;
  stage_name: string;
  status: GenerationStageStatus;
  input_context?: string;
  output_payload?: string;
  tokens_used: number;
  duration_ms: number;
  error_message?: string;
  created_at: string;
  completed_at?: string;
}

export type DocumentStatus = 'draft' | 'in_review' | 'approved';

export interface DocumentVersion {
  id: string;
  document_id: string;
  version_number: number;
  content_md: string;
  change_summary: string;
  created_by: string;
  author_name?: string;
  created_at: string;
}

export type FeedbackCategory =
  | 'correction'
  | 'missing_info'
  | 'additional_fact'
  | 'clarification'
  | 'style'
  | 'other';

export type FeedbackStatus = 'open' | 'addressed' | 'dismissed';

export interface DocumentFeedback {
  id: string;
  document_id: string;
  user_id: string;
  author_name?: string;
  author_email?: string;
  section_anchor?: string;
  category: FeedbackCategory;
  content: string;
  is_blocking: boolean;
  status: FeedbackStatus;
  created_at: string;
  updated_at: string;
}

export interface DocumentItem {
  id: string;
  generation_id?: string;
  project_id: string;
  title: string;
  template_type: string;
  content_md: string;
  status: DocumentStatus;
  current_version_id?: string;
  confluence_page_id?: string;
  created_at: string;
  updated_at: string;
  versions?: DocumentVersion[];
  feedback?: DocumentFeedback[];
}

export interface ConfluenceTarget {
  id: string;
  project_id: string;
  connection_id?: string;
  space_key: string;
  space_name: string;
  parent_page_id?: string;
  parent_page_title?: string;
  is_default: boolean;
  updated_by?: string;
  updater_name?: string;
  updated_at?: string;
}

export interface AtlassianSpace {
  key: string;
  name: string;
  type: string;
}

export interface AtlassianPage {
  id: string;
  title: string;
}

export interface NotificationPreference {
  id: string;
  user_id: string;
  project_id?: string;
  channel: 'email' | 'teams';
  enabled: boolean;
  teams_webhook_masked?: string;
  created_at: string;
}

export interface TokenUsage {
  id: string;
  project_id: string;
  generation_id?: string;
  document_id?: string;
  stage_id?: string;
  operation: string;
  model_used: string;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  created_at: string;
}

export interface ProjectTokenSummary {
  project_id: string;
  total_tokens: number;
  total_input_tokens: number;
  total_output_tokens: number;
  events_count: number;
  by_model: Record<string, number>;
  by_operation: Record<string, number>;
  recent_usage: TokenUsage[];
}

