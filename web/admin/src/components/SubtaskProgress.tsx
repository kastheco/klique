import type { SubtaskEntry, SubtaskStatus } from "../types";
import styles from "./SubtaskProgress.module.css";

interface SubtaskProgressProps {
  subtasks: SubtaskEntry[];
}

function completedCount(subtasks: SubtaskEntry[]): number {
  return subtasks.filter(
    (s) => s.status === "complete" || s.status === "done" || s.status === "closed",
  ).length;
}

function statusClass(status: SubtaskStatus): string {
  switch (status) {
    case "complete":
      return styles.statusComplete;
    case "done":
      return styles.statusDone;
    case "closed":
      return styles.statusClosed;
    case "running":
      return styles.statusRunning;
    case "failed":
      return styles.statusFailed;
    case "blocked":
      return styles.statusBlocked;
    case "in_review":
      return styles.statusInReview;
    case "pending":
      return styles.statusPending;
    default:
      return styles.statusUnknown;
  }
}

export default function SubtaskProgress({ subtasks }: SubtaskProgressProps) {
  const total = subtasks.length;
  const done = completedCount(subtasks);
  const percent = total === 0 ? 0 : Math.round((done / total) * 100);

  return (
    <div className={styles.card}>
      <h3 className={styles.heading}>subtasks</h3>

      <div className={styles.summary}>
        <span className={styles.count}>
          {done}/{total} complete
        </span>
        <span className={styles.percent}>{percent}%</span>
      </div>

      <div className={styles.progressTrack}>
        <div
          className={styles.progressFill}
          style={{ width: `${percent}%` }}
          role="progressbar"
          aria-valuenow={percent}
          aria-valuemin={0}
          aria-valuemax={100}
        />
      </div>

      {total === 0 ? (
        <p className={styles.empty}>no subtasks recorded</p>
      ) : (
        <ul className={styles.list}>
          {subtasks.map((st) => (
            <li key={st.task_number} className={styles.item}>
              <span className={`${styles.dot} ${statusClass(st.status)}`} />
              <span className={styles.taskNum}>#{st.task_number}</span>
              <span className={styles.title}>{st.title}</span>
              <span className={`${styles.badge} ${statusClass(st.status)}`}>
                {st.status === "in_review" ? "in review" : st.status}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
