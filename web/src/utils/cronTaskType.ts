export type CronTaskType = "ping" | "tcp_ping" | "http";

interface TaskTypeStyle {
  /** Short label used in compact chips */
  short: string;
  /** Full label used in tables and selectors */
  label: string;
  bgcolor: string;
  color: string;
  border: string;
}

const TASK_TYPE_STYLES: Record<CronTaskType, TaskTypeStyle> = {
  ping: {
    short: "ICMP",
    label: "ICMP Ping",
    bgcolor: "#EFF6FF",
    color: "#2563EB",
    border: "#BFDBFE",
  },
  tcp_ping: {
    short: "TCP",
    label: "TCP Ping",
    bgcolor: "#F5F3FF",
    color: "#7C3AED",
    border: "#DDD6FE",
  },
  http: {
    short: "HTTP",
    label: "HTTP(S) Request",
    bgcolor: "#ECFDF5",
    color: "#059669",
    border: "#A7F3D0",
  },
};

export const getCronTaskTypeStyle = (taskType: string | undefined): TaskTypeStyle =>
  TASK_TYPE_STYLES[(taskType as CronTaskType) ?? "ping"] ?? TASK_TYPE_STYLES.ping;
