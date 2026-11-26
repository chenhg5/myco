import { readFile } from "node:fs/promises";
import { pathToFileURL } from "node:url";
import { appsPath } from "../../apps-paths.mjs";
const { installSSHFixture } = await import(pathToFileURL(appsPath("templates", "server-console-fixture.mjs")).href);
export async function appScene(c, slug, sample) {
  const { page, api, lang, T, root } = c;
  const { page: p } = await api(
    "/app/hubs/hub_studio/templates/" +
      encodeURIComponent("myco/" + slug) +
      "/use",
    "POST",
    { name: c.scenePageName || T("演示 · ", "Demo · ") + slug, sample: false, project: "website" },
  );
  const data =
    sample ||
    JSON.parse(
      await readFile(
        appsPath("dist", "official", slug, "sample.json"),
        "utf8",
      ),
    );
  for (const [coll, rows] of Object.entries(data))
    for (const { id, ...data } of rows)
      await api(
        `/app/hubs/hub_studio/pages/${p.id}/data/${coll}/${id || "main"}`,
        "PUT",
        { data },
      );
  await page.evaluate(
    (id) =>
      window.dispatchEvent(
        new CustomEvent("myco-open-page", {
          detail: { kind: "page", hubId: "hub_studio", id },
        }),
      ),
    p.id,
  );
  const f = page.locator("iframe:visible").last();
  await f.waitFor();
  const frame = await f.contentFrame();
  await frame.locator("#root > *").first().waitFor();
  await page.waitForTimeout(900);
  return { frame, id: p.id };
}
export async function coedit(c) {
  const { T, api, page } = c;
  const data = {
    team: [{ id: "main", title: T("官网发布", "Website launch"), key: "WEB" }],
    projects: [],
    comments: [],
    issues: [
      {
        id: "launch",
        n: 1,
        title: T("上线新的定价页", "Ship the new pricing page"),
        desc: T("完成审查，等待发布。", "Review complete; ready to release."),
        status: "progress",
        priority: 2,
        assignee: "build-backend",
        labels: ["Website"],
        order: 1,
        by: "mbp-frontend",
        created: "2026-11-20T09:00:00Z",
      },
      {
        id: "docs",
        n: 2,
        title: T("补全安装文档", "Complete installation docs"),
        desc: "",
        status: "todo",
        priority: 3,
        assignee: "desk-docs",
        labels: ["Docs"],
        order: 2,
        by: "mbp-frontend",
        created: "2026-11-20T09:00:00Z",
      },
      {
        id: "tests",
        n: 3,
        title: T("检查移动端布局", "Check mobile layouts"),
        desc: "",
        status: "done",
        priority: 2,
        assignee: "mbp-reviewer",
        labels: ["QA"],
        order: 3,
        by: "mbp-frontend",
        created: "2026-11-20T09:00:00Z",
      },
    ],
  };
  const { frame, id } = await appScene(c, "issues", data);
  // A change through the mock Hub's page-data API, observed by the real template.
  await api(`/app/hubs/hub_studio/pages/${id}/data/issues/launch`, "PUT", {
    data: { ...data.issues[0], status: "done" },
  });
  await page.waitForTimeout(1100);
  await frame.locator(".app.is").waitFor();
}
export async function design(c) {
  const { T } = c;
  const data = {
    board: [{ id: "main", title: T("产品探索", "Product discovery") }],
    items: [
      {
        id: "title",
        type: "text",
        text: T("把想法变成下一步", "Turn ideas into next steps"),
        x: 80,
        y: 36,
        w: 900,
        h: 50,
        font_size: 28,
        z: 1,
      },
      ...[
        T("发现问题", "Discover"),
        T("设计方案", "Design"),
        T("验证假设", "Validate"),
      ].map((text, i) => ({
        id: "n" + i,
        type: "rect",
        text,
        x: 80 + i * 320,
        y: 180,
        w: 220,
        h: 100,
        color: ["gray", "blue", "green"][i],
        font_size: 20,
        z: i + 2,
      })),
      ...[
        T(
          "记录用户的原话\n先把问题讲清楚",
          "Listen to users\nDefine the problem",
        ),
        T(
          "用草图交流\n先做最小的方案",
          "Sketch together\nStart with a small solution",
        ),
        T(
          "明确验证指标\n从反馈里做决定",
          "Choose a measure\nLearn from feedback",
        ),
      ].map((text, i) => ({
        id: "s" + i,
        type: "sticky",
        text,
        x: 80 + i * 320,
        y: 340,
        w: 220,
        h: 160,
        color: ["gray", "blue", "yellow"][i],
        z: i + 6,
      })),
      {
        id: "a1",
        type: "arrow",
        from: "n0",
        to: "n1",
        from_side: "right",
        to_side: "left",
        route: "elbow",
        text: T("明确问题", "Define"),
        color: "gray",
      },
      {
        id: "a2",
        type: "arrow",
        from: "n1",
        to: "n2",
        from_side: "right",
        to_side: "left",
        route: "elbow",
        text: T("试验", "Test"),
        color: "gray",
      },
    ],
  };
  await appScene(c, "whiteboard", data);
}
export async function motion(c) {
  const data = JSON.parse(
    await readFile(
      appsPath("dist", "official", "motion", "sample.json"),
      "utf8",
    ),
  );
  const replacements = {
    "myco 宣传片": "myco product film",
    "让 agent 一起做事": "Let agents work together",
    "一个地方，看见每个 agent 在做什么": "One place to see every agent at work",
    小标题: "Kicker",
    副标题: "Subtitle",
    标题: "Title",
    要点: "Highlights",
    数据: "Metrics",
    截图: "Preview",
    三步开始: "Get started",
    片尾: "Closing",
    派活: "Delegate",
    盯进度: "Follow progress",
    拿结果: "Get results",
  };
  if (c.lang === "en") {
    // Only the title scene is used; no untranslated marketing copy is shown.
    data.project[0].title = "myco product film";
    data.project[0].motion = "Scene 1: introduce your agent team.";
    data.scenes = data.scenes.slice(0, 1);
    for (const [k, v] of Object.entries(data.scenes[0]))
      if (typeof v === "string") {
        let x = v;
        for (const [a, b] of Object.entries(replacements))
          x = x.replaceAll(a, b);
        data.scenes[0][k] = x;
      }
  } else data.scenes = data.scenes.slice(0, 1);
  const { frame } = await appScene(c, "motion", data);
  await frame
    .locator(".mo-card")
    .first()
    .evaluate((el) => el.click());
  await c.page.waitForTimeout(1000);
}
export async function disk(c) {
  const script = await readFile(
    appsPath("templates", "shots-os.js"),
    "utf8",
  );
  await c.context.addInitScript({
    content: `if(location.pathname.startsWith('/p/')) { ${script} }`,
  });
  // Intercept only the iframe's local OS bridge; all calls remain in memory.
  await c.context.addInitScript(() => {
    if (!location.pathname.startsWith("/p/")) return;
    let b;
    Object.defineProperty(window, "myco", {
      configurable: true,
      get: () => b,
      set: (v) => {
        b = v;
        if (v && window.__FAKE_OS__) {
          v.os = window.__FAKE_OS__;
          v.features = [...(v.features || []), "os"];
        }
      },
    });
  });
  const { frame } = await appScene(c, "disk", {});
  await frame
    .locator(".dk-disk .btn-primary")
    .first()
    .evaluate((el) => el.click());
  await frame.locator(".sb-seg").first().waitFor();
}
export async function servers(c) {
  await c.context.addInitScript({
    content: `if(location.pathname.startsWith('/p/')) (${installSSHFixture.toString()})(${JSON.stringify({ lang: c.lang, theme: c.theme })});`,
  });
  const { frame } = await appScene(c, "server-console", {});
  const T = c.T;
  await frame
    .getByRole("button", {
      name: T("添加 SSH 主机", "Add SSH host"),
      exact: true,
    })
    .evaluate((el) => el.click());
  await frame
    .getByLabel(T("显示名称", "Display name"), { exact: true })
    .fill("Demo Linux");
  await frame
    .getByLabel(T("SSH 配置别名或 user@host", "SSH alias or user@host"), {
      exact: true,
    })
    .fill("fixture");
  await frame
    .getByRole("button", {
      name: T("保存并连接", "Save and connect"),
      exact: true,
    })
    .evaluate((el) => el.click());
  await frame.locator(".sv-metrics").waitFor();
}

