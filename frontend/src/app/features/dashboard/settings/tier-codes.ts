/**
 * Tier code helpers, mirroring the Go rules in user_setting_dto: a code is
 * "<n>m" or "<n>h", 1 minute to 8 hours, whole hours rendered as "<n>h".
 */
export const MAX_TIERS = 10;
export const MIN_TIER_MINUTES = 1;
export const MAX_TIER_MINUTES = 8 * 60;

export function tierMinutes(code: string): number {
  const match = /^(\d{1,4})([mh])$/.exec(code.trim().toLowerCase());
  if (!match) return 0;
  const n = Number(match[1]);
  return match[2] === 'h' ? n * 60 : n;
}

export function tierCode(minutes: number): string {
  return minutes % 60 === 0 ? `${minutes / 60}h` : `${minutes}m`;
}

/** "45m" → "45 menit", "2h" → "2 jam", "90m" → "1,5 jam". */
export function tierDuration(code: string): string {
  const minutes = tierMinutes(code);
  if (minutes < 60 || minutes % 30 !== 0) return `${minutes} menit`;
  const hours = minutes / 60;
  return `${Number.isInteger(hours) ? hours : hours.toString().replace('.', ',')} jam`;
}

export function sortTiers(codes: string[], order: 'desc' | 'asc'): string[] {
  const sorted = [...codes].sort((a, b) => tierMinutes(b) - tierMinutes(a));
  return order === 'desc' ? sorted : sorted.reverse();
}

/** Validates a custom tier entry; returns an error message or null. */
export function validateCustomTier(minutes: number, selected: string[]): string | null {
  if (!Number.isInteger(minutes) || minutes < MIN_TIER_MINUTES || minutes > MAX_TIER_MINUTES) {
    return 'Tier harus antara 1 menit sampai 8 jam';
  }
  if (selected.some((code) => tierMinutes(code) === minutes)) {
    return 'Tier ini sudah ada';
  }
  if (selected.length >= MAX_TIERS) {
    return `Maksimal ${MAX_TIERS} tier`;
  }
  return null;
}
