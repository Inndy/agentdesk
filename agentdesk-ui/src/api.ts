export interface Option {
  id: string;
  label: string;
  description?: string;
  pros?: string;
  cons?: string;
}

export interface Issue {
  project_id: string;
  issue_id: string;
  revision: number;
  kind: "decision" | "action" | "notice";
  status: "pending" | "answered" | "acked" | "withdrawn";
  title: string;
  context: string;
  options?: Option[];
  recommended_option?: string;
  recommendation_reason?: string;
  blocking_scope?: string;
  created_by: string;
  owner_coordinator: string;
  origin_session?: string;
  task_ref?: string;
  branch?: string;
  commit?: string;
  related_documents?: string[];
  priority: string;
  deferred: boolean;
  created_at: string;
  updated_at: string;
  reply?: {
    revision: number;
    option_id?: string;
    text?: string;
    ack: boolean;
    actor: string;
    at: string;
  };
}

export interface Session {
  project_id: string;
  name: string;
  actor: string;
}

export interface History {
  event_id: string;
  kind: string;
  revision: number;
  actor: string;
  at: string;
  receipts: { consumer: string; holder: string; at: string }[];
}

export class ApiError extends Error {
  status: number;
  code: string;
  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

export async function api<T>(path: string, body?: unknown, method?: string): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    method: method ?? (body === undefined ? "GET" : "POST"),
    credentials: "same-origin",
    headers: body === undefined ? {} : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const envelope = await response.json();
  if (!response.ok) {
    throw new ApiError(response.status, envelope.code, envelope.error ?? "Service error");
  }
  if (envelope.schema_version !== 1)
    throw new ApiError(503, "schema_version", "Unsupported service version");
  return envelope.data as T;
}
