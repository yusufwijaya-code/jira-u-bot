import { Component, DestroyRef, ElementRef, OnInit, computed, inject, signal, viewChild } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Router, RouterLink } from '@angular/router';
import { AdminService, ChatConversation, ChatLogEntry, UserRow } from '../../../core/services/admin.service';
import { IconComponent } from '../../../shared/ui/icon.component';
import { ToastService } from '../../../core/ui/toast.service';
import { SelectComponent, SelectOption } from '../../../shared/ui/select.component';
import { DateRange, DateRangePickerComponent, todayISO } from '../../../shared/ui/date-range-picker.component';
import { UserAvatarComponent, adminUserAvatarUrl } from '../../../shared/ui/user-avatar.component';
import { clock, dayLabel, formatChatBody, shortWhen, wibDay } from './chat-format';

const PAGE_SIZE = 50;
/** How often the open chat checks for new messages. */
export const LIVE_INTERVAL_MS = 5000;
/** Bodies longer than this (chars or lines) start folded. */
const FOLD_CHARS = 600;
const FOLD_LINES = 12;

const KIND_LABEL: Record<ChatLogEntry['kind'], string> = {
  reminder: 'Reminder',
  message: 'Pesan',
  admin: 'Pesan admin',
  reply: 'Balasan',
  command: 'Command',
  click: 'Klik tombol',
  added: 'Tambah app',
  removed: 'Hapus app',
};

/** The "Jenis" filter: '' = all; otherwise a comma list of kinds. */
const KIND_FILTERS: SelectOption<string>[] = [
  { value: '', label: 'Semua jenis' },
  { value: 'reminder,message', label: 'Reminder & notifikasi' },
  { value: 'command,click', label: 'Command & klik dari user' },
  { value: 'reply', label: 'Balasan Nudge' },
  { value: 'admin', label: 'Pesan admin' },
  { value: 'added,removed', label: 'Aktivasi app' },
];

/** A user in the list: registered user and/or someone with messages. */
interface ChatUser {
  email: string;
  name: string;
  user: UserRow | null;
  conv: ChatConversation | null;
}

/** A message shown in the thread; pending ones are being sent by the admin. */
type Msg = ChatLogEntry & { pending?: 'sending' | 'failed' };

type Row = { type: 'day'; key: string; label: string } | { type: 'msg'; key: string; e: Msg; first: boolean };

let pendingSeq = 0;

/**
 * Admin › Pesan: a chat client over the Nudge message log. The left list
 * holds one conversation per user; the right pane shows the selected user's
 * DM with Nudge (Nudge on the right — the admin's side), live-updated, with
 * a composer to message that user.
 */
@Component({
  selector: 'app-chat-logs',
  standalone: true,
  imports: [FormsModule, RouterLink, IconComponent, SelectComponent, DateRangePickerComponent, UserAvatarComponent],
  templateUrl: './chat-logs.component.html',
})
export class ChatLogsComponent implements OnInit {
  private admin = inject(AdminService);
  private route = inject(ActivatedRoute);
  private router = inject(Router);
  private toast = inject(ToastService);

  private scroller = viewChild<ElementRef<HTMLElement>>('scroller');
  private composer = viewChild<ElementRef<HTMLTextAreaElement>>('composer');

  protected readonly kindFilters = KIND_FILTERS;
  protected readonly kindLabel = KIND_LABEL;
  protected readonly today = todayISO();
  protected readonly format = formatChatBody;
  protected readonly clock = clock;
  protected readonly avatarUrl = adminUserAvatarUrl;
  protected readonly maxMessage = 4000;

  private users = signal<UserRow[]>([]);
  private conversations = signal<ChatConversation[]>([]);
  protected listLoading = signal(true);

  protected email = signal('');
  protected from = signal('');
  protected to = signal('');
  protected kind = '';
  protected filtersOpen = signal(false);

  protected messages = signal<Msg[]>([]);
  protected hasOlder = signal(false);
  protected loading = signal(false);
  protected loadingOlder = signal(false);
  protected error = signal('');
  protected newBelow = signal(0);
  protected atBottom = signal(true);
  protected expanded = signal<Set<number>>(new Set());
  protected copied = signal(0);

  protected draft = signal('');
  protected search = signal('');
  /** Mobile: the list is shown instead of the chat. */
  protected showList = signal(true);

