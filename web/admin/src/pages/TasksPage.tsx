import { useState, useEffect } from "react";
import { Link, useLocation } from "react-router";
import StatusBadge from "../components/StatusBadge";
import { listTasks, resolveProjectName } from "../api";
import type { Status, TaskEntry } from "../types";
import styles from "./TasksPage.module.css";

type TaskFilter = "all" | Status;
const FILTERS: TaskFilter[] = [
  "all",
  "ready",
  "planning",
  "implementing",
  "reviewing",
  "done",
  "cancelled",
];

function truncate(text: string, max = 80): string {
  if (!text) return "";
  if (text.length <= max) return text;
  return text.slice(0, max) + "…";
}

function formatDate(value?: string): string {
  if (!value) return "—";
  const d = new Date(value);
  if (isNaN(d.getTime())) return "—";
  return d.toISOString().slice(0, 10);
}

export default function TasksPage() {
  const location = useLocation();
  const project = resolveProjectName(location.search);

  const [allTasks, setAllTasks] = useState<TaskEntry[]>([]);
  const [statusFilter, setStatusFilter] = useState<TaskFilter>("all");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setLoading(true);
    setError(null);
    listTasks(project)
      .then((data) => {
        const sorted = [...data].sort(
          (a, b) =>
            new Date(b.created_at ?? 0).getTime() -
            new Date(a.created_at ?? 0).getTime(),
        );
        setAllTasks(sorted);
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : "unknown error");
      })
      .finally(() => setLoading(false));
  }, [project]);

  const tasks =
    statusFilter === "all"
      ? allTasks
      : allTasks.filter((t) => t.status === statusFilter);

  const countLabel =
    statusFilter === "all"
      ? `${tasks.length} tasks`
      : `${tasks.length} ${statusFilter} tasks`;

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.title}>{loading ? "tasks" : countLabel}</h1>
        <div className={styles.filters}>
          {FILTERS.map((f) => (
            <button
              key={f}
              className={`${styles.filterBtn} ${statusFilter === f ? styles.filterActive : ""}`}
              onClick={() => setStatusFilter(f)}
            >
              {f}
            </button>
          ))}
        </div>
      </div>

      {error && <div className={styles.error}>{error}</div>}

      {loading && <div className={styles.loading}>loading...</div>}

      {!loading && !error && tasks.length === 0 && (
        <div className={styles.empty}>no tasks found</div>
      )}

      {!loading && tasks.length > 0 && (
        <div className={styles.tableWrapper}>
          <table className={styles.table}>
            <thead>
              <tr>
                <th>status</th>
                <th>filename</th>
                <th>goal</th>
                <th>topic</th>
                <th>branch</th>
                <th>created</th>
              </tr>
            </thead>
            <tbody>
              {tasks.map((task) => (
                <tr key={task.filename} className={styles.row}>
                  <td>
                    <StatusBadge status={task.status} />
                  </td>
                  <td>
                    <Link
                      to={`/tasks/${encodeURIComponent(task.filename)}`}
                      className={styles.taskLink}
                    >
                      {task.filename}
                    </Link>
                  </td>
                  <td className={styles.goalCell}>
                    {truncate(task.goal ?? "")}
                  </td>
                  <td>{task.topic || "—"}</td>
                  <td className={styles.branchCell}>{task.branch || "—"}</td>
                  <td>{formatDate(task.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
