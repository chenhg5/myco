// Translates fixture copy only. Product UI translations remain the App's own.
const pairs = [
  ["写-审-改", "Write–review–revise"],
  ["改完跑测试", "Change and test"],
  ["分头做再汇总", "Parallel work and synthesis"],
  ["发布前问你一句", "Ask before release"],
  [
    "一个做、一个审，没过就改，最多 3 轮；最后你点头。",
    "One agent creates, another reviews; revise up to three times, then ask you.",
  ],
  [
    "改完在 MacBook 上跑测试；没过把报错交回去再改。",
    "Run tests after the change; send failures back for another revision.",
  ],
  [
    "设置页加一个「导出对话」按钮",
    "Add an Export conversation button to Settings",
  ],
  [
    "README 补上 Windows 安装说明",
    "Add Windows setup instructions to the README",
  ],
  ["修 Safari 上登录闪退", "Fix the Safari sign-in crash"],
  ["每天早上汇总 issue", "Morning issue digest"],
  ["CI 失败时排查", "Investigate CI failures"],
  ["新 issue 走写-审-改", "Review each new issue"],
  [
    "看一下昨天新开的 issue，挑出要紧的，在这里回我一份清单。",
    "Review yesterday’s new issues and send back the important ones.",
  ],
  [
    "CI 构建失败了，看一下日志找出原因，能修就修，修不了告诉我卡在哪：",
    "CI failed. Inspect the logs, fix what you can, and report any blocker:",
  ],
  ["修 issue：", "Fix issue: "],
  ["要做什么", "Task"],
  ["改动在哪个分支", "Branch with the change"],
  ["能不能合", "Ready to merge"],
  ["要改的地方", "Requested changes"],
  ["完成这件事：", "Complete this task: "],
  ["审一下 ", "Review "],
  [" 的改动：", " changes: "],
  ["审查", "Review"],
  ["你来定", "Your decision"],
  ["就这样", "Approve"],
  ["不要了", "Cancel"],
  [
    "「{{steps.write.summary}}」审过了。就这样吗？",
    "“{{steps.write.summary}}” passed review. Approve?",
  ],
  ["首页加载慢，排查一下", "Investigate slow home-page loading"],
  ["登录页适配深色模式", "Add dark mode to sign-in"],
  ["导出 CSV 加上表头", "Include headers in CSV exports"],
  ["修复搜索结果重复", "Fix duplicate search results"],
  [
    "交的结果不符合要求：approved is missing",
    "Output validation failed: approved is missing",
  ],
  ["设置页加快捷键说明", "Add keyboard shortcuts to Settings"],
  ["通知改成可以静音", "Add a mute option for notifications"],
  ["README 加上 FAQ", "Add an FAQ to the README"],
  ["清理没用的依赖", "Remove unused dependencies"],
  ["错误提示统一文案", "Unify error messages"],
  ["列表加分页", "Add pagination to lists"],
  ["超时了（30m）", "Timed out (30m)"],
  ["上传加进度条", "Show upload progress"],
  ["修 Windows 路径问题", "Fix Windows paths"],
  ["做", "Create"],
  ["审", "Review"],
  ["改", "Revise"],
];
export function fixtureEnglish(v) {
  if (typeof v === "string") {
    for (const [a, b] of [...pairs].sort((a, b) => b[0].length - a[0].length))
      v = v.replaceAll(a, b);
    return v;
  }
  if (Array.isArray(v)) return v.map(fixtureEnglish);
  if (v && typeof v === "object")
    return Object.fromEntries(
      Object.entries(v).map(([k, x]) => [k, fixtureEnglish(x)]),
    );
  return v;
}
export async function localizeFixtures(context) {
  await context.route(
    /\/app\/hubs\/[^/]+\/(?:workflows|workflow-runs|automations)(?:[/?]|$)/,
    async (route) => {
      const response = await route.fetch();
      if (!(response.headers()["content-type"] || "").includes("json"))
        return route.fulfill({ response });
      return route.fulfill({
        response,
        json: fixtureEnglish(await response.json()),
      });
    },
  );
}
