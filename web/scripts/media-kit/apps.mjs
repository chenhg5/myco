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
    { name: T("演示 · ", "Demo · ") + slug, sample: false, project: "website" },
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
