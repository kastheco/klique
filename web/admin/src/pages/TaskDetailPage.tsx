import { useEffect, useMemo, useState } from "react";
import { useLocation, useParams } from "react-router";
import ReactMarkdown from "react-markdown";
import type { TaskEntry, SubtaskEntry } from "../types";
import { getTask, getTaskContent, getSubtasks, resolveProjectName } from "../api";
import StatusBadge from "../components/StatusBadge";
import MetadataPanel from "../components/MetadataPanel";
import SubtaskProgress from "../components/SubtaskProgress";
import styles from "./TaskDetailPage.module.css";

export default function TaskDetailPage() {
  const { filename: rawFilename } = useParams<{ filename: string }>();
  const filename = rawFilename ? decodeURIComponent(rawFilename) : undefined;

  const { search } = useLocation();
  const project = useMemo(() => resolveProjectName(search), [search]);

  const [task, setTask] = useState<TaskEntry | null>(null);
  const [content, setContent] = useState<string>("");
  const [subtasks, setSubtasks] = useState<SubtaskEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!filename) return;

    let cancelled = false;

    setLoading(true);
    setError(null);

    Promise.all([
      getTask(project, filename),
      getTaskContent(project, filename),
      getSubtasks(project, filename),
    ])
      .then(([taskData, contentData, subtasksData]) => {
        if (cancelled) return;
        setTask(taskData);
        setContent(contentData);
        setSubtasks(subtasksData);
        setLoading(false);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(err instanceof Error ? err.message : "failed to load task");
        setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [filename, project]);

  if (!filename) {
    return (
      <div className={styles.errorPanel}>
        <p className={styles.errorText}>no task filename provided</p>
      </div>
    );
  }

  if (loading) {
    return (
      <div className={styles.loadingPanel}>
        <p className={styles.loadingText}>loading task…</p>
      </div>
    );
  }

  if (error || !task) {
    return (
      <div className={styles.errorPanel}>
        <p className={styles.errorText}>{error ?? "task not found"}</p>
      </div>
    );
  }

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <h1 className={styles.title}>{filename}</h1>
        <div className={styles.badges}>
          <StatusBadge status={task.status} />
          {task.topic && task.topic.trim() !== "" && (
            <span className={styles.topicPill}>{task.topic}</span>
          )}
        </div>
      </header>

      <div className={styles.layout}>
        <section className={styles.main}>
          <div className={styles.markdown}>
            <ReactMarkdown>{content}</ReactMarkdown>
          </div>
        </section>

        <aside className={styles.sidebar}>
          <MetadataPanel task={task} />
          <SubtaskProgress subtasks={subtasks} />
        </aside>
      </div>
    </div>
  );
}
