import { Subscription } from 'rxjs';
import { Component, computed, effect, inject, input, output, signal, untracked, viewChild } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { SelectComponent, SelectOption } from '../../../../shared/ui/select.component';
import { ToastService } from '../../../../core/ui/toast.service';
import { ConfirmService } from '../../../../core/ui/confirm.service';
import { RouterLink } from '@angular/router';
import { BoardContextService } from '../../../../core/services/board-context.service';
import { CreateMeta, CreateStatus, IssueCreateService } from '../../../../core/services/issue-create.service';
import { SingleCardFormComponent } from './single-card-form.component';
import { BulkSubtasksComponent } from './bulk-subtasks.component';

/** Tab "Buat Card" (spec US-4..US-7). */
@Component({
  selector: 'app-create-card',
  standalone: true,
  imports: [RouterLink, FormsModule, SelectComponent, SingleCardFormComponent, BulkSubtasksComponent],
  templateUrl: './create-card.component.html',
  styleUrl: './create-card.component.scss',
})
export class CreateCardComponent {
  private api = inject(IssueCreateService);
  private toast = inject(ToastService);
  private confirm = inject(ConfirmService);
  protected boards = inject(BoardContextService).boards;
  boardId = input<number | null>(null);
  initialMode = input<'single' | 'bulk'>('single');
  initialParent = input<string | null>(null);
  created = output<void>();

  protected localBoard = signal<number | null>(null);
  protected boardOptions = computed<SelectOption<number | null>[]>(() => [
    { value: null, label: 'Pilih board…' },
    ...this.boards().map((b) => ({ value: b.boardId, label: b.name })),
  ]);
  protected board = signal<number | null>(null);
  protected mode = signal<'single' | 'bulk'>('single');
  protected status = signal<CreateStatus | null>(null);
  protected meta = signal<CreateMeta | null>(null);
  protected statusError = signal(false);
  private bulk = viewChild(BulkSubtasksComponent);
  private metaSub?: Subscription;
  protected metaError = signal<string | null>(null);

  constructor() {
    this.api.status().subscribe({ next: (s) => this.status.set(s), error: () => this.statusError.set(true) });
    effect(() => {
      const id = this.boardId() ?? this.localBoard();
      untracked(() => {
        if (this.board() !== id && this.hasUnsent()) this.toast.info('Preview dibuang karena board diganti');
        this.mode.set(this.initialMode());
        this.loadMeta(id);
      });
    });
  }

  protected async setMode(m: 'single' | 'bulk'): Promise<void> {
    if (m === this.mode()) return;
    if (this.hasUnsent() && !(await this.confirm.ask({ title: 'Buang preview?', message: 'Preview subtask yang belum dibuat akan hilang.', confirmText: 'Ya, buang', tone: 'danger' }))) return;
    this.mode.set(m);
  }

  hasUnsent(): boolean {
    return this.mode() === 'bulk' && !!this.bulk()?.hasUnsent();
  }

  protected loadMeta(id: number | null): void {
    this.metaSub?.unsubscribe();
    this.board.set(id);
    this.meta.set(null);
    this.metaError.set(null);
    if (!id) return;
    this.metaSub = this.api.meta(id).subscribe({
      next: (m) => this.meta.set(m),
      error: (err) => {
        if (err?.status === 412) this.status.update((s) => (s ? { ...s, tokenReady: false } : s));
        else this.metaError.set(err?.error?.error ?? 'Gagal memuat tipe card dari Jira');
      },
    });
  }
}
