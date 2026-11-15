import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { AdminService, ChatLogEntry, UserRow } from '../../../core/services/admin.service';
import { IconComponent } from '../../../shared/ui/icon.component';

const PAGE_SIZE = 50;

const KIND_LABEL: Record<ChatLogEntry['kind'], string> = {
  reminder: 'Reminder',
  message: 'Pesan',
  reply: 'Balasan',
  command: 'Command',
  click: 'Klik tombol',
  added: 'Tambah app',
  removed: 'Hapus app',
};

const CHANNEL_LABEL: Record<string, string> = {
  app: 'Nudge app',
  operator: 'Akun operator',
  fallback: 'Cadangan (operator)',
};

const STATUS_LABEL: Record<ChatLogEntry['status'], string> = {
  received: 'Diterima',
  sent: 'Terkirim',
  failed: 'Gagal',
  skipped: 'Dilewati',
};

/** Admin › Riwayat pesan: what Nudge sent and what users sent it. */
@Component({
  selector: 'app-chat-logs',
  standalone: true,
  imports: [FormsModule, IconComponent],
  templateUrl: './chat-logs.component.html',
})
export class ChatLogsComponent implements OnInit {
  private admin = inject(AdminService);
  private route = inject(ActivatedRoute);
  private router = inject(Router);

  protected users = signal<UserRow[]>([]);
  protected items = signal<ChatLogEntry[]>([]);
  protected nextBeforeID = signal(0);
  protected loading = signal(false);
  protected error = signal('');
  protected expanded = signal<Set<number>>(new Set());

  protected email = '';
  protected from = '';
  protected to = '';

  protected readonly kindLabel = KIND_LABEL;
  protected readonly statusLabel = STATUS_LABEL;

  ngOnInit(): void {
    const q = this.route.snapshot.queryParamMap;
    this.email = q.get('email') ?? '';
    this.from = q.get('from') ?? '';
    this.to = q.get('to') ?? '';
    this.admin.listUsers().subscribe({ next: (res) => this.users.set(res.users ?? []), error: () => undefined });
    this.load(false);
  }

  /** Applies the filters (kept in the URL so the view can be shared). */
  protected apply(): void {
    this.router.navigate([], {
      relativeTo: this.route,
      queryParams: { email: this.email || null, from: this.from || null, to: this.to || null },
      replaceUrl: true,
    });
    this.load(false);
  }

  protected reset(): void {
    this.email = this.from = this.to = '';
    this.apply();
  }

  protected load(more: boolean): void {
    this.loading.set(true);
    this.error.set('');
    this.admin
      .chatLogs({
        email: this.email,
        from: this.from,
        to: this.to,
        beforeID: more ? this.nextBeforeID() : undefined,
        limit: PAGE_SIZE,
      })
      .subscribe({
        next: (page) => {
          this.items.set(more ? [...this.items(), ...(page.items ?? [])] : (page.items ?? []));
          this.nextBeforeID.set(page.nextBeforeID ?? 0);
          this.loading.set(false);
        },
        error: (err) => {
          this.loading.set(false);
          this.error.set(err?.status === 400 ? 'Filter tanggal tidak valid.' : 'Gagal memuat riwayat pesan, coba lagi.');
        },
      });
  }

  protected toggle(id: number): void {
    const next = new Set(this.expanded());
    if (next.has(id)) next.delete(id);
    else next.add(id);
    this.expanded.set(next);
  }

  protected isLong(e: ChatLogEntry): boolean {
    return e.body.length > 180 || e.body.split('\n').length > 3;
  }

  protected channelLabel(c: string): string {
    return CHANNEL_LABEL[c] ?? '—';
  }

  protected userName(email: string): string {
    return this.users().find((u) => u.email.toLowerCase() === email.toLowerCase())?.userName ?? '';
  }

  /** "15 Nov 2026 18.44" in WIB. */
  protected time(iso: string): string {
    const d = new Date(iso);
    const tz = 'Asia/Jakarta';
    const date = d.toLocaleDateString('id-ID', { timeZone: tz, day: '2-digit', month: 'short', year: 'numeric' }).replace('.', '');
    const clock = d.toLocaleTimeString('id-ID', { timeZone: tz, hour: '2-digit', minute: '2-digit' });
    return `${date} ${clock}`;
  }
}
