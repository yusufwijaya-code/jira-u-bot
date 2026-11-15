import {
  Component,
  DestroyRef,
  ElementRef,
  HostListener,
  OnInit,
  computed,
  inject,
  signal,
  viewChild,
} from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
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

/** A user in the picker: registered user and/or someone with messages. */
interface ChatUser {
  email: string;
  name: string;
  userId: number | null;
  conv: ChatConversation | null;
}

type Row = { type: 'day'; key: string; label: string } | { type: 'msg'; key: string; e: ChatLogEntry; first: boolean };

/**
 * Admin › Pesan as a chat clone: pick ONE user, read their DM with
 * Nudge as bubbles (Nudge left, the user right), live-updated every few
 * seconds.
 */
@Component({
  selector: 'app-chat-logs',
  standalone: true,
  imports: [FormsModule, IconComponent, SelectComponent, DateRangePickerComponent, UserAvatarComponent],
  templateUrl: './chat-logs.component.html',
})
export class ChatLogsComponent implements OnInit {
  private admin = inject(AdminService);
  private route = inject(ActivatedRoute);
  private router = inject(Router);
  private host = inject(ElementRef<HTMLElement>);
  private toast = inject(ToastService);

  private scroller = viewChild<ElementRef<HTMLElement>>('scroller');

  protected readonly kindFilters = KIND_FILTERS;
  protected readonly kindLabel = KIND_LABEL;
  protected readonly today = todayISO();
  protected readonly format = formatChatBody;
  protected readonly clock = clock;
  protected readonly avatarUrl = adminUserAvatarUrl;

  private users = signal<UserRow[]>([]);
  private conversations = signal<ChatConversation[]>([]);

  protected email = signal('');
  protected from = signal('');
  protected to = signal('');
  protected kind = '';

  protected messages = signal<ChatLogEntry[]>([]);
  protected hasOlder = signal(false);
  protected loading = signal(false);
  protected loadingOlder = signal(false);
  protected error = signal('');
  protected newBelow = signal(0);

  protected draft = signal('');
  protected sending = signal(false);
  protected readonly maxMessage = 4000;

  protected pickerOpen = signal(false);
  protected pickerQuery = signal('');

  /** Everyone selectable: users with messages first (newest), then the rest. */
  protected chatUsers = computed<ChatUser[]>(() => {
    const byEmail = new Map(this.users().map((u) => [u.email.toLowerCase(), u]));
    const seen = new Set<string>();
    const out: ChatUser[] = [];
    for (const c of this.conversations()) {
      const key = c.email.toLowerCase();
      const u = byEmail.get(key);
      seen.add(key);
      out.push({ email: c.email, name: u?.userName ?? '', userId: u?.userId ?? null, conv: c });
    }
    for (const u of this.users()) {
      if (!seen.has(u.email.toLowerCase())) out.push({ email: u.email, name: u.userName, userId: u.userId, conv: null });
    }
    return out;
  });

  protected filteredUsers = computed(() => {
    const q = this.pickerQuery().trim().toLowerCase();
    return q ? this.chatUsers().filter((u) => u.email.toLowerCase().includes(q) || u.name.toLowerCase().includes(q)) : this.chatUsers();
  });

  protected selected = computed<ChatUser | null>(
    () => this.chatUsers().find((u) => u.email.toLowerCase() === this.email().toLowerCase()) ?? null,
  );

