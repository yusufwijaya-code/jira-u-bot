// Pure display helpers shared by the list and column views.

/** Colour family of a status dot, from its name (Jira workflows differ per project). */
export type StatusTone = 'todo' | 'process' | 'review' | 'done' | 'other';

export function statusTone(status: string): StatusTone {
  const s = status.toLowerCase();
  if (/(complete|done|selesai|closed|resolved)/.test(s)) return 'done';
  if (/review/.test(s)) return 'review';
  if (/(process|progress|develop)/.test(s)) return 'process';
  if (/(to do|todo|backlog|open)/.test(s)) return 'todo';
  return 'other';
}

/** 5400 → "1j 30m", 2700 → "45m", 7200 → "2j", 20 → "0m". */
export function shortDuration(totalSeconds: number): string {
  const minutes = Math.floor(Math.abs(totalSeconds) / 60);
  const h = Math.floor(minutes / 60);
  const m = minutes % 60;
  if (h === 0) return `${m}m`;
  return m === 0 ? `${h}j` : `${h}j ${m}m`;
}
