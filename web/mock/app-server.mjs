// Dev-only stand-in for the App backend (cmd/myco-app). No dependencies.
//   node mock/app-server.mjs        (or: npm run dev:app)
// Serves the `app/...` contract with two hubs: one healthy, one unreachable.
// Writes mutate in-memory state and are lost on restart.
import http from "node:http";

const PORT = Number(process.env.APP_MOCK_PORT || 7781);
// MOCK_SHELL=mac reserves room for the macOS traffic lights; browser does not.
const SHELL = process.env.MOCK_SHELL === "browser" ? "browser" : "mac";
const iso = (offsetSec = 0) => new Date(Date.now() - offsetSec * 1000).toISOString();
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// CLI metadata used when a session is spawned from the App.
const CLI_MODELS = {
  claude: "claude-opus-5-5", codex: "gpt-6-sol", pi: "pi-3", kiro: "kiro-2",
  cursor: "cursor-auto", gemini: "gemini-3-pro", opencode: "claude-opus-5-5",
};
const CLI_VERSIONS = {
  claude: "2.1.240", codex: "0.52.0", pi: "1.8.0", kiro: "0.4.2",
  cursor: "0.48.0", gemini: "0.12.0", opencode: "0.6.1",
};

function makeAgent(over) {
  const base = {
    id: "ag_" + Math.random().toString(36).slice(2, 10),
    name: "agent",
    description: "",
    capabilities: [],
    labels: [],
    kind: "agent",
    binding: { harness: "claude", session_id: "sess_" + Math.random().toString(36).slice(2, 12) },
    delivery: { mode: "pull" },
    allow_from: [],
    online: true,
    created_at: iso(86_400 * 12),
    last_seen_at: iso(20),
  };
  return { ...base, ...over, binding: { ...base.binding, ...(over.binding || {}) }, delivery: { ...base.delivery, ...(over.delivery || {}) } };
}

let msgSeq = 0;
function makeMessage(over) {
  msgSeq += 1;
  const base = {
    id: `msg_${msgSeq}`,
    from: "glenn", to: "agent", thread_id: "th_demo", type: "task",
    body: "", status: "delivered", attempts: 1,
    created_at: iso(120), updated_at: iso(60),
  };
  return { ...base, ...over };
}

const studioThread = [
  makeMessage({ thread_id: "th_release", from: "glenn", to: "pm-codex", type: "task", body: "发布 v0.16.8：改 VERSION，跑 make check，打 tag 并推送。", created_at: iso(900), updated_at: iso(900) }),
  makeMessage({ thread_id: "th_release", from: "pm-codex", to: "glenn", type: "reply", body: "release/v0.16.8 分支检查全部通过，等你确认后打 tag。", created_at: iso(600), updated_at: iso(600) }),
  makeMessage({ thread_id: "th_release", from: "glenn", to: "pm-codex", type: "task", body: "可以，打 tag 吧。", created_at: iso(300), updated_at: iso(300) }),
];

