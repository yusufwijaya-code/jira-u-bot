import { ChangeDetectionStrategy, Component, computed, inject, input, signal } from '@angular/core';
import { BoardContextService } from '../../core/services/board-context.service';
import { IconComponent } from './icon.component';

const FALLBACK_COLORS = ['#5b4cf0', '#ff9f1c', '#12a150', '#3b82f6', '#e5484d', '#0ea5e9'];

/**
 * A board's Jira project avatar (served by the backend proxy). Falls back to
 * a colored tile with the project initial if the image can't be loaded;
 * `boardId` null renders the "Semua board" grid icon.
 */
@Component({
  selector: 'app-board-avatar',
  standalone: true,
  imports: [IconComponent],
  changeDetection: ChangeDetectionStrategy.OnPush,
  host: { class: 'board-avatar', '[style.--size.px]': 'size()' },
  template: `
    @if (boardId() === null) {
      <span class="board-avatar-all"><app-icon name="layers" [size]="size() * 0.6" /></span>
    } @else if (!failed()) {
      <img [src]="src()" alt="" [attr.width]="size()" [attr.height]="size()" (error)="failedFor.set(boardId())" />
    } @else {
      <span class="board-avatar-fallback" [style.background]="color()">{{ initial() }}</span>
    }
  `,
})
export class BoardAvatarComponent {
  boardId = input<number | null>(null);
  projectKey = input<string>('');
  size = input<number>(24);

  /** The board whose image failed — reset automatically when boardId changes. */
  protected failedFor = signal<number | null>(null);
  protected failed = computed(() => this.failedFor() !== null && this.failedFor() === this.boardId());
  private context = inject(BoardContextService);

  protected src = computed(() => {
    const id = this.boardId();
    return id === null ? '' : this.context.avatarUrl(id);
  });
  protected initial = computed(() => (this.projectKey() || '?').charAt(0).toUpperCase());
  protected color = computed(() => {
    const key = this.projectKey() || String(this.boardId() ?? '');
    const hash = [...key].reduce((sum, ch) => sum + ch.charCodeAt(0), 0);
    return FALLBACK_COLORS[hash % FALLBACK_COLORS.length];
  });
}
