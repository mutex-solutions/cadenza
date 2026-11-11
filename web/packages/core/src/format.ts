/** Plain words for thread statuses. */
export function statusLabel(status: string): string {
  switch (status) {
    case "active":
      return "Working";
    case "changes_requested":
      return "Changes requested";
    case "ready":
      return "Ready to review";
    case "approved":
      return "Approved";
    case "archived":
      return "Archived";
    default:
      return status;
  }
}

/** Display names for agent ids the server sends. */
export function agentLabel(agent: string): string {
  switch (agent) {
    case "claude-code":
    case "claude":
      return "Claude Code";
    case "codex":
      return "Codex";
    case "gemini":
      return "Gemini";
    case "hermes":
      return "Hermes";
    case "cursor":
      return "Cursor";
    default:
      return agent || "Agent";
  }
}

/** Short relative time, e.g. "now", "5 min", "3 h", "2 d", or a date. */
export function timeAgo(ms: number, now: number = Date.now()): string {
  const s = Math.max(0, Math.round((now - ms) / 1000));
  if (s < 45) return "now";
  const m = Math.round(s / 60);
  if (m < 60) return `${m} min`;
  const h = Math.round(m / 60);
  if (h < 24) return `${h} h`;
  const d = Math.round(h / 24);
  if (d < 7) return `${d} d`;
  return new Date(ms).toISOString().slice(0, 10);
}

/** Human file size, e.g. "1.2 MB". */
export function fileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB"];
  let v = bytes / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v < 10 ? v.toFixed(1) : Math.round(v)} ${units[i]}`;
}

/** Video time as m:ss. */
export function clock(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000));
  return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, "0")}`;
}

/**
 * How a post is named in pickers: "Round 2 · Layout". Agents often title
 * posts "Round 2: layout" themselves; then the title is used as is.
 */
export function postLabel(seq: number, title: string): string {
  const t = title.trim();
  if (/^round\b/i.test(t)) return t;
  return t ? `Round ${seq} · ${t}` : `Round ${seq}`;
}