// A real official app, driven only by synthetic shared records. No OS calls.
const launchScenes = new WeakMap();
export async function webAutomationLaunch(c) {
  const { page, context, api, T, lang } = c;
  await api("/app/mock/media-web-automation", "POST", {lang, phase:"running"});
  // Only the in-memory browser status is advertised. Native actions remain
  // unavailable, and every request is still confined to loopback by the runner.
  await context.addInitScript(({machine, profile}) => {
    if (!location.pathname.startsWith("/p/")) return;
    let bridge;
    Object.defineProperty(window, "myco", {configurable:true, get:()=>bridge, set:value=>{
      bridge=value;
      if (value) {
        value.features=[...(value.features || []), "browser"];
        value.browser={status:async()=>({protocol:1,version:"0.5.0",installed:true,supported:true,browsers:["chrome"],machine,profiles:[{id:"profile_demo",name:profile,running:true,agents:true}]}),
          approvals:{list:async()=>[]},record:{active:async()=>[]}};
      }
    }});
  }, {machine:T("公司电脑", "Office computer"),profile:T("订单后台", "Orders admin")});
  await page.reload();
  await page.locator(".app-shell").waitFor();
  await page.getByText(T("官网改版", "Website redesign"),{exact:true}).first().click();
  await page.locator(".sp-side .sp-member:visible",{hasText:T("云端助手（Codex）", "Cloud assistant (Codex)")}).click();
  const chat=page.locator(".conv-detail:visible").last();
  await chat.getByRole("button",{name:T("只看这一栏", "Show only this pane"),exact:true}).click();
  const title=T("本周订单导出", "Weekly order export");
  const titles=[T("打开订单后台", "Open orders admin"),T("筛选本周订单", "Filter this week"),T("导出订单表格", "Export orders"),T("等待文件下载", "Wait for download")];
  const nodes=[
    {id:"open",type:"navigate",url:"https://orders.example.com/admin",pos:[0,0],title:titles[0]},
    {id:"filter",type:"select",target:{locators:[{kind:"label",value:T("日期范围", "Date range")}]},value:"this-week",pos:[0,128],title:titles[1]},
    {id:"export",type:"click",target:{locators:[{kind:"role",role:"button",name:T("导出 CSV", "Export CSV")}]},pos:[0,256],title:titles[2]},
    {id:"download",type:"wait",wait:"download",into:"orders_file",pos:[0,384],title:titles[3]},
  ];
  const script={name:title,description:T("在公司电脑上导出本周订单", "Export this week's orders on Office computer"),start_url:"https://orders.example.com/admin",profile:T("订单后台", "Orders admin"),params:[],on_error:"stop",source:"agent",graph:{nodes,edges:[["open","filter"],["filter","export"],["export","download"]]}};
  const run={script_id:"weekly_orders",script_version:1,script_name:title,state:"running",node:"export",by:"cloud-assistant",by_kind:"agent",machine:T("公司电脑", "Office computer"),profile:script.profile,started_at:new Date(Date.now()-12000).toISOString(),steps:[{node:"open",state:"ok",ms:1200},{node:"filter",state:"ok",ms:800},{node:"export",state:"running"}],output:{}};
  const {frame,id}=await appScene({...c,scenePageName:T("网页自动化", "Web Automation")},"web-automation",{scripts:[{id:"weekly_orders",...script}],runs:[{id:"orders_run",...run}],drafts:[]});
  await frame.getByRole("tab",{name:T("运行记录", "Runs"),exact:true}).evaluate(el=>el.click());
  await frame.locator(".wa-run .wa-step-state.is-running").waitFor();
  launchScenes.set(page,{...c,id,frame,run,nodes});
}
export async function webAutomationLaunchVideo({page}) {
  const c=launchScenes.get(page), {api,lang,T,id,nodes}=c;
  const put=async(data)=>api(`/app/hubs/hub_studio/pages/${id}/data/runs/orders_run`,"PUT",{data});
  await api("/app/mock/media-web-automation","POST",{lang,phase:"ready"});
  await put({...c.run,state:"queued",node:"",steps:[]});
  // Reopen the conversation so its incremental message cache starts empty.
  await page.reload();
  await page.locator(".app-shell").waitFor();
  await page.getByText(T("官网改版", "Website redesign"),{exact:true}).first().click();
  await page.locator(".sp-side .sp-member:visible",{hasText:T("云端助手（Codex）", "Cloud assistant (Codex)")}).click();
  await page.locator(".conv-detail:visible").last().getByRole("button",{name:T("只看这一栏", "Show only this pane"),exact:true}).click();
  await page.evaluate(id=>window.dispatchEvent(new CustomEvent("myco-open-page",{detail:{kind:"page",hubId:"hub_studio",id}})),id);
  const frame=await page.locator("iframe:visible").last().contentFrame();
  await frame.getByRole("tab",{name:T("运行记录", "Runs"),exact:true}).evaluate(el=>el.click());
  const input=page.locator(".conv-detail:visible .conv-compose-input").last();
  await input.fill(T("帮我去「公司电脑」的后台，把本周订单导出成表格。", "Export this week's orders from the admin site on my Office computer."));
  await page.waitForTimeout(1700);
  await input.fill("");
  await api("/app/mock/media-web-automation","POST",{lang,phase:"running"});
  for (let i=0;i<nodes.length;i++) {
    await put({...c.run,started_at:new Date(Date.now()-i*2400).toISOString(),node:nodes[i].id,steps:nodes.slice(0,i+1).map((n,j)=>({node:n.id,state:j===i?"running":"ok",...(j<i?{ms:800+j*300}:{})}))});
    await frame.locator(".wa-step-state.is-running").waitFor();
    await page.waitForTimeout(2400);
  }
  await put({...c.run,state:"succeeded",node:"download",ended_at:new Date().toISOString(),steps:nodes.map((n,i)=>({node:n.id,state:"ok",ms:800+i*300})),output:{orders_file:"orders-this-week.csv",orders:128}});
  await api("/app/mock/media-web-automation","POST",{lang,phase:"done"});
  await frame.getByText("orders-this-week.csv",{exact:false}).waitFor();
  await page.waitForTimeout(1400);
}
