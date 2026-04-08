const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${API_URL}${path}`, {
    ...options,
    headers: { 'Content-Type': 'application/json', ...options?.headers },
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`API ${res.status}: ${text}`);
  }
  const text = await res.text();
  if (!text) return undefined as T;
  return JSON.parse(text);
}

// --- Types ---

export interface Agent {
  id: string;
  name: string;
  role: string;
  system_prompt: string;
  model: string;
  tools: string[];
  channels: string[];
  guardrails: { max_tokens_per_run?: number; max_runs_per_hour?: number; blocked_actions?: string[] };
  created_at: string;
  updated_at: string;
  memory?: Record<string, string>;
}

export interface Workflow {
  id: string;
  name: string;
  description: string;
  template_id: string;
  status: string;
  created_at: string;
  nodes?: WorkflowNode[];
  edges?: WorkflowEdge[];
}

export interface WorkflowNode {
  id: string;
  workflow_id: string;
  agent_id: string;
  label: string;
  position_x: number;
  position_y: number;
  is_entry: boolean;
}

export interface WorkflowEdge {
  id: string;
  workflow_id: string;
  source_node_id: string;
  target_node_id: string;
  condition: string;
  priority: number;
}

export interface AgentCost {
  agent_id: string;
  tokens_in: number;
  tokens_out: number;
  cost_usd: number;
  source: string;
}

export interface CostSummary {
  total_tokens_in: number;
  total_tokens_out: number;
  total_cost_usd: number;
  agent_breakdown?: AgentCost[];
}

export interface Execution {
  id: string;
  workflow_id: string | null;
  execution_type: string;
  status: string;
  triggered_by: string;
  iteration_count: number;
  started_at: string;
  completed_at: string | null;
  cost_summary?: CostSummary;
}

export interface Schedule {
  id: string;
  agent_id: string;
  cron_expr: string;
  task_prompt: string;
  enabled: boolean;
  last_run: string | null;
  next_run: string | null;
}

export interface Message {
  id: string;
  execution_id: string;
  from_agent_id: string;
  to_agent_id: string | null;
  content: string;
  channel: string;
  status: string;
  created_at: string;
}

export interface Template {
  id: string;
  name: string;
  description: string;
}

// --- Agents ---

export const listAgents = () => request<Agent[]>('/api/agents');
export const getAgent = (id: string) => request<Agent>(`/api/agents/${id}`);
export const createAgent = (data: Partial<Agent>) =>
  request<Agent>('/api/agents', { method: 'POST', body: JSON.stringify(data) });
export const updateAgent = (id: string, data: Partial<Agent>) =>
  request<Agent>(`/api/agents/${id}`, { method: 'PUT', body: JSON.stringify(data) });
export const deleteAgent = (id: string) =>
  request<void>(`/api/agents/${id}`, { method: 'DELETE' });

export const getMemory = (id: string) => request<Record<string, string>>(`/api/agents/${id}/memory`);
export const setMemory = (id: string, key: string, value: string) =>
  request<void>(`/api/agents/${id}/memory`, { method: 'PUT', body: JSON.stringify({ key, value }) });
export const deleteMemoryKey = (id: string, key: string) =>
  request<void>(`/api/agents/${id}/memory/${key}`, { method: 'DELETE' });

// --- Schedules ---

export const getSchedules = (agentId: string) =>
  request<Schedule[]>(`/api/agents/${agentId}/schedules`);
export const createSchedule = (agentId: string, cronExpr: string, taskPrompt: string) =>
  request<Schedule>(`/api/agents/${agentId}/schedules`, { method: 'POST', body: JSON.stringify({ cron_expr: cronExpr, task_prompt: taskPrompt }) });
export const deleteSchedule = (agentId: string, scheduleId: string) =>
  request<void>(`/api/agents/${agentId}/schedules/${scheduleId}`, { method: 'DELETE' });
export const toggleSchedule = (agentId: string, scheduleId: string, enabled: boolean) =>
  request<void>(`/api/agents/${agentId}/schedules/${scheduleId}`, { method: 'PUT', body: JSON.stringify({ enabled }) });

// --- Workflows ---

export const listWorkflows = () => request<Workflow[]>('/api/workflows');
export const getWorkflow = (id: string) => request<Workflow>(`/api/workflows/${id}`);
export const createWorkflow = (data: Partial<Workflow>) =>
  request<Workflow>('/api/workflows', { method: 'POST', body: JSON.stringify(data) });
export const updateWorkflow = (id: string, data: Partial<Workflow>) =>
  request<Workflow>(`/api/workflows/${id}`, { method: 'PUT', body: JSON.stringify(data) });
export const deleteWorkflow = (id: string) =>
  request<void>(`/api/workflows/${id}`, { method: 'DELETE' });
export const executeWorkflow = (id: string) =>
  request<{ execution_id: string }>(`/api/workflows/${id}/execute`, { method: 'POST', body: JSON.stringify({ trigger: 'manual' }) });

export const createNode = (workflowId: string, data: Partial<WorkflowNode>) =>
  request<WorkflowNode>(`/api/workflows/${workflowId}/nodes`, { method: 'POST', body: JSON.stringify(data) });
export const deleteNode = (workflowId: string, nodeId: string) =>
  request<void>(`/api/workflows/${workflowId}/nodes/${nodeId}`, { method: 'DELETE' });
export const createEdge = (workflowId: string, data: Partial<WorkflowEdge>) =>
  request<WorkflowEdge>(`/api/workflows/${workflowId}/edges`, { method: 'POST', body: JSON.stringify(data) });
export const updateEdge = (workflowId: string, edgeId: string, data: Partial<WorkflowEdge>) =>
  request<WorkflowEdge>(`/api/workflows/${workflowId}/edges/${edgeId}`, { method: 'PUT', body: JSON.stringify(data) });
export const deleteEdge = (workflowId: string, edgeId: string) =>
  request<void>(`/api/workflows/${workflowId}/edges/${edgeId}`, { method: 'DELETE' });

// --- Executions ---

export const getExecution = (id: string) => request<Execution>(`/api/executions/${id}`);
export const listExecutions = () => request<Execution[]>('/api/executions');
export const getMessages = (id: string) => request<Message[]>(`/api/executions/${id}/messages`);

// --- Templates ---

export const listTemplates = () => request<Template[]>('/api/templates');
export const loadTemplate = (id: string) =>
  request<{ workflow_id: string; template_id: string; status: string }>(`/api/templates/${id}/load`, { method: 'POST' });
