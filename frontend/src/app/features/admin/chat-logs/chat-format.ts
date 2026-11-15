/** Helpers for the admin "Pesan" chat view. */

const escapeHtml = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;').replace(/'/g, '&#39;');

const URL_RE = /(https?:\/\/[^\s<>"']+[^\s<>"'.,;:!?)\]])/g;

/** Google Chat markup (*bold*, _italic_, ~strike~, `code`) on escaped text. */
function inline(text: string): string {
  return escapeHtml(text)
    .replace(/`([^`\n]+)`/g, '<code>$1</code>')
    .replace(/(^|[\s(])\*([^*\n]+)\*(?=$|[\s).,!?:;])/g, '$1<strong>$2</strong>')
    .replace(/(^|[\s(])_([^_\n]+)_(?=$|[\s).,!?:;])/g, '$1<em>$2</em>')
    .replace(/(^|[\s(])~([^~\n]+)~(?=$|[\s).,!?:;])/g, '$1<s>$2</s>');
}

/**
 * A message body as safe HTML: everything is escaped first, then Google Chat
 * markup and links are added. Newlines stay (rendered with pre-wrap).
 */
export function formatChatBody(body: string): string {
  let out = '';
  let last = 0;
  for (const m of body.matchAll(URL_RE)) {
    const i = m.index ?? 0;
    out += inline(body.slice(last, i));
    const url = escapeHtml(m[0]);
    out += `<a href="${url}" target="_blank" rel="noopener noreferrer">${url}</a>`;
    last = i + m[0].length;
  }
  return out + inline(body.slice(last));
}

const TZ = 'Asia/Jakarta';

/** WIB calendar day of an ISO time, as YYYY-MM-DD. */
export function wibDay(iso: string): string {
  return new Date(new Date(iso).getTime() + 7 * 3600_000).toISOString().slice(0, 10);
}

/** "Hari ini", "Kemarin" or "Sab, 15 Nov 2026" for a WIB day. */
export function dayLabel(day: string, today: string): string {
  if (day === today) return 'Hari ini';
  const y = new Date(Date.parse(today + 'T00:00:00Z') - 86400_000).toISOString().slice(0, 10);
  if (day === y) return 'Kemarin';
  return new Date(day + 'T00:00:00Z')
    .toLocaleDateString('id-ID', { timeZone: 'UTC', weekday: 'short', day: 'numeric', month: 'short', year: 'numeric' })
    .replace(/\./g, '');
}

/** "18.44" in WIB. */
export function clock(iso: string): string {
  return new Date(iso).toLocaleTimeString('id-ID', { timeZone: TZ, hour: '2-digit', minute: '2-digit' });
}

/** Short time for the user list: clock today, else "15 Nov". */
export function shortWhen(iso: string, today: string): string {
  if (wibDay(iso) === today) return clock(iso);
  return new Date(iso).toLocaleDateString('id-ID', { timeZone: TZ, day: 'numeric', month: 'short' }).replace('.', '');
}
