export type Status =
  | "ready"
  | "planning"
  | "implementing"
  | "reviewing"
  | "done"
  | "cancelled";

export type SubtaskStatus =
  | "pending"
  | "running"
  | "complete"
  | "failed"
  | "closed"
  | "done"
  | "blocked"
  | "in_review";

export interface TaskEntry {
  filename: string;
  status: Status;
  description?: string;
  branch?: string;
  topic?: string;
  created_at?: string;
  implemented?: string;
  planning_at?: string;
  implementing_at?: string;
  reviewing_at?: string;
  done_at?: string;
  goal?: string;
  content?: string;
  clickup_task_id?: string;
  review_cycle?: number;
  pr_url?: string;
  pr_review_decision?: string;
  pr_check_status?: string;
}

export interface SubtaskEntry {
  task_number: number;
  title: string;
  status: SubtaskStatus;
}

export interface TopicEntry {
  name: string;
  created_at: string;
}

export interface AuditEvent {
  id: number;
  kind: string;
  timestamp: string;
  project: string;
  task_file: string;
  instance_title: string;
  agent_type: string;
  wave_number: number;
  task_number: number;
  message: string;
  detail: string;
  level: string;
}
