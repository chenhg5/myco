import { Laptop, Plus, Server } from "lucide-react";

/** Shown on the nodes and hubs pages while the App has no hubs at all. */
export function Onboarding({ onAdd, onLocal }: { onAdd: () => void; onLocal: () => void }) {
  return (
    <div className="app-page">
      <div className="app-empty">
        <div className="app-empty-icon"><Server /></div>
        <h2>连接第一个 Hub</h2>
        <p>
          这个 App 会汇总你添加的所有 Hub 上的 agent。也可以在下面直接在这台电脑上开一个
          Hub，不需要服务器。
        </p>
        <div className="app-empty-actions">
          <button type="button" className="app-btn app-btn-primary" onClick={onAdd}>
            <Plus />添加 Hub
          </button>
          <button type="button" className="app-btn" onClick={onLocal}>
            <Laptop />在本机新建 Hub
          </button>
        </div>
        <div>
          <code className="app-code">myco console token create --name &lt;设备名&gt;</code>
        </div>
      </div>
    </div>
  );
}
