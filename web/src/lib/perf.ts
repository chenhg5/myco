/** Owner-only GET /v1/admin/perf. Pool counters are cumulative since started_at. */
export interface DatabasePoolStats {
  at: string;
  started_at: string;
  max_open_connections: number;
  open_connections: number;
  in_use: number;
  idle: number;
  wait_count: number;
  wait_duration_ms: number;
}
export interface PerfStat {
  name: string;
  count: number;
  p50_ms: number;
  p95_ms: number;
  max_ms: number;
  total_ms: number;
  avg_rows?: number;
  max_rows?: number;
}
export interface PerfReport {
  minutes: number;
  requests: PerfStat[];
  statements: PerfStat[];
  /** Current workspace's primary pool; absent on older Hubs. */
  database?: DatabasePoolStats;
}
