// Preserve system-following CSS for older hosts, and add an explicit App
// theme path. This also works when a webview forces the OS media preference.
export default function appThemeCSS() {
  const prefix = (selector, guard) =>
    selector.includes(":root")
      ? selector.replace(":root", `:root${guard}`)
      : /^html\b/.test(selector)
        ? selector.replace(/^html/, `html${guard}`)
        : `:root${guard} ${selector}`;
  function scope(container, guard) {
    container.walkRules((rule) => {
      if (rule.parent?.type === "atrule" && /keyframes$/.test(rule.parent.name))
        return;
      rule.selectors = rule.selectors.map((s) => prefix(s, guard));
    });
  }
  return {
    postcssPlugin: "myco-app-theme",
    Once(root) {
      const queries = [];
      root.walkAtRules("media", (rule) => {
        if (/\(prefers-color-scheme:\s*(dark|light)\)/.test(rule.params))
          queries.push(rule);
      });
      for (const rule of queries) {
        const match = /\(prefers-color-scheme:\s*(dark|light)\)/.exec(
            rule.params,
          ),
          theme = match[1];
        const explicit = rule.clone();
        scope(explicit, `[data-myco-theme="${theme}"]`);
        explicit.params = explicit.params.replace(match[0], "(min-width: 0px)");
        scope(rule, ":not([data-myco-theme])");
        rule.after(explicit);
      }
    },
  };
}
