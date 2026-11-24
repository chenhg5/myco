// Synthetic bridge for UI regression and catalog screenshots only. No SSH is executed.
export const installSSHFixture = ({ lang, theme }) => {
  const p = {
    id: "ssh_fixture",
    name: "Fixture Linux",
    destination: "fixture",
    host: "fixture.test",
    user: "root",
    port: 2222,
    binding: "a".repeat(64),
    agent_enabled: false,
  };
  window.calls = [];
  window.nativeProfiles = [];
  const result = (stdout) => ({
    stdout,
    stderr: "",
    exit_code: 0,
    truncated: false,
    duration_ms: 20,
  });
  const overview =
    '@@myco:os\nLinux 6.8 x86_64\nPRETTY_NAME="Ubuntu fixture"\n@@myco:uptime\n86400 20\n0.1 0.2 0.3 1/20 22\n@@myco:cpu0\ncpu 100 0 50 800 20 0 0 0\n@@myco:cpu1\ncpu 110 0 50 890 20 0 0 0\n@@myco:memory\nMemTotal: 8000000 kB\nMemAvailable: 5000000 kB\nSwapTotal: 0 kB\nSwapFree: 0 kB\n@@myco:disks\nFilesystem 1024-blocks Used Available Capacity Mounted on\n/dev/fixture 1000000 300000 700000 30% /\n';
  const bridge = {
    features: ["os", "ssh"],
    preferences: { language: lang, theme },
    ready: Promise.resolve({
      page: { id: "pg_fixture" },
      me: { name: "owner" },
    }),
    data: { list: async () => ({ records: [], seq: 0 }), on: () => () => {} },
    os: {
      has: async () => true,
      call: async (name, a) => {
        window.calls.push({ name, args: structuredClone(a) });
        if (name === "ssh.profiles") {
          if (a.op === "list") return { profiles: window.nativeProfiles };
          if (a.op === "create") {
            if (a.destination !== "fixture") throw Error("wrong target");
            window.nativeProfiles = [p];
            return p;
          }
          if (a.op === "agent_enabled") {
            p.agent_enabled = a.agent_enabled;
            p.agent_hub = "https://workspace.test";
            return p;
          }
          if (a.op === "delete") {
            window.nativeProfiles = [];
            return { ok: true };
          }
        }
        if (
          name !== "ssh.run" ||
          a.profile_id !== p.id ||
          a.binding !== p.binding
        )
          throw Error("missing target binding");
        if (a.action === "overview") return result(overview);
        if (a.action === "services.list")
          return result("nginx.service loaded active running Web server\n");
        if (a.action === "containers.list")
          return window.dockerDenied
            ? {
                ...result(""),
                exit_code: 1,
                stderr: "Docker permission denied",
              }
            : result(
                JSON.stringify({
                  ID: "abcdef123456",
                  Names: "web",
                  Image: "nginx:alpine",
                  State: "running",
                  Status: "Up",
                  Ports: "80/tcp",
                }),
              );
        if (a.action === "services.restart") {
          if (window.failMutation)
            throw { code: "outcome_unknown", message: "lost response" };
          return result("");
        }
        if (a.action.endsWith(".logs"))
          return result(
            '<img src=x onerror="window.injected=true">\nfixture log',
          );
        throw Error("unexpected action");
      },
    },
  };
  Object.defineProperty(window, "myco", {
    configurable: true,
    get: () => bridge,
    set: () => {},
  });
};