const state = {
  hubs: [
    {
      id: "hub_studio",
      name: "Studio Mac",
      url: "http://192.168.1.20:7780",
      token: "myco_console_demo_studio",
      ok: true,
      hub_version: "0.16.6",
      update_available: true,
      threads: { th_release: studioThread },
      machines: [
        {
          id: "m_studio", name: "Studio Mac", hostname: "studio.local", os: "darwin", arch: "arm64",
          myco_version: "0.16.8", online: true, last_seen_at: iso(5), can_spawn: true,
        },
        {
          id: "m_bench", name: "Bench Linux", hostname: "bench-01", os: "linux", arch: "x86_64",
          myco_version: "0.9.4", online: true, last_seen_at: iso(30), can_spawn: false,
        },
        {
          id: "m_office", name: "Office PC", hostname: "office-pc", os: "windows", arch: "amd64",
          myco_version: "0.16.6", online: true, last_seen_at: iso(90), can_spawn: true,
        },
      ],
      agents: [
        makeAgent({
          name: "pm-codex", display_name: "PM (Codex)", description: "规划发布并审查 PR。",
          kind: "agent", machine_id: "m_studio",
          runtime: { cli: "codex", cli_version: "0.52.0", model: "gpt-6-sol", model_source: "configured", access: "workspace" },
          status_text: "正在审查 multihub 分支", status_at: iso(180),
          labels: ["machine=studio-mac", "group=release", "role=pm"], capabilities: ["review", "release"],
          binding: { harness: "codex", environment: "macOS 26.1", workdir: "~/code/myco" },
          aliases: ["pm"], allow_from: ["glenn"], enrolled_by: "console",
        }),
        makeAgent({
          name: "docs-claude", description: "保持 README 和 docs/ 与代码同步。",
          kind: "agent", machine_id: "m_studio",
          runtime: { cli: "claude", cli_version: "2.1.240", model: "claude-opus-5-5", model_source: "observed", access: "readonly" },
          labels: ["machine=studio-mac", "group=docs"], capabilities: ["docs"],
          online: false, last_seen_at: iso(5400), status_at: iso(5400),
          binding: { harness: "claude", environment: "macOS 26.1", workdir: "~/code/myco" },
        }),
        makeAgent({
          name: "ios-shell", display_name: "iOS Shell", description: "按需构建和测试 iOS App。",
          kind: "shell", machine_id: "m_bench",
          runtime: { cli: "shell", access: "workspace" },
          status_text: "正在运行 make test-ios", status_at: iso(45),
          labels: ["machine=bench-01", "group=ios"], capabilities: ["build", "test"],
          binding: { harness: "shell", environment: "Linux 6.8", workdir: "~/code/app-ios" },
          delivery: { mode: "webhook", url: "http://bench-01:7790/hook", has_secret: true },
        }),
        makeAgent({
          name: "glenn", display_name: "Glenn", description: "人。负责发任务和验收。",
          kind: "human", binding: { harness: "human" },
          labels: ["role=human"], status_at: iso(3),
        }),
        makeAgent({
          name: "release-cloud", display_name: "Release (Claude 云端)", description: "在 claude.ai 上整理发布说明。",
          kind: "agent", runtime: { cli: "claude-cloud", model: "claude-opus-5-5", model_source: "default", access: "readonly" },
          status_text: "正在整理发布说明", status_at: iso(60),
          labels: ["group=release"], binding: { harness: "claude-cloud", environment: "claude.ai" },
        }),
        makeAgent({
          name: "review-cloud", display_name: "Review (Codex 云端)", description: "在 Codex 云端做代码审查。",
          kind: "agent", runtime: { cli: "codex-cloud", model: "gpt-6-sol", model_source: "observed", access: "readonly" },
          labels: ["group=release"], binding: { harness: "codex-cloud", environment: "Codex 云端" },
        }),
        makeAgent({
          name: "legacy-codex", description: "旧版 runner，还没有上报运行信息。",
          kind: "agent", machine_id: "m_bench",
          labels: ["machine=bench-01"], online: true, status_at: iso(20),
          binding: { harness: "codex", environment: "Linux 6.8", workdir: "~/code/legacy" },
        }),
        makeAgent({
          name: "studio-mac-machine", display_name: "Studio Mac", kind: "machine", machine_id: "m_studio",
          labels: ["machine=studio-mac"], binding: { harness: "machine" },
        }),
        makeAgent({
          name: "bench-01-machine", display_name: "Bench Linux", kind: "machine", machine_id: "m_bench",
          labels: ["machine=bench-01"], binding: { harness: "machine" },
        }),
      ],
      recent: [
        makeMessage({ thread_id: "th_release", from: "glenn", to: "pm-codex", type: "task", body: "可以，打 tag 吧。", created_at: iso(300), updated_at: iso(300) }),
        makeMessage({ thread_id: "th_release", from: "pm-codex", to: "glenn", type: "reply", body: "release/v0.16.8 分支检查全部通过，等你确认后打 tag。", created_at: iso(600), updated_at: iso(600) }),
        makeMessage({ thread_id: "th_ios", from: "glenn", to: "ios-shell", type: "task", body: "请跑一遍 iOS 测试套件，附上日志。", created_at: iso(240), updated_at: iso(240) }),
      ],
    },
    {
      id: "hub_lab",
      name: "Lab Linux",
      url: "http://10.0.0.42:7780",
      token: "myco_console_demo_lab",
      ok: false,
      error: "dial tcp 10.0.0.42:7780: connect: connection refused",
      hub_version: "0.16.4",
      update_available: true,
      agents: [],
      threads: {},
      recent: [],
    },
  ],
  counter: 0,
};

