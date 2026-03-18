import type { TaskEntry } from "../types";
import styles from "./MetadataPanel.module.css";

interface MetadataPanelProps {
  task: TaskEntry;
}

function formatTimestamp(value?: string): string {
  if (!value) return "—";
  try {
    const d = new Date(value);
    if (isNaN(d.getTime())) return value;
    return d.toLocaleString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  } catch {
    return value;
  }
}

function renderOptional(value?: string): string {
  if (!value || value.trim() === "") return "—";
  return value;
}

export default function MetadataPanel({ task }: MetadataPanelProps) {
  const hasPR = Boolean(task.pr_review_decision || task.pr_check_status);

  return (
    <div className={styles.card}>
      <h3 className={styles.heading}>metadata</h3>

      <dl className={styles.list}>
        {task.topic && (
          <>
            <dt className={styles.term}>topic</dt>
            <dd className={styles.detail}>{task.topic}</dd>
          </>
        )}

        {task.branch && (
          <>
            <dt className={styles.term}>branch</dt>
            <dd className={`${styles.detail} ${styles.mono}`}>{task.branch}</dd>
          </>
        )}

        {task.goal && (
          <>
            <dt className={styles.term}>goal</dt>
            <dd className={styles.detail}>{task.goal}</dd>
          </>
        )}

        {task.description && (
          <>
            <dt className={styles.term}>description</dt>
            <dd className={styles.detail}>{task.description}</dd>
          </>
        )}

        {task.review_cycle !== undefined && task.review_cycle > 0 && (
          <>
            <dt className={styles.term}>review cycle</dt>
            <dd className={styles.detail}>{task.review_cycle}</dd>
          </>
        )}

        {task.clickup_task_id && (
          <>
            <dt className={styles.term}>clickup</dt>
            <dd className={`${styles.detail} ${styles.mono}`}>{task.clickup_task_id}</dd>
          </>
        )}

        <dt className={styles.term}>created</dt>
        <dd className={styles.detail}>{formatTimestamp(task.created_at)}</dd>

        {task.planning_at && (
          <>
            <dt className={styles.term}>planning at</dt>
            <dd className={styles.detail}>{formatTimestamp(task.planning_at)}</dd>
          </>
        )}

        {task.implementing_at && (
          <>
            <dt className={styles.term}>implementing at</dt>
            <dd className={styles.detail}>{formatTimestamp(task.implementing_at)}</dd>
          </>
        )}

        {task.reviewing_at && (
          <>
            <dt className={styles.term}>reviewing at</dt>
            <dd className={styles.detail}>{formatTimestamp(task.reviewing_at)}</dd>
          </>
        )}

        {task.done_at && (
          <>
            <dt className={styles.term}>done at</dt>
            <dd className={styles.detail}>{formatTimestamp(task.done_at)}</dd>
          </>
        )}

        {task.implemented && (
          <>
            <dt className={styles.term}>implemented</dt>
            <dd className={styles.detail}>{renderOptional(task.implemented)}</dd>
          </>
        )}
      </dl>

      {hasPR && (
        <div className={styles.prSection}>
          <h4 className={styles.subheading}>pull request</h4>
          {task.pr_url && (
            <a
              href={task.pr_url}
              target="_blank"
              rel="noreferrer"
              className={styles.prLink}
            >
              {task.pr_url}
            </a>
          )}
          <dl className={styles.list}>
            {task.pr_review_decision && (
              <>
                <dt className={styles.term}>review decision</dt>
                <dd className={styles.detail}>{task.pr_review_decision}</dd>
              </>
            )}
            {task.pr_check_status && (
              <>
                <dt className={styles.term}>check status</dt>
                <dd className={styles.detail}>{task.pr_check_status}</dd>
              </>
            )}
          </dl>
        </div>
      )}
    </div>
  );
}
