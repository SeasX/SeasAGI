export interface User {
  user_id: string;
  email: string;
  created_at: string;
  updated_at: string;
}

export interface UsageRecord {
  record_id: string;
  user_id: string;
  channel_id: string;
  model: string;
  request_count: number;
  input_tokens: number;
  output_tokens: number;
  is_error: number;
  recorded_at: string;
}

export interface UsageStats {
  total_requests: number;
  total_input_tokens: number;
  total_output_tokens: number;
  total_tokens: number;
  total_errors: number;
  distinct_users: number;
}