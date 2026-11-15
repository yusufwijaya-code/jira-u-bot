import { Subscription } from 'rxjs';
import { Component, effect, inject, input, output, signal, untracked } from '@angular/core';
import { RouterLink } from '@angular/router';
import { BoardContextService } from '../../../../core/services/board-context.service';
import { CreateMeta, CreateStatus, IssueCreateService } from '../../../../core/services/issue-create.service';
import { SingleCardFormComponent } from './single-card-form.component';
import { BulkSubtasksComponent } from './bulk-subtasks.component';

/** Tab "Buat Card" (spec US-4..US-7). */
@Component({
  selector: 'app-create-card',
  standalone: true,
  imports: [RouterLink, SingleCardFormComponent, BulkSubtasksComponent],
  templateUrl: './create-card.component.html',
  styleUrl: './create-card.component.scss',
})
export class CreateCardComponent {
  private api = inject(IssueCreateService);
  protected boards = inject(BoardContextService).boards;
  boardId = input<number | null>(null);
  initialMode = input<'single' | 'bulk'>('single');
  initialParent = input<string | null>(null);
  created = output<void>();

  protected localBoard = signal<number | null>(null);
  protected board = signal<number | null>(null);
  protected mode = signal<'single' | 'bulk'>('single');
  protected status = signal<CreateStatus | null>(null);
  protected meta = signal<CreateMeta | null>(null);
  protected statusError = signal(false);
  private metaSub?: Subscription;
  protected metaError = signal<string | null>(null);

  constructor() {
    this.api.status().subscribe({ next: (s) => this.status.set(s), error: () => this.statusError.set(true) });
    effect(() => {
      const id = this.boardId() ?? this.localBoard();
      untracked(() => { this.mode.set(this.initialMode()); this.loadMeta(id); });
    });
  }

  protected loadMeta(id: number | null): void {
    this.metaSub?.unsubscribe();
    this.board.set(id);
    this.meta.set(null);
    this.metaError.set(null);
    if (!id) return;
    this.metaSub = this.api.meta(id).subscribe({
      next: (m) => this.meta.set(m),
      error: (err) => this.metaError.set(err?.status === 412 ? null : err?.error?.error ?? 'Gagal memuat tipe card dari Jira'),
    });
  }
}
