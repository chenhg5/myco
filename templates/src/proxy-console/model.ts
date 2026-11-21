import type {
  OSProxyProfile,
  OSProxyNode,
  OSProxyConnection,
  OSProxySnapshot,
} from "../_lib/myco";
export type Profile = OSProxyProfile;
export type Node = OSProxyNode;
export type Connection = OSProxyConnection;
export type Snapshot = OSProxySnapshot;
export type Sample = { at: number; down: number; up: number };
export const bytes = (n: number) => {
  if (!Number.isFinite(n) || n < 0) return "—";
  if (n < 1024) return `${Math.round(n)} B`;
  const i = Math.min(4, Math.floor(Math.log(n) / Math.log(1024)));
  return `${(n / 1024 ** i).toFixed(i === 1 ? 0 : 1)} ${["B", "KB", "MB", "GB", "TB"][i]}`;
};
export function rate(
  previous: Snapshot | undefined,
  next: Snapshot,
): Sample | undefined {
  if (
    !previous ||
    next.warnings.some((w) => w.startsWith("/connections")) ||
    previous.warnings.some((w) => w.startsWith("/connections"))
  )
    return;
  const seconds = (Date.parse(next.at) - Date.parse(previous.at)) / 1000;
  const down = next.traffic.downloadTotal - previous.traffic.downloadTotal,
    up = next.traffic.uploadTotal - previous.traffic.uploadTotal;
  if (
    ![seconds, down, up].every(Number.isFinite) ||
    seconds <= 0 ||
    seconds > 30 ||
    down < 0 ||
    up < 0
  )
    return;
  return { at: Date.parse(next.at), down: down / seconds, up: up / seconds };
}
export const demoProfile: Profile = {
  endpoint: "http://127.0.0.1:9090",
  engine: "mihomo",
  has_secret: true,
};
export const demoSnapshot: Snapshot = {
  version: "示例内核",
  at: new Date().toISOString(),
  config: {
    mode: "rule",
    "mixed-port": 7890,
    port: 0,
    "socks-port": 0,
    "allow-lan": false,
    "log-level": "info",
    tun: { enable: false },
  },
  warnings: [],
  proxies: {
    自动选择: {
      name: "自动选择",
      type: "URLTest",
      now: "香港 · 01",
      all: ["香港 · 01", "日本 · 01", "新加坡 · 01"],
    },
    默认代理: {
      name: "默认代理",
      type: "Selector",
      now: "自动选择",
      all: ["自动选择", "香港 · 01", "日本 · 01", "新加坡 · 01", "DIRECT"],
    },
    流媒体: {
      name: "流媒体",
      type: "Selector",
      now: "日本 · 01",
      all: ["香港 · 01", "日本 · 01", "新加坡 · 01", "DIRECT"],
    },
    "香港 · 01": {
      name: "香港 · 01",
      type: "Shadowsocks",
      history: [{ delay: 32 }],
    },
    "日本 · 01": {
      name: "日本 · 01",
      type: "Trojan",
      history: [{ delay: 78 }],
    },
    "新加坡 · 01": {
      name: "新加坡 · 01",
      type: "Shadowsocks",
      history: [{ delay: 61 }],
    },
    DIRECT: { name: "DIRECT", type: "Direct" },
  },
  traffic: {
    downloadTotal: 2762143897,
    uploadTotal: 183771842,
    connections: [
      {
        id: "demo1",
        metadata: {
          host: "github.com",
          destinationIP: "140.82.113.4",
          destinationPort: "443",
          network: "tcp",
          process: "Safari",
        },
        upload: 185600,
        download: 7214882,
        start: new Date().toISOString(),
        chains: ["香港 · 01", "自动选择", "默认代理"],
        rule: "DomainSuffix",
      },
      {
        id: "demo2",
        metadata: {
          host: "api.figma.com",
          destinationIP: "",
          destinationPort: "443",
          network: "tcp",
          process: "Figma",
        },
        upload: 34272,
        download: 2891245,
        start: new Date().toISOString(),
        chains: ["日本 · 01", "流媒体"],
        rule: "DomainSuffix",
      },
      {
        id: "demo3",
        metadata: {
          host: "developer.apple.com",
          destinationIP: "",
          destinationPort: "443",
          network: "tcp",
          process: "Safari",
        },
        upload: 8273,
        download: 853289,
        start: new Date().toISOString(),
        chains: ["DIRECT"],
        rule: "GeoIP",
      },
    ],
  },
};
export const demoSamples: Sample[] = Array.from({ length: 45 }, (_, i) => ({
  at: Date.now() - (44 - i) * 2000,
  down:
    (0.35 +
      Math.abs(Math.sin(i * 0.31)) * 1.6 +
      Math.abs(Math.cos(i * 0.79)) * 0.5) *
    1024 ** 2,
  up: (0.06 + Math.abs(Math.sin(i * 0.41)) * 0.14) * 1024 ** 2,
}));