// This machine: the CLI starts out missing so the install flow can be tried,
// cloudflared is present, Tailscale is not.
const local = {
  myco: null,
  hub: null,
  tools: {
    cloudflared: { path: "/opt/homebrew/bin/cloudflared" },
    tailscale: null,
  },
  device: "glenn-mac",
};

function totals(agents) {
  const t = { agents: agents.length, working: 0, pending: 0, idle: 0, offline: 0 };
  for (const a of agents) {
    const s = a.online ? (a.status_text ? "working" : "idle") : "offline";
    t[s] += 1;
  }
  return t;
}

function agentStatus(a) {
  const state = a.online ? (a.status_text ? "working" : "idle") : "offline";
  return {
    agent: a, state,
    queued: state === "working" ? 2 : 0,
    in_progress: state === "working" ? 1 : 0,
    held: 0,
    current: state === "working"
      ? [makeMessage({ thread_id: "th_release", from: "glenn", to: a.name, body: a.status_text || "处理中…" })]
      : [],
  };
}

function hubView(h) {
  const view = { id: h.id, name: h.name, url: h.url, ok: h.ok, hub_version: h.hub_version, update_available: h.update_available };
  if (!h.ok) view.error = h.error;
  return view;
}

function overviewFor(h) {
  return {
    generated_at: iso(0),
    viewer: { kind: "admin" },
    online_window_seconds: 120,
    totals: totals(h.agents),
    agents: h.agents.map(agentStatus),
    // Real machines only; cloud sessions are not machines.
    machines: (h.machines || []).map((m) => ({
      ...m,
      agents: h.agents.filter((a) => a.kind !== "machine" && a.machine_id === m.id).map((a) => a.name),
    })),
    recent: h.recent,
  };
}

function appOverview() {
  return {
    generated_at: iso(0),
    hubs: state.hubs.map((h) => h.ok
      ? { id: h.id, name: h.name, ok: true, overview: overviewFor(h) }
      : { id: h.id, name: h.name, ok: false, error: h.error }),
  };
}

function send(res, status, data) {
  const body = data === undefined ? "" : JSON.stringify(data);
  res.writeHead(status, { "Content-Type": "application/json; charset=utf-8", "Cache-Control": "no-store" });
  res.end(body);
}

function fail(res, status, error, message, suggestion) {
  send(res, status, { error, message, ...(suggestion ? { suggestion } : {}) });
}