  /** Everyone selectable: users with messages first (newest), then the rest. */
  protected chatUsers = computed<ChatUser[]>(() => {
    const byEmail = new Map(this.users().map((u) => [u.email.toLowerCase(), u]));
    const seen = new Set<string>();
    const out: ChatUser[] = [];
    for (const c of this.conversations()) {
      const key = c.email.toLowerCase();
      const u = byEmail.get(key) ?? null;
      seen.add(key);
      out.push({ email: c.email, name: u?.userName ?? '', user: u, conv: c });
    }
    for (const u of this.users()) {
      if (!seen.has(u.email.toLowerCase())) out.push({ email: u.email, name: u.userName, user: u, conv: null });
    }
    return out;
  });

  protected filteredUsers = computed(() => {
    const q = this.search().trim().toLowerCase();
    return q ? this.chatUsers().filter((u) => u.email.toLowerCase().includes(q) || u.name.toLowerCase().includes(q)) : this.chatUsers();
  });

  protected selected = computed<ChatUser | null>(
    () => this.chatUsers().find((u) => u.email.toLowerCase() === this.email().toLowerCase()) ?? null,
  );

  protected activeFilters = computed(() => (this.from() ? 1 : 0) + (this.kind ? 1 : 0));

  /** Messages grouped under WIB day separators. */
  protected rows = computed<Row[]>(() => {
    const rows: Row[] = [];
    let day = '';
    let prev: Msg | null = null;
    for (const e of this.messages()) {
      const d = wibDay(e.createdDate);
      if (d !== day) {
        rows.push({ type: 'day', key: 'd' + d, label: dayLabel(d, this.today) });
        day = d;
        prev = null;
      }
      const first = !prev || prev.direction !== e.direction || isEvent(prev) || isEvent(e);
      rows.push({ type: 'msg', key: 'm' + e.id, e, first });
      prev = e;
    }
    return rows;
  });

  /** Live updates run unless the date filter ends before today. */
  protected live = computed(() => !!this.email() && (!this.to() || this.to() >= this.today) && (!this.from() || this.from() <= this.today));

  protected isEvent = isEvent;

  constructor() {
    const timer = setInterval(() => this.poll(), LIVE_INTERVAL_MS);
    inject(DestroyRef).onDestroy(() => clearInterval(timer));
  }

  ngOnInit(): void {
    const q = this.route.snapshot.queryParamMap;
    this.from.set(q.get('from') ?? '');
    this.to.set(q.get('to') ?? '');
    this.kind = q.get('kind') ?? '';
    if (this.from() || this.kind) this.filtersOpen.set(true);
    const wanted = q.get('email') ?? '';

    this.admin.listUsers().subscribe({ next: (res) => this.users.set(res.users ?? []), error: () => undefined });
    this.admin.chatConversations().subscribe({
      next: (res) => {
        this.conversations.set(res.conversations ?? []);
        this.listLoading.set(false);
        if (!this.email()) this.select(wanted || res.conversations?.[0]?.email || '', !!wanted);
      },
      error: () => {
        this.listLoading.set(false);
        if (!this.email() && wanted) this.select(wanted, true);
      },
    });
    if (wanted) this.select(wanted, true);
  }

  /** Opens one user's chat; openChat switches mobile to the chat view. */
  protected select(email: string, openChat = true): void {
    if (!email) return;
    if (openChat) this.showList.set(false);
    if (email.toLowerCase() === this.email().toLowerCase()) return;
    this.email.set(email);
    this.draft.set('');
    this.expanded.set(new Set());
    this.syncUrl();
    this.reload();
  }

  protected backToList(): void {
    this.showList.set(true);
  }

  protected setRange(r: DateRange): void {
    this.from.set(r.from);
    this.to.set(r.to || r.from);
    this.syncUrl();
    this.reload();
  }

  protected clearRange(): void {
    this.from.set('');
    this.to.set('');
    this.syncUrl();
    this.reload();
  }

  protected setKind(k: string): void {
    this.kind = k;
    this.syncUrl();
    this.reload();
  }

  protected clearFilters(): void {
    this.kind = '';
    this.clearRange();
  }

  private syncUrl(): void {
    this.router.navigate([], {
      relativeTo: this.route,
      queryParams: { email: this.email() || null, from: this.from() || null, to: this.to() || null, kind: this.kind || null },
      replaceUrl: true,
    });
  }

