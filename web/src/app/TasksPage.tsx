import { ListTodo } from "lucide-react";

/** Placeholder for the task queue view that comes in a later version. */
export function TasksPage() {
  return (
    <div className="app-page">
      <div className="app-empty" style={{ marginTop: "14vh" }}>
        <div className="app-empty-icon"><ListTodo /></div>
        <h2>任务即将推出</h2>
        <p>以后可以在这里查看所有 Hub 上的任务队列、进度和结果。</p>
      </div>
    </div>
  );
}
