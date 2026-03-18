import { useEffect, useMemo, useState } from "react";
import { listTasks, listAuditEvents, resolveProjectName } from "../api";
import type { AuditEvent, Status, TaskEntry } from "../types";
import styles from "./DashboardPage.module.css";

const STATUS_ORDER: Status[] = [
  "ready",
  "planning",
  "implementing",
  "reviewing",
  "done",
  "cancelled",
];

const STATUS_BORDER_CLASSES: Record<Status, string> = {
  ready: styles.ready,
  planning: styles.planning,
  implementing: styles.implementing,
  reviewing: styles.reviewing,
  done: styles.done,
  cancelled: styles.cancelled,
};

function countByStatus(tasks: TaskEntry[]): Record<Status, number> {
  const counts: Record<Status, number> = {
    ready: 0,
    planning: 0,
    implementing: 0,
    reviewing: 0,
    done: 0,
    cancelled: 0,
  };
  for (const task of tasks) {
    if (task.status in counts) {
      counts[task.status]++;
    }
  }
  return counts;
}

function formatRelativeTime(timestamp: string): string {
  const date = new Date(timestamp);
  if (isNaN(date.getTime())) return "just now";

  const rtf = new Intl.RelativeTimeFormat("en", { numeric: "auto" });
  const diffMs = date.getTime() - Date.now();
  const diffSec = Math.round(diffMs / 1000);

  if (Math.abs(diffSec) < 60) return rtf.format(diffSec, "second");
  const diffMin = Math.round(diffMs / 60000);
  if (Math.abs(diffMin) < 60) return rtf.format(diffMin, "minute");
  const diffHr = Math.round(diffMs / 3600000);
  if (Math.abs(diffHr) < 24) return rtf.format(diffHr, "hour");
  const diffDay = Math.round(diffMs / 86400000);
  return rtf.format(diffDay, "day");
}

export default function DashboardPage() {
  const [tasks, setTasks] = useState<TaskEntry[]>([]);
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const project = useMemo(
    () => resolveProjectName(window.location.search),
    [],
  );

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    Promise.all([listTasks(project), listAuditEvents(project)])
      .then(([fetchedTasks, fetchedEvents]) => {
        if (cancelled) return;
        setTasks(fetchedTasks);
        setEvents(fetchedEvents.slice(0, 20));
        setLoading(false);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : "failed to load data");
        setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [project]);

  const counts = useMemo(() => countByStatus(tasks), [tasks]);

  if (loading) {
    return (
      <div className={styles.page}>
        <h1 className={styles.pageTitle}>dashboard</h1>
        <p className={styles.loadingText}>loading...</p>
      </div>
    );
  }

  return (
    <div className={styles.page}>
      <h1 className={styles.pageTitle}>dashboard</h1>

      {error ? (
        <div className={styles.error}>{error}</div>
      ) : (
        <>
          <h2 className={styles.sectionTitle}>task status</h2>
          <div className={styles.cardsGrid}>
            {STATUS_ORDER.map((status) => (
              <div
                key={status}
                className={`${styles.statusCard} ${STATUS_BORDER_CLASSES[status]}`}
              >
                <div className={styles.statusCardLabel}>{status}</div>
                <div className={styles.statusCardCount}>{counts[status]}</div>
              </div>
            ))}
          </div>

          <div className={styles.activitySection}>
            <h2 className={styles.sectionTitle}>recent activity</h2>
            {events.length === 0 ? (
              <div className={styles.emptyActivity}>no recent activity</div>
            ) : (
              <div className={styles.activityList}>
                {events.map((event) => (
                  <div key={event.id} className={styles.activityItem}>
                    <span className={styles.activityKind}>{event.kind}</span>
                    <span className={styles.activityMessage}>
                      {event.message || event.detail || "—"}
                    </span>
                    <span className={styles.activityTime}>
                      {formatRelativeTime(event.timestamp)}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </>
      )}
    </div>
  );
}
