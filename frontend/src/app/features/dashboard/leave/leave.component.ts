import { Component, computed, inject, signal } from '@angular/core';
import { AddLeaveResult, Leave, LeaveService } from '../../../core/services/leave.service';
import { ToastService } from '../../../core/ui/toast.service';
import { IconComponent } from '../../../shared/ui/icon.component';
import { DateRangePickerComponent } from '../../../shared/ui/date-range-picker.component';

const DAYS = ['Minggu', 'Senin', 'Selasa', 'Rabu', 'Kamis', 'Jumat', 'Sabtu'];
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'Mei', 'Jun', 'Jul', 'Agu', 'Sep', 'Okt', 'Nov', 'Des'];

/** Today in WIB as 2006-01-02. */
function todayWIB(): string {
  return new Date(Date.now() + 7 * 3600_000).toISOString().slice(0, 10);
}

/** "2026-11-18" → "Rabu, 18 Nov 2026" (no locale data needed). */
export function leaveLabel(date: string): string {
  const [y, m, d] = date.split('-').map(Number);
  const day = new Date(Date.UTC(y, m - 1, d)).getUTCDay();
  return `${DAYS[day]}, ${d} ${MONTHS[m - 1]} ${y}`;
}

/**
 * Cuti (Tech Spec "Papan Tiket & Cuti", US-2): full leave days. A leave day
 * stops the ticket clock and the reminders for this user only.
 */
@Component({
  selector: 'app-leave',
  standalone: true,
  imports: [IconComponent, DateRangePickerComponent],
  templateUrl: './leave.component.html',
})
export class LeaveComponent {
  private api = inject(LeaveService);
  private toast = inject(ToastService);

  protected leaves = signal<Leave[]>([]);
  protected loading = signal(true);
  protected saving = signal(false);
  protected confirming = signal<string | null>(null);

  protected from = signal(todayWIB());
  protected to = signal('');
  protected note = signal('');

  protected valid = computed(() => {
    const from = this.from();
    const to = this.to();
    return !!from && (!to || to >= from) && this.note().length <= 200;
  });

  protected today = todayWIB();
  /** The API accepts leave up to 60 days back. */
  protected minDate = new Date(Date.now() + 7 * 3600_000 - 60 * 86400_000).toISOString().slice(0, 10);

  protected label = leaveLabel;

  constructor() {
    this.load();
  }

  protected load(): void {
    this.loading.set(true);
    this.api.list().subscribe({
      next: (res) => {
        this.leaves.set(res.leaves ?? []);
        this.loading.set(false);
      },
      error: () => {
        this.loading.set(false);
        this.toast.error('Gagal memuat cuti', 'Coba muat ulang halaman.');
      },
    });
  }

  protected submit(): void {
    if (!this.valid() || this.saving()) return;
    this.saving.set(true);
    this.api.add(this.from(), this.to(), this.note().trim()).subscribe({
      next: (res) => {
        this.saving.set(false);
        this.toast.success('Cuti disimpan', this.summary(res));
        this.to.set('');
        this.note.set('');
        this.load();
      },
      error: (err) => {
        this.saving.set(false);
        this.toast.error('Gagal menyimpan cuti', err?.error?.error ?? 'Coba lagi.');
      },
    });
  }

  private summary(res: AddLeaveResult): string {
    const parts = [`${res.added.length} hari cuti disimpan`];
    if (res.skipped.length) parts.push(`${res.skipped.length} dilewati (akhir pekan/libur)`);
    if (res.recalculating) parts.push('waktu tiket sedang dihitung ulang dari Jira');
    return parts.join(', ') + '.';
  }

  protected remove(date: string): void {
    this.confirming.set(null);
    this.api.remove(date).subscribe({
      next: (res) => {
        this.toast.success('Cuti dihapus', res.recalculating ? 'Waktu tiket sedang dihitung ulang dari Jira.' : leaveLabel(date));
        this.load();
      },
      error: (err) => this.toast.error('Gagal menghapus cuti', err?.error?.error ?? 'Coba lagi.'),
    });
  }
}
