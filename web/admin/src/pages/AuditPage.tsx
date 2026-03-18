import { useState, useEffect } from "react";
import { Link, useLocation } from "react-router";
import { fetchAuditEvents, resolveProjectName } from "../api";
import type { AuditEvent } from "../types";
import styles from "./AuditPage.module.css";

const EVENT_KINDS = [
  "agent_spawned",
  "agent_finished",
  "agent_killed",
  "agent_paused",
  "agent_resumed",
  "agent_restarted",
  "plan_transition",
  "plan_created",
  "plan_merged",
  "plan_cancelled",
  "wave_started",
  "wave_completed",
  "wave_failed",
  "prompt_sent",
  "git_push",
  "pr_created",
  "permission_detected",
  "permission_answered",
  "fsm_error",
  "error",
  "session_started",
  "session_stopped",
] as const;

const LIMIT_OPTIONS = [25, 50, 100, 200, 500] as const;

function formatDateTime(value: string): string {
  if (!value) return "—";
  const d = new Date(value);
  if (isNaN(d.getTime())) return value;
  return d.toLocaleString();
}

function prettyDetail(detail: string): string {
  if (!detail) return "";
  try {
    return JSON.stringify(JSON.parse(detail), null, 2);
  } catch {
    return detail;
  }
}

type Tone = "lifecycle" | "plan" | "wave" | "ops" | "error";

const LIFECYCLE_KINDS = new Set([
  "agent_spawned",
  "agent_finished",
  "agent_killed",
  "agent_paused",
  "agent_resumed",
  "agent_restarted",
  "session_started",
  "session_stopped",
]);

const PLAN_KINDS = new Set([
  "plan_transition",
  "plan_created",
  "plan_merged",
  "plan_cancelled",
]);

const WAVE_KINDS = new Set(["wave_started", "wave_completed", "wave_failed"]);
const ERROR_KINDS = new Set(["fsm_error", "error"]);

function kindTone(kind: string): Tone {
  if (LIFECYCLE_KINDS.has(kind)) return "lifecycle";
  if (PLAN_KINDS.has(kind)) return "plan";
  if (WAVE_KINDS.has(kind)) return "wave";
  if (ERROR_KINDS.has(kind)) return "error";
  return "ops";
}

const TONE_CLASSES: Record<Tone, string> = {
  lifecycle: styles.toneLifecycle,
  plan: styles.tonePlan,
  wave: styles.toneWave,
  ops: styles.toneOps,
  error: styles.toneError,
};

function levelClass(level: string): string {
  const l = level || "info";
  if (l === "error") return styles.levelError;
  if (l === "warn") return styles.levelWarn;
  return styles.levelInfo;
}

export default function AuditPage() {
  const { search } = useLocation();
  const project = resolveProjectName(search);

  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [kind, setKind] = useState("");
  const [taskInput, setTaskInput] = useState("");
  const [taskFile, setTaskFile] = useState("");
  const [limit, setLimit] = useState(100);
  const [expandedRowId, setExpandedRowId] = useState<number | null>(null);

  // Debounce taskInput -> taskFile (300ms)
  useEffect(() => {
    const timer = setTimeout(() => setTaskFile(taskInput), 300);
    return () => clearTimeout(timer);
  }, [taskInput]);

  // Fetch events whenever project, kind, taskFile, or limit changes
  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    fetchAuditEvents(project, {
      kind: kind || undefined,
      task: taskFile || undefined,
      limit,
    })
      .then((data) => {
        if (cancelled) return;
        setEvents(data);
        setLoading(false);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : "unknown error");
        setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [project, kind, taskFile, limit]);

  function toggleRow(event: AuditEvent) {
    if (!event.detail) return;
    setExpandedRowId((prev) => (prev === event.id ? null : event.id));
  }

  const rows = events.flatMap((event) => {
    const isExpanded = expandedRowId === event.id;
    const hasDetail = Boolean(event.detail);
    const rowClasses = [
      styles.row,
      hasDetail ? styles.expandable : "",
      isExpanded ? styles.expanded : "",
    ]
      .filter(Boolean)
      .join(" ");

    const mainRow = (
      <tr
        key={event.id}
        className={rowClasses}
        onClick={() => toggleRow(event)}
        tabIndex={hasDetail ? 0 : undefined}
        role={hasDetail ? "button" : undefined}
        aria-expanded={hasDetail ? isExpanded : undefined}
        onKeyDown={
          hasDetail
            ? (e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  toggleRow(event);
                }
              }
            : undefined
        }
      >
        <td className={styles.timestamp}>{formatDateTime(event.timestamp)}</td>
        <td>
          <span className={`${styles.badge} ${levelClass(event.level)}`}>
            {event.level || "info"}
          </span>
        </td>
        <td>
          <span
            className={`${styles.kindBadge} ${TONE_CLASSES[kindTone(event.kind)]}`}
          >
            {event.kind}
          </span>
        </td>
        <td>
          {event.task_file ? (
            <Link
              to={`/tasks/${encodeURIComponent(event.task_file)}`}
              className={styles.taskLink}
              onClick={(e) => e.stopPropagation()}
            >
              {event.task_file}
            </Link>
          ) : (
            <span className={styles.empty}>—</span>
          )}
        </td>
        <td className={styles.message}>{event.message}</td>
      </tr>
    );

    if (!isExpanded || !hasDetail) {
      return [mainRow];
    }

    const detailRow = (
      <tr key={`detail-${event.id}`} className={styles.detailRow}>
        <td colSpan={5}>
          <pre className={styles.detailPre}>{prettyDetail(event.detail)}</pre>
        </td>
      </tr>
    );

    return [mainRow, detailRow];
  });

  return (
    <div className={styles.page}>
      <h1 className={styles.heading}>audit log</h1>

      <div className={styles.filters}>
        <select
          className={styles.filterSelect}
          value={kind}
          onChange={(e) => setKind(e.target.value)}
        >
          <option value="">all kinds</option>
          {EVENT_KINDS.map((k) => (
            <option key={k} value={k}>
              {k}
            </option>
          ))}
        </select>

        <input
          className={styles.filterInput}
          type="text"
          placeholder="filter by task file..."
          value={taskInput}
          onChange={(e) => setTaskInput(e.target.value)}
        />

        <select
          className={styles.filterSelect}
          value={limit}
          onChange={(e) => setLimit(Number(e.target.value))}
        >
          {LIMIT_OPTIONS.map((n) => (
            <option key={n} value={n}>
              limit: {n}
            </option>
          ))}
        </select>
      </div>

      {error && <p className={styles.errorMsg}>{error}</p>}

      <div className={styles.tableWrapper}>
        {loading && <p className={styles.loading}>loading...</p>}
        <table className={styles.table}>
          <thead>
            <tr>
              <th>timestamp</th>
              <th>level</th>
              <th>kind</th>
              <th>task</th>
              <th>message</th>
            </tr>
          </thead>
          <tbody>
            {rows}
            {!loading && events.length === 0 && (
              <tr>
                <td colSpan={5} className={styles.emptyCell}>
                  no events found
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