  private query(extra: { beforeID?: number; afterID?: number; limit?: number }) {
    return this.admin.chatLogs({
      email: this.email(),
      from: this.from(),
      to: this.to(),
      kind: this.kind,
      limit: PAGE_SIZE,
      ...extra,
    });
  }

  protected reload(): void {
    if (!this.email()) return;
    this.loading.set(true);
    this.error.set('');
    this.newBelow.set(0);
    this.query({}).subscribe({
      next: (page) => {
        this.messages.set([...(page.items ?? [])].reverse());
        this.hasOlder.set(!!page.nextBeforeID);
        this.loading.set(false);
        this.scrollToBottom();
      },
      error: (err) => {
        this.loading.set(false);
        this.messages.set([]);
        this.error.set(err?.status === 400 ? 'Filter tidak valid.' : 'Gagal memuat chat, coba lagi.');
      },
    });
  }

  protected loadOlder(): void {
    const oldest = this.messages().find((m) => m.id > 0);
    if (!oldest || this.loadingOlder()) return;
    const el = this.scroller()?.nativeElement;
    const before = el ? el.scrollHeight - el.scrollTop : 0;
    this.loadingOlder.set(true);
    this.query({ beforeID: oldest.id }).subscribe({
      next: (page) => {
        this.messages.set([...[...(page.items ?? [])].reverse(), ...this.messages()]);
        this.hasOlder.set(!!page.nextBeforeID);
        this.loadingOlder.set(false);
        // Keep the reader where they were.
        setTimeout(() => {
          if (el) el.scrollTop = el.scrollHeight - before;
        });
      },
      error: () => this.loadingOlder.set(false),
    });
  }

  /** Live: new messages in the open chat, and the list's previews. */
  protected poll(): void {
    if (typeof document !== 'undefined' && document.hidden) return;
    this.admin.chatConversations().subscribe({ next: (res) => this.conversations.set(res.conversations ?? []), error: () => undefined });
    if (!this.live() || this.loading()) return;
    this.fetchNew(false);
  }

  /** Appends messages newer than the last shown; stick=true scrolls down. */
  private fetchNew(stick: boolean): void {
    if (!this.email()) return;
    const real = this.messages().filter((m) => m.id > 0);
    const newest = real.length ? real[real.length - 1].id : 0;
    this.query({ afterID: newest || undefined, limit: 200 }).subscribe({
      next: (page) => {
        const known = new Set(this.messages().map((m) => m.id));
        const fresh = [...(page.items ?? [])].reverse().filter((e) => e.id > newest && !known.has(e.id));
        if (!fresh.length) return;
        const stay = stick || this.nearBottom();
        // A sent admin message replaces its pending bubble.
        const sentBodies = fresh.filter((e) => e.kind === 'admin').map((e) => e.body);
        const kept = this.messages().filter((m) => !(m.pending === 'sending' && sentBodies.includes(m.body)));
        this.messages.set([...kept.filter((m) => m.id > 0), ...fresh, ...kept.filter((m) => m.id < 0)]);
        if (stay) this.scrollToBottom();
        else this.newBelow.update((n) => n + fresh.length);
      },
      error: () => undefined,
    });
  }

  /** Sends the draft; the bubble shows at once while it goes out. */
  protected send(retry?: Msg): void {
    const text = (retry ? retry.body : this.draft()).trim();
    const to = this.email();
    if (!text || !to) return;
    if (retry) this.messages.update((list) => list.filter((m) => m.id !== retry.id));
    const pending: Msg = {
      id: -++pendingSeq,
      email: to,
      direction: 'out',
      kind: 'admin',
      channel: '',
      status: 'sent',
      body: text,
      createdDate: new Date().toISOString(),
      pending: 'sending',
    };
    this.messages.update((list) => [...list, pending]);
    if (!retry) this.draft.set('');
    this.scrollToBottom();
    this.focusComposer();

    this.admin.sendChatMessage(to, text).subscribe({
      next: (res) => {
        if (res.status === 'skipped') {
          this.toast.error('Pesan tidak terkirim', 'User ini belum bisa dihubungi (Nudge belum aktif dan tidak ada akun operator).');
        }
        this.fetchNew(true);
        // Drop the placeholder if the log entry is filtered out of view.
        setTimeout(() => this.messages.update((list) => list.filter((m) => m.id !== pending.id)), LIVE_INTERVAL_MS);
      },
      error: (err) => {
        this.messages.update((list) => list.map((m) => (m.id === pending.id ? { ...m, pending: 'failed', error: err?.error?.error || 'Pesan gagal dikirim' } : m)));
      },
    });
  }

