/** Metadata-only task coordination routes; message bodies are never in trees. */
export interface TaskTreeWait { kind: string; id?: string }
export interface TaskTreeNode {
  id: string; from: string; agent: string; status: string; age_seconds: number;
  thread_id: string; project?: string; parent_id?: string; relation: string;
  reference?: boolean; waiting_on: TaskTreeWait[]; children: TaskTreeNode[];
}
export interface TaskTreeResponse { root: TaskTreeNode; as_of: string; truncated: boolean }
export interface ReassignRequest { to: string; note?: string }
export interface ReassignResponse {
  id: string; from: string; to: string; previous_to: string; thread_id: string;
  project?: string; status: string; dry_run?: boolean;
}
