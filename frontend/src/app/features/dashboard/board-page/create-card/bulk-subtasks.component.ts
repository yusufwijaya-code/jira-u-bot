import { Component, input, output } from '@angular/core';
import { CreateMeta } from '../../../../core/services/issue-create.service';

/** Placeholder until the bulk subtask flow lands. */
@Component({
  selector: 'app-bulk-subtasks',
  standalone: true,
  template: `<p>Segera hadir</p>`,
})
export class BulkSubtasksComponent {
  boardId = input.required<number>();
  meta = input.required<CreateMeta>();
  aiReady = input(false);
  initialParent = input<string | null>(null);
  created = output<void>();
}