  protected discard(m: Msg): void {
    this.messages.update((list) => list.filter((x) => x.id !== m.id));
  }

  /** Enter sends, Shift+Enter adds a line. */
  protected onComposerKey(event: KeyboardEvent): void {
    if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
      event.preventDefault();
      this.send();
    }
  }

  protected onScroll(): void {
    const bottom = this.nearBottom();
    this.atBottom.set(bottom);
    if (bottom && this.newBelow()) this.newBelow.set(0);
  }

  protected jumpToNew(): void {
    this.newBelow.set(0);
    this.scrollToBottom(true);
  }

  private nearBottom(): boolean {
    const el = this.scroller()?.nativeElement;
    return !el || el.scrollHeight - el.scrollTop - el.clientHeight < 80;
  }

  private scrollToBottom(smooth = false): void {
    setTimeout(() => {
      const el = this.scroller()?.nativeElement;
      if (!el) return;
      if (smooth && el.scrollTo) el.scrollTo({ top: el.scrollHeight, behavior: 'smooth' });
      else el.scrollTop = el.scrollHeight;
      this.atBottom.set(true);
    });
  }

  private focusComposer(): void {
    setTimeout(() => this.composer()?.nativeElement.focus());
  }

  protected isLong(e: Msg): boolean {
    return e.body.length > FOLD_CHARS || e.body.split('\n').length > FOLD_LINES;
  }

  protected toggleFold(id: number): void {
    const next = new Set(this.expanded());
    if (next.has(id)) next.delete(id);
    else next.add(id);
    this.expanded.set(next);
  }

  protected copy(e: Msg): void {
    navigator.clipboard?.writeText(e.body).then(
      () => {
        this.copied.set(e.id);
        setTimeout(() => this.copied() === e.id && this.copied.set(0), 1500);
      },
      () => this.toast.error('Gagal menyalin'),
    );
  }

  protected displayName(u: ChatUser | null): string {
    return u ? u.name || u.email : '';
  }

  protected userAvatar(u: ChatUser | null): string | null {
    return u?.user ? adminUserAvatarUrl(u.user.userId) : null;
  }

  protected when(iso: string): string {
    return shortWhen(iso, this.today);
  }

  /** Full date for a bubble's tooltip. */
  protected fullTime(iso: string): string {
    return new Date(iso).toLocaleString('id-ID', { timeZone: 'Asia/Jakarta', dateStyle: 'full', timeStyle: 'short' }) + ' WIB';
  }

  protected preview(c: ChatConversation): string {
    const who = c.lastDirection === 'in' ? '' : 'Nudge: ';
    // Plain text: drop the *bold* / _italic_ / ~strike~ markers of Chat markup.
    const body = (c.lastBody ?? '').replace(/[*_~`]/g, '').replace(/\s+/g, ' ').trim();
    return who + (body || KIND_LABEL[c.lastKind as ChatLogEntry['kind']] || '');
  }

  /** How Nudge's messages reach this user, for the chat header. */
  protected senderInfo(u: ChatUser | null): string {
    if (!u?.user) return '';
    return u.user.effectiveChatSender === 'nudge' ? 'Pesan lewat app Nudge' : 'Pesan lewat akun operator';
  }

  /** Who sent a Nudge message, as shown above its bubble. */
  protected senderLabel(e: Msg): string {
    switch (e.channel) {
      case 'operator':
        return 'via akun operator';
      case 'fallback':
        return 'via akun operator (cadangan)';
      case 'app':
        return 'App';
      default:
        return e.kind === 'admin' ? 'Admin' : 'App';
    }
  }

  protected eventText(e: Msg, name: string): string {
    return e.kind === 'added' ? `${name} menambahkan app Nudge` : `${name} menghapus app Nudge`;
  }
}

function isEvent(e: ChatLogEntry): boolean {
  return e.kind === 'added' || e.kind === 'removed';
}
