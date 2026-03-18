import type { TaskEntry, SubtaskEntry, TopicEntry, AuditEvent } from "./types";

async function requestJSON<T>(path: string): Promise<T> {
  const response = await fetch(path);
  if (!response.ok) {
    let message = `HTTP error ${response.status}`;
    try {
      const body = (await response.json()) as { error?: string };
      if (body.error) {
        message = body.error;
      }
    } catch {
      // ignore parse errors
    }
    throw new Error(message);
  }
  return response.json() as Promise<T>;
}

async function requestText(path: string): Promise<string> {
  const response = await fetch(path);
  if (!response.ok) {
    throw new Error(`HTTP error ${response.status}`);
  }
  return response.text();
}

type AuditEventResponse = {
  ID: number;
  Kind: string;
  Timestamp: string;
  Project: string;
  TaskFile: string;
  InstanceTitle: string;
  AgentType: string;
  WaveNumber: number;
  TaskNumber: number;
  Message: string;
  Detail: string;
  Level: string;
};

function normalizeAuditEvent(raw: AuditEventResponse): AuditEvent {
  return {
    id: raw.ID,
    kind: raw.Kind,
    timestamp: raw.Timestamp,
    project: raw.Project,
    task_file: raw.TaskFile,
    instance_title: raw.InstanceTitle,
    agent_type: raw.AgentType,
    wave_number: raw.WaveNumber,
    task_number: raw.TaskNumber,
    message: raw.Message,
    detail: raw.Detail,
    level: raw.Level,
  };
}

export function resolveProjectName(
  search: string,
  hostname: string = window.location.hostname,
): string {
  const params = new URLSearchParams(search);
  const project = params.get("project");
  if (project) return project;

  const local = ["localhost", "127.0.0.1", ""];
  if (!local.includes(hostname)) return hostname;

  return "kasmos";
}

export async function listTasks(project: string): Promise<TaskEntry[]> {
  return requestJSON<TaskEntry[]>(`/v1/projects/${project}/tasks`);
}

export async function getTask(
  project: string,
  filename: string,
): Promise<TaskEntry> {
  return requestJSON<TaskEntry>(
    `/v1/projects/${project}/tasks/${encodeURIComponent(filename)}`,
  );
}

export async function getTaskContent(
  project: string,
  filename: string,
): Promise<string> {
  return requestText(
    `/v1/projects/${project}/tasks/${encodeURIComponent(filename)}/content`,
  );
}

export async function getSubtasks(
  project: string,
  filename: string,
): Promise<SubtaskEntry[]> {
  return requestJSON<SubtaskEntry[]>(
    `/v1/projects/${project}/tasks/${encodeURIComponent(filename)}/subtasks`,
  );
}

export async function listTopics(project: string): Promise<TopicEntry[]> {
  return requestJSON<TopicEntry[]>(`/v1/projects/${project}/topics`);
}

export async function listAuditEvents(project: string): Promise<AuditEvent[]> {
  const raw = await requestJSON<AuditEventResponse[]>(
    `/v1/projects/${project}/audit-events`,
  );
  return raw.map(normalizeAuditEvent);
}

export type AuditEventFilter = {
  kind?: string;
  task?: string;
  limit?: number;
};

export async function fetchAuditEvents(
  project: string,
  filter?: AuditEventFilter,
): Promise<AuditEvent[]> {
  const params = new URLSearchParams();
  if (filter?.kind) params.append("kind", filter.kind);
  if (filter?.task) params.set("task", filter.task);
  if (filter?.limit != null) params.set("limit", String(filter.limit));
  const qs = params.toString();
  const url = `/v1/projects/${project}/audit-events${qs ? `?${qs}` : ""}`;
  const raw = await requestJSON<AuditEventResponse[]>(url);
  return raw.map(normalizeAuditEvent);
}