  /** Messages grouped under WIB day separators. */
  protected rows = computed<Row[]>(() => {
    const rows: Row[] = [];
    let day = '';
    let prev: ChatLogEntry | null = null;
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

  constructor() {
    const timer = setInterval(() => this.poll(), LIVE_INTERVAL_MS);
    inject(DestroyRef).onDestroy(() => clearInterval(timer));
  }

  ngOnInit(): void {
    const q = this.route.snapshot.queryParamMap;
    this.from.set(q.get('from') ?? '');
    this.to.set(q.get('to') ?? '');
    this.kind = q.get('kind') ?? '';
    const wanted = q.get('email') ?? '';

    this.admin.listUsers().subscribe({ next: (res) => this.users.set(res.users ?? []), error: () => undefined });
    this.admin.chatConversations().subscribe({
      next: (res) => {
        this.conversations.set(res.conversations ?? []);
        if (!this.email()) this.select(wanted || res.conversations?.[0]?.email || '');
      },
      error: () => {
        if (!this.email() && wanted) this.select(wanted);
      },
    });
    if (wanted) this.select(wanted);
  }

  protected isEvent = isEvent;

  protected select(email: string): void {
    this.pickerOpen.set(false);
    this.pickerQuery.set('');
    if (!email || email.toLowerCase() === this.email().toLowerCase()) return;
    this.email.set(email);
    this.syncUrl();
    this.reload();
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
        this.error.set(err?.status === 400 ? 'Filter tidak valid.' : 'Gagal memuat riwayat pesan, coba lagi.');
      },
    });
  }

  protected loadOlder(): void {
    const oldest = this.messages()[0];
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

  /** Live: fetch messages newer than the last one shown. */
  protected poll(): void {
    if (!this.live() || this.loading() || (typeof document !== 'undefined' && document.hidden)) return;
    this.fetchNew(false);
    this.admin.chatConversations().subscribe({ next: (res) => this.conversations.set(res.conversations ?? []), error: () => undefined });
  }

  /** Appends messages newer than the last shown; stick=true scrolls down. */
  private fetchNew(stick: boolean): void {
    if (!this.email()) return;
    const msgs = this.messages();
    const newest = msgs.length ? msgs[msgs.length - 1].id : 0;
    this.query({ afterID: newest || undefined, limit: 200 }).subscribe({
      next: (page) => {
        const fresh = [...(page.items ?? [])].reverse().filter((e) => e.id > newest);
        if (!fresh.length) return;
        const atBottom = stick || this.nearBottom();
        this.messages.set([...this.messages(), ...fresh.filter((e) => !this.messages().some((m) => m.id === e.id))]);
        if (atBottom) this.scrollToBottom();
        else this.newBelow.update((n) => n + fresh.length);
      },
      error: () => undefined,
    });
  }

  /** Sends the draft to the selected user through their sender. */
  protected send(): void {
    const text = this.draft().trim();
    const to = this.email();
    if (!text || !to || this.sending()) return;
    this.sending.set(true);
    this.admin.sendChatMessage(to, text).subscribe({
      next: (res) => {
        this.sending.set(false);
        this.draft.set('');
        if (res.status === 'skipped') {
          this.toast.error('Pesan tidak terkirim', 'User ini belum bisa dihubungi (Nudge belum aktif dan tidak ada akun operator).');
        }
        this.fetchNew(true);
      },
      error: (err) => {
        this.sending.set(false);
        this.toast.error('Pesan gagal dikirim', err?.error?.error || 'Coba lagi sebentar lagi.');
        this.fetchNew(true);
      },
    });
  }

  /** Enter sends, Shift+Enter adds a line. */
  protected onComposerKey(event: KeyboardEvent): void {
    if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
      event.preventDefault();
      this.send();
    }
  }

  protected onScroll(): void {
    if (this.newBelow() && this.nearBottom()) this.newBelow.set(0);
  }

  protected jumpToNew(): void {
    this.newBelow.set(0);
    this.scrollToBottom();
  }

  private nearBottom(): boolean {
    const el = this.scroller()?.nativeElement;
    return !el || el.scrollHeight - el.scrollTop - el.clientHeight < 80;
  }

  private scrollToBottom(): void {
    setTimeout(() => {
      const el = this.scroller()?.nativeElement;
      if (el) el.scrollTop = el.scrollHeight;
    });
  }

  protected togglePicker(): void {
    this.pickerOpen.update((o) => !o);
    this.pickerQuery.set('');
  }

  @HostListener('document:pointerdown', ['$event'])
  protected onOutside(event: Event): void {
    const picker = this.host.nativeElement.querySelector('.chat-user-picker');
    if (this.pickerOpen() && picker && !(event.target instanceof Node && picker.contains(event.target))) {
      this.pickerOpen.set(false);
    }
  }

  @HostListener('document:keydown.escape')
  protected onEscape(): void {
    this.pickerOpen.set(false);
  }

  protected displayName(u: ChatUser | null): string {
    return u ? u.name || u.email : '';
  }

  protected when(iso: string): string {
    return shortWhen(iso, this.today);
  }

  protected preview(c: ChatConversation): string {
    const who = c.lastDirection === 'in' ? 'User: ' : 'Nudge: ';
    return who + (c.lastBody || KIND_LABEL[c.lastKind as ChatLogEntry['kind']] || '');
  }

  /** Who sent a Nudge message, as shown above its bubble. */
  protected senderLabel(e: ChatLogEntry): string {
    switch (e.channel) {
      case 'operator':
        return 'via akun operator';
      case 'fallback':
        return 'via akun operator (cadangan)';
      default:
        return 'App';
    }
  }

  protected eventText(e: ChatLogEntry, name: string): string {
    return e.kind === 'added' ? `${name} menambahkan app Nudge` : `${name} menghapus app Nudge`;
  }
}

function isEvent(e: ChatLogEntry): boolean {
  return e.kind === 'added' || e.kind === 'removed';
}
