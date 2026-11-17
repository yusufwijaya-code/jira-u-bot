import { Component, computed, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { HttpErrorResponse } from '@angular/common/http';
import { TrackingRule, TrackingRuleError, TrackingRuleService } from '../../../core/services/tracking-rule.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';
import { BoardAvatarComponent } from '../../../shared/ui/board-avatar.component';
import { recalculateAction } from './recalculate-action';

/**
 * Admin › Aturan Board (Tech Spec "Aturan Board (Admin CRUD)"): every Jira
 * project Nudge tracks — which statuses count, when a ticket is finished and
 * where its Story Points live. "Hapus" = nonaktifkan.
 */
@Component({
  selector: 'app-tracking-rules',
  standalone: true,
  imports: [RouterLink, IconComponent, BoardAvatarComponent],
  templateUrl: './tracking-rules.component.html',
  styleUrl: './tracking-rules.component.scss',
})
export class TrackingRulesComponent {
  private api = inject(TrackingRuleService);
  private confirm = inject(ConfirmService);
  private toast = inject(ToastService);

  protected rules = signal<TrackingRule[]>([]);
  protected loading = signal(true);
  protected error = signal<string | null>(null);
  protected busyKey = signal<string | null>(null);
  protected activeCount = computed(() => this.rules().filter((r) => r.isActive).length);

  constructor() {
    this.load();
  }

  protected load(): void {
    this.loading.set(true);
    this.error.set(null);
    this.api.list().subscribe({
      next: (res) => {
        this.rules.set(res.rules ?? []);
        this.loading.set(false);
      },
      error: () => {
        this.error.set('Gagal memuat aturan board, coba lagi');
        this.loading.set(false);
      },
    });
  }

  protected formatEdited(iso: string): string {
    if (!iso) return '—';
    return new Date(iso).toLocaleString('id-ID', {
      timeZone: 'Asia/Jakarta', day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit',
    });
  }

  protected async deactivate(rule: TrackingRule): Promise<void> {
    const confirmed = await this.confirm.ask({
      title: `Nonaktifkan ${rule.projectKey}?`,
      message: `Tiket ${rule.projectKey} berhenti dipantau. Riwayat tetap tersimpan.`,
      confirmText: 'Nonaktifkan',
      tone: 'danger',
      icon: 'power',
    });
    if (confirmed) this.setActive(rule, false);
  }

  protected activate(rule: TrackingRule): void {
    this.setActive(rule, true);
  }

  private setActive(rule: TrackingRule, active: boolean): void {
    this.busyKey.set(rule.projectKey);
    this.api.setActive(rule.projectKey, active).subscribe({
      next: (saved) => {
        this.busyKey.set(null);
        this.rules.update((list) => list.map((r) => (r.projectKey === saved.projectKey ? saved : r)));
        this.toast.show(
          'success',
          active ? `${saved.projectKey} dipantau lagi` : `${saved.projectKey} dinonaktifkan`,
          active
            ? 'Tiket yang masih di status dihitung dibuka lagi pada cycle poller berikutnya.'
            : 'Sesi yang sedang berjalan ditutup tanpa menambah waktu.',
          10000,
          recalculateAction(this.api, this.toast),
        );
      },
      error: (err: HttpErrorResponse) => {
        this.busyKey.set(null);
        if (err?.status === 422) {
          this.toast.error('Minimal satu board harus aktif', 'Aktifkan board lain dulu sebelum menonaktifkan yang ini.');
          return;
        }
        this.toast.error('Gagal menyimpan', (err?.error as TrackingRuleError | undefined)?.message ?? 'Coba lagi sebentar.');
      },
    });
  }
}
