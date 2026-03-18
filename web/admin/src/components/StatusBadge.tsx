import styles from "./StatusBadge.module.css";

export type StatusBadgeProps = { status: string };

const STATUS_LABELS: Record<string, string> = {
  ready: "ready",
  planning: "planning",
  implementing: "implementing",
  reviewing: "reviewing",
  done: "done",
  cancelled: "cancelled",
  pending: "pending",
  running: "running",
  complete: "complete",
  failed: "failed",
  closed: "closed",
  blocked: "blocked",
  in_review: "in review",
};

const STATUS_CLASSES: Record<string, string> = {
  ready: styles.ready,
  planning: styles.planning,
  implementing: styles.implementing,
  reviewing: styles.reviewing,
  done: styles.done,
  cancelled: styles.cancelled,
  pending: styles.pending,
  running: styles.running,
  complete: styles.complete,
  failed: styles.failed,
  closed: styles.closed,
  blocked: styles.blocked,
  in_review: styles.inReview,
};

export default function StatusBadge({ status }: StatusBadgeProps) {
  const label = STATUS_LABELS[status] ?? status;
  const cls = STATUS_CLASSES[status] ?? styles.unknown;
  return <span className={`${styles.badge} ${cls}`}>{label}</span>;
}