function readBody(req) {
  return new Promise((resolve) => {
    let raw = "";
    req.on("data", (c) => { raw += c; if (raw.length > 1e6) req.destroy(); });
    req.on("end", () => {
      try { resolve(raw ? JSON.parse(raw) : {}); } catch { resolve(null); }
    });
  });
}

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url || "/", `http://${req.headers.host || "localhost"}`);
  const parts = url.pathname.split("/").filter(Boolean).map(decodeURIComponent);
  const method = req.method || "GET";
  console.error(`${method} ${url.pathname}`);

  if (method === "OPTIONS") { res.writeHead(204); res.end(); return; }

  // /app/hubs
  if (parts[0] === "app" && parts[1] === "hubs" && parts.length === 2) {
    if (method === "GET") return send(res, 200, state.hubs.map(hubView));
    if (method === "POST") {
      const body = await readBody(req);
      if (!body || typeof body.name !== "string" || typeof body.url !== "string" || typeof body.token !== "string") {
        return fail(res, 400, "invalid_request", "name、url 和 token 为必填项");
      }
      if (body.token.trim().length < 8 || !body.token.startsWith("myco_")) {
        return fail(res, 401, "invalid_token", "Hub 拒绝了这个控制台令牌。",
          "请在 Hub 所在机器上运行 myco console token create --name <设备名> 重新创建。");
      }
      if (state.hubs.some((h) => h.url === body.url.trim())) {
        return fail(res, 409, "already_exists", "这个 Hub 地址已经添加过了。");
      }
      state.counter += 1;
      const hub = {
        id: `hub_added_${state.counter}`, name: body.name.trim(), url: body.url.trim(),
        token: body.token.trim(), ok: true, hub_version: "0.16.6", update_available: false,
        agents: [], threads: {}, recent: [],
      };
      state.hubs.push(hub);
      return send(res, 201, hubView(hub));
    }
    return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
  }

  // /app/overview
  if (parts[0] === "app" && parts[1] === "overview" && parts.length === 2) {
    if (method !== "GET") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    return send(res, 200, appOverview());
  }

  // /app/info
  if (parts[0] === "app" && parts[1] === "info" && parts.length === 2) {
    if (method !== "GET") return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
    return send(res, 200, { version: "v0.16.8-demo", shell: SHELL });
  }

  // /app/local[...]
  if (parts[0] === "app" && parts[1] === "local") {
    if (parts.length === 2 && method === "GET") return send(res, 200, local);

    if (parts.length === 3 && parts[2] === "myco" && method === "POST") {
      if (!local.myco) {
        await sleep(2000);
        local.myco = { path: "/Users/glenn/.local/bin/myco", version: "0.16.8" };
      }
      return send(res, 200, local.myco);
    }

    if (parts.length === 3 && parts[2] === "hub" && method === "POST") {
      const body = await readBody(req);
      const tunnel = body?.tunnel;
      if (!["cloudflare", "tailscale", "none"].includes(tunnel)) {
        return fail(res, 400, "invalid_request", "tunnel 必须是 cloudflare、tailscale 或 none。");
      }
      if (!local.myco) {
        return fail(res, 409, "myco_missing", "还没有安装 myco 命令行。", "先在这个页面点“安装”。");
      }
      const hostname = typeof body.hostname === "string" ? body.hostname.trim() : "";
      if (tunnel === "cloudflare" && !local.tools.cloudflared) {
        return fail(res, 409, "cloudflared_missing", "没有找到 cloudflared。", "在终端运行 brew install cloudflared。");
      }
      if (tunnel === "tailscale" && !local.tools.tailscale) {
        return fail(res, 409, "tailscale_missing", "没有找到 Tailscale。", "先安装 Tailscale 并为这台设备开启 Funnel。");
      }
      if (tunnel === "cloudflare" && hostname && !/^[a-z0-9.-]+$/i.test(hostname)) {
        return fail(res, 400, "invalid_hostname", "域名看起来不对。", "例如 hub.example.com。");
      }

      await sleep(4000);

      const publicUrl = tunnel === "none"
        ? null
        : tunnel === "tailscale"
          ? "https://glenn-mac.tail1a2b.ts.net"
          : hostname
            ? `https://${hostname}`
            : "https://quiet-forest-4821.trycloudflare.com";
      const warnings = tunnel === "none"
        ? ["只在本机和同一局域网内可用。"]
        : tunnel === "tailscale"
          ? ["请确认已在这台设备上开启 Funnel。"]
          : hostname
            ? ["请确认域名已托管在 Cloudflare，并且执行过一次 cloudflared tunnel login。"]
            : ["临时地址会在 Hub 重启后变化。", "这台电脑休眠或关机时，其他机器会连不上。"];

      local.hub = {
        running: true,
        local_url: "http://127.0.0.1:7780",
        ...(publicUrl ? { public_url: publicUrl } : {}),
        version: "0.16.8",
        added: true,
      };

      const appUrl = publicUrl || local.hub.local_url;
      let appHub = state.hubs.find((h) => h.id === "hub_local");
      if (!appHub) {
        appHub = {
          id: "hub_local", name: `${local.device} 本机`, url: appUrl,
          token: "myco_console_local_demo", ok: true, hub_version: "0.16.8",
          update_available: false, agents: [], threads: {}, recent: [],
        };
        state.hubs.push(appHub);
      } else {
        appHub.url = appUrl;
        appHub.ok = true;
        delete appHub.error;
      }

      return send(res, 200, {
        hub: hubView(appHub),
        public_url: publicUrl || local.hub.local_url,
        ...(publicUrl ? { join_url: `${publicUrl}/join/demo-enroll-token` } : {}),
        warnings,
      });
    }

    if (parts.length === 4 && parts[2] === "hub" && parts[3] === "stop" && method === "POST") {
      if (local.hub) local.hub.running = false;
      const appHub = state.hubs.find((h) => h.id === "hub_local");
      if (appHub) {
        appHub.ok = false;
        appHub.error = "本机 Hub 已停止";
      }
      return send(res, 200, { stopped: true });
    }

    return fail(res, 405, "method_not_allowed", `不支持 ${method} 请求`);
  }

  // /app/hubs/{id}[/...]
  if (parts[0] === "app" && parts[1] === "hubs" && parts.length >= 3) {
    const hub = state.hubs.find((h) => h.id === parts[2]);
    if (!hub) return fail(res, 404, "not_found", "没有这个 Hub");

    if (parts.length === 3 && method === "DELETE") {
      state.hubs = state.hubs.filter((h) => h.id !== hub.id);
      if (hub.id === "hub_local" && local.hub) local.hub.added = false;
      return send(res, 204);
    }

    if (parts.length === 5 && parts[3] === "threads" && method === "GET") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      return send(res, 200, { messages: hub.threads[parts[4]] || [] });
    }

    // /app/hubs/{id}/links — a one-off enrollment link for adding a machine.
    if (parts.length === 4 && parts[3] === "links" && method === "POST") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const body = await readBody(req);
      const uses = Number.isInteger(body?.uses) ? body.uses : 1;
      const hours = Number.isInteger(body?.expires_hours) ? body.expires_hours : 24;
      state.counter += 1;
      const joinUrl = `${hub.url.replace(/\/+$/, "")}/join/demo-${state.counter}`;
      return send(res, 200, {
        join_url: joinUrl,
        command: `curl -fsSL ${joinUrl} | bash`,
        uses,
        expires_hours: hours,
      });
    }

    // /app/hubs/{id}/machines/{mid}
    if (parts.length === 5 && parts[3] === "machines") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const machine = (hub.machines || []).find((m) => m.id === parts[4]);
      if (!machine) return fail(res, 404, "not_found", "没有这台机器");
      if (method === "PATCH") {
        const body = await readBody(req);
        const name = typeof body?.name === "string" ? body.name.trim() : "";
        if (!name) return fail(res, 400, "invalid_request", "name 不能为空。");
        machine.name = name;
        const agents = hub.agents.filter((a) => a.kind !== "machine" && a.machine_id === machine.id).length;
        return send(res, 200, { ...machine, agents });
      }
    }

    // /app/hubs/{id}/machines/{mid}/spawn — start a session on that machine.
    if (parts.length === 6 && parts[3] === "machines" && parts[5] === "spawn" && method === "POST") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const machine = (hub.machines || []).find((m) => m.id === parts[4]);
      if (!machine) return fail(res, 404, "not_found", "没有这台机器");
      if (!machine.can_spawn) {
        return fail(res, 403, "machine_cannot_spawn", "这台机器不能开新会话。",
          "它的 myco 版本太旧，或者 runner 没在运行。");
      }
      const body = await readBody(req);
      if (!body || !(body.cli in CLI_MODELS)) {
        return fail(res, 400, "invalid_request", "cli 必须是 claude、codex、pi、kiro、cursor、gemini 或 opencode。");
      }
      const dir = typeof body.dir === "string" ? body.dir.trim() : "";
      if (!dir) return fail(res, 400, "invalid_request", "dir 为必填项。");
      if (dir === "/etc" || dir.startsWith("/etc/")) {
        return fail(res, 403, "dir_not_allowed", "目录不在允许范围内。");
      }
      const access = body.access === undefined ? "workspace" : body.access;
      if (!["readonly", "workspace", "full"].includes(access)) {
        return fail(res, 400, "invalid_request", "access 必须是 readonly、workspace 或 full。");
      }

      await sleep(3000);

      state.counter += 1;
      const wanted = typeof body.name === "string" ? body.name.trim() : "";
      const stem = wanted || `${body.cli}-${state.counter}`;
      let name = stem;
      let n = 2;
      while (hub.agents.some((a) => a.name === name)) { name = `${stem}-${n}`; n += 1; }

      const model = typeof body.model === "string" ? body.model.trim() : "";
      const osLabel = { darwin: "macOS", linux: "Linux", windows: "Windows", cloud: "云端" }[machine.os] || machine.os;
      const agent = makeAgent({
        name,
        description: typeof body.description === "string" ? body.description : "",
        kind: "agent",
        machine_id: machine.id,
        runtime: {
          cli: body.cli,
          cli_version: CLI_VERSIONS[body.cli],
          model: model || CLI_MODELS[body.cli],
          model_source: model ? "configured" : "default",
          access,
        },
        status_text: "刚启动，等待任务",
        status_at: iso(0),
        created_at: iso(0),
        last_seen_at: iso(0),
        labels: [`machine=${machine.name.toLowerCase().replace(/\s+/g, "-")}`],
        binding: { harness: body.cli, environment: osLabel, workdir: dir },
      });
      hub.agents.push(agent);

      return send(res, 200, {
        agent,
        session_id: agent.binding.session_id,
        cli: body.cli,
        workdir: dir,
        access,
        reply: `环境已就绪。我在 ${dir} 里，随时可以开始工作。`,
      });
    }

    if (parts.length >= 5 && parts[3] === "agents") {
      if (!hub.ok) return fail(res, 502, "hub_unreachable", hub.error || "Hub 连不上");
      const agent = hub.agents.find((a) => a.name === parts[4]);
      if (!agent) return fail(res, 404, "not_found", `没有名为 ${parts[4]} 的节点`);

      if (parts.length === 5 && method === "DELETE") {
        hub.agents = hub.agents.filter((a) => a.name !== agent.name);
        hub.recent = hub.recent.filter((m) => m.from !== agent.name && m.to !== agent.name);
        return send(res, 204);
      }
      if (parts.length === 6 && parts[5] === "profile" && method === "PUT") {
        const body = await readBody(req);
        if (body?.display_name !== undefined) agent.display_name = body.display_name || undefined;
        if (body?.description !== undefined) agent.description = body.description;
        return send(res, 200, agent);
      }
      if (parts.length === 6 && parts[5] === "allow_from" && method === "PUT") {
        const body = await readBody(req);
        if (!Array.isArray(body?.allow_from)) return fail(res, 400, "invalid_request", "allow_from 必须是字符串数组");
        agent.allow_from = body.allow_from.map(String);
        return send(res, 200, agent);
      }
      if (parts.length === 6 && parts[5] === "kick" && method === "POST") {
        agent.online = false;
        agent.status_text = undefined;
        agent.last_seen_at = iso(0);
        return send(res, 204);
      }
    }
    return fail(res, 404, "not_found", `没有匹配的路由：${method} ${url.pathname}`);
  }

  return fail(res, 404, "not_found", `没有匹配的路由：${method} ${url.pathname}`);
});

server.listen(PORT, "127.0.0.1", () => {
  console.error(`myco app mock listening on http://127.0.0.1:${PORT}`);
});
