// A made-up computer for screenshots of apps that use myco.os: a Mac's disk
// with the usual suspects, scanned in a second (空间清理); a microphone, the
// Mac's sound and a meeting app to record, and speech to text (会议记录).
// shots.mjs puts it in the bridge for the apps that ask for it.
(() => {
  const G = 1e9, M = 1e6;
  // name: size in bytes (a file), or { … } (a folder); "*": smaller items folded together [count, bytes]
  const home = {
    Library: {
      Developer: { Xcode: { DerivedData: { "MycoApp-dkqhzt": 9.2 * G, "Sunrise-afkpwz": 6.4 * G, "Notes-bxmeor": 3.1 * G, "*": [42, 2.3 * G] }, "iOS DeviceSupport": 7.8 * G, Archives: 2.6 * G }, CoreSimulator: { Devices: 11.4 * G, Caches: 2.2 * G } },
      "Application Support": { Google: 8.7 * G, Code: 5.9 * G, Slack: 3.8 * G, Claude: 2.4 * G, "*": [212, 4.1 * G] },
      Caches: { "com.spotify.client": 3.2 * G, Google: 3.9 * G, Homebrew: 2.6 * G, "com.apple.Safari": 1.1 * G, "*": [388, 2.4 * G] },
      Containers: { "com.apple.mail": 6.1 * G, "com.tencent.xinWeChat": 9.6 * G, "*": [140, 3.3 * G] },
      "*": [96, 3.6 * G],
    },
    Movies: { "Final Cut 项目.fcpbundle": 28.4 * G, "发布会彩排.mov": 9.8 * G, "屏幕录制 2026-10-14.mov": 4.2 * G, "*": [31, 2.1 * G] },
    Pictures: { "Photos Library.photoslibrary": 41.3 * G, "*": [210, 1.9 * G] },
    Downloads: { "Xcode_16.1.xip": 7.9 * G, "ubuntu-24.04-desktop-arm64.iso": 3.1 * G, "Docker.dmg": 0.6 * G, "设计稿-终版.sketch": 1.4 * G, "*": [512, 6.2 * G] },
    code: { myco: { node_modules: 2.9 * G, ".git": 1.1 * G, web: 0.8 * G, "*": [48, 0.6 * G] }, sunrise: { node_modules: 3.4 * G, build: 2.2 * G, "*": [60, 0.7 * G] }, "*": [26, 6.8 * G] },
    Documents: { "合同扫描": 3.6 * G, "财务 2025": 2.1 * G, "*": [640, 9.4 * G] },
    Music: { "Music": 5.7 * G },
    ".docker": 6.3 * G, ".cache": 4.4 * G, "*": [74, 3.1 * G],
  };
  const disk = {
    Users: { glenn: home, Shared: 1.2 * G },
    Applications: { "Xcode.app": 12.1 * G, "Docker.app": 2.9 * G, "Final Cut Pro.app": 5.8 * G, "Microsoft Word.app": 2.4 * G, "Google Chrome.app": 1.6 * G, "*": [84, 9.7 * G] },
    opt: { homebrew: 8.9 * G },
    private: { var: { vm: 8.6 * G, folders: 5.4 * G, db: 1.2 * G, "*": [40, 0.9 * G] } },
    Library: { "Application Support": 6.3 * G, Developer: 4.1 * G, "*": [60, 3.2 * G] },
    "*": [22, 2.4 * G],
  };
  const ROOT = "/System/Volumes/Data";
  const largest = [];
  function build(name, path, spec) {
    if (typeof spec === "number") {
      const n = { path, name, dir: !/\.(xip|iso|dmg|sketch|mov)$/.test(name), size: spec, disk: spec, files: 1, modified: Date.now() - 86400000 * 40 };
      if (!n.dir) largest.push({ path, size: spec, disk: spec, modified: n.modified });
      else n.files = Math.round(spec / 3e6);
      return n;
    }
    const n = { path, name, dir: true, size: 0, disk: 0, files: 0, children: [] };
    for (const [k, v] of Object.entries(spec)) {
      if (k === "*") { n.more = v[0]; n.more_disk = n.more_size = v[1]; n.more_files = v[0] * 7; continue; }
      n.children.push(build(k, path + "/" + k, v));
    }
    n.children.sort((a, b) => b.disk - a.disk);
    n.disk = n.size = n.children.reduce((s, c) => s + c.disk, 0) + (n.more_disk || 0);
    n.files = n.children.reduce((s, c) => s + c.files, 0) + (n.more_files || 0);
    return n;
  }
  const tree = build("Macintosh HD", ROOT, disk);
  largest.sort((a, b) => b.size - a.size);
  const job = (value, steps) => {
    const subs = {};
    const emit = (t, d) => [...(subs[t] || []), ...(subs["*"] || [])].forEach((f) => f(d, { n: 0, type: t }));
    const done = new Promise((resolve) => {
      let i = 0;
      const tick = () => {
        i++;
        // (the root's folders finish one by one, the largest last)
        const kids = [...tree.children].reverse();
        const done = kids.slice(0, Math.floor(kids.length * i / steps)).map((k) => ({ name: k.name, path: k.path, dir: k.dir, disk: k.disk, files: k.files })).sort((a, b) => b.disk - a.disk);
        emit("progress", { files: Math.round(tree.files * i / steps), disk: tree.disk * i / steps, current: ROOT + "/Users/glenn/Library/Caches/com.spotify.client", top: done });
        if (i >= steps) resolve(value); else setTimeout(tick, window.__SCAN_MS__ || 120);
      };
      setTimeout(tick, 60);
    });
    return { on: (t, f) => { (subs[t] ||= []).push(f); return () => {}; }, done, cancel: async () => true, write: async () => true, end: async () => true };
  };
  const call = (v) => Promise.resolve(JSON.parse(JSON.stringify(v)));
  // a job that sends events on a timer; write("stop") ends it with value()
  const timed = (every, onTick, value, opts = {}) => {
    const subs = {};
    const emit = (t, d) => [...(subs[t] || []), ...(subs["*"] || [])].forEach((f) => f(d, { n: 0, type: t }));
    let resolve, timer, i = 0;
    const done = new Promise((r) => { resolve = r; });
    const finish = () => { clearInterval(timer); resolve(value(i)); };
    setTimeout(() => {
      if (opts.first) opts.first(emit);
      timer = setInterval(() => { i++; if (onTick(i, emit) === false) finish(); }, every);
    }, 30);
    return { on: (t, f) => { (subs[t] ||= []).push(f); return () => {}; }, done, cancel: async () => { finish(); return true; },
      write: async (d) => { if (d === "stop") finish(); if (d === "pause") emit("paused", {}); if (d === "resume") emit("resumed", {}); return true; }, end: async () => true };
  };
  const REC = "/Users/glenn/.myco/recordings/20261120-153000";
  const SAID = [
    [2.1, 7.8, "好，人都到齐了，我们开始吧。今天主要对一下 0.22 版的发布计划。"],
    [8.4, 15.2, "先说时间，原定下周三发，现在桌面端还有两个问题没收尾。"],
    [30.2, 36.9, "可以，这样不卡整体进度。那安装包里要不要带上转写模型？"],
  ];
  window.__FAKE_OS__ = {
    list: () => call({ os: "darwin", arch: "arm64", caps: ["system.info", "system.disks", "system.settings", "fs.scan", "fs.trash", "fs.reveal", "fs.pick", "fs.stat",
      "audio.sources", "audio.record", "speech.status", "speech.install", "speech.transcribe"].map((name) => ({ name, about: "", available: true, job: /scan|record|install|transcribe/.test(name) })) }),
    mediaURL: (p) => "/__media/" + encodeURIComponent(p),
    audio: {
      sources: () => call({ sources: [
        { id: "mic", kind: "mic", name: "麦克风", default: true },
        { id: "system", kind: "system", name: "电脑的声音" },
        { id: "app:com.tencent.meeting", kind: "app", name: "腾讯会议", bundle: "com.tencent.meeting", playing: true },
        { id: "app:us.zoom.xos", kind: "app", name: "zoom.us", bundle: "us.zoom.xos" },
        { id: "app:com.electron.lark", kind: "app", name: "Feishu", bundle: "com.electron.lark" },
        { id: "app:com.google.Chrome", kind: "app", name: "Google Chrome", bundle: "com.google.Chrome" },
      ] }),
      record: (a) => {
        const ids = (a.tracks || []).map((t) => t.id);
        const t0 = window.__REC_AT__ || 0;
        return timed(100, (i, emit) => {
          const s = t0 + i / 10;
          const tracks = {};
          // two people taking turns, with pauses
          for (const id of ids) {
            const talking = id === "them" ? Math.sin(s / 3.1) > -0.2 : Math.sin(s / 3.1) < -0.4;
            tracks[id] = { rms: 0, peak: talking ? 0.08 + 0.5 * Math.abs(Math.sin(s * 7.3 + (id === "me" ? 1 : 0)) * Math.sin(s * 2.1)) : 0.003 };
          }
          emit("level", { seconds: s, tracks });
        }, (i) => ({ dir: REC, seconds: t0 + i / 10, rate: 16000, ended: "stopped", tracks: ids.map((id) => ({ id, source: "", path: REC + "/" + id + ".wav", seconds: t0 + i / 10 })) }),
        { first: (emit) => emit("started", { dir: REC, files: Object.fromEntries(ids.map((id) => [id, REC + "/" + id + ".wav"])), rate: 16000 }) });
      },
    },
    speech: {
      status: () => call({ ready: !window.__NO_SPEECH__, engine: true, engine_version: "v2.0.2", vad: true, dir: "/Users/glenn/.myco/speech", engine_size_mb: window.__NO_SPEECH__ ? 25 : 0,
        models: [{ id: "sensevoice", name: "SenseVoice", languages: ["zh", "en"], size_mb: 158, installed: !window.__NO_SPEECH__ }] }),
      install: () => timed(200, (i, emit) => { emit("progress", { step: "model", received: i * 16e6, total: 165e6 }); return i < 10; }, () => ({ ready: true })),
      transcribe: () => timed(150, (i, emit) => {
        const s = SAID[i - 1];
        if (s) emit("segment", { start: s[0], end: s[1], text: s[2] });
        emit("progress", { done: i * 20, total: 60 });
        return i < SAID.length;
      }, () => ({ segments: [], seconds: 60, ms: 900, model: "sensevoice" })),
    },
    has: async () => true,
    system: {
      info: () => call({ os: "darwin", name: "macOS", version: "15.1", arch: "arm64", model: "Mac15,6", hostname: "glenn-mbp", user: "glenn", home: "/Users/glenn", cpus: 12, cpu_model: "Apple M3 Pro", memory: 36 * G, uptime: 400000 }),
      disks: () => call([
        { mount: "/", device: "/dev/disk3s1s1", fs: "apfs", label: "/", total: 494.4 * G, free: 70.1 * G, available: 61.8 * G, scan_path: ROOT },
        { mount: "/Volumes/备份", device: "/dev/disk5s2", fs: "apfs", label: "备份", total: 2000 * G, free: 812 * G, available: 812 * G, removable: true },
      ]),
      settings: () => call(true),
    },
    fs: {
      scan: () => job({ roots: [tree], largest: largest.slice(0, 200), files: tree.files, dirs: 182340, bytes: tree.size, disk: tree.disk, skipped: 37, skipped_paths: [], ms: 41200 }, 8),
      trash: (a) => call({ done: a.paths || [], failed: [] }),
      reveal: () => call(true),
      pick: () => Promise.reject(Object.assign(new Error("cancelled"), { code: "cancelled" })),
      stat: (a) => call({ name: a.path.split("/").pop(), path: a.path, type: "file", size: 63e6, modified: Date.now(), mode: "-rw-------" }),
    },
  };
})();
