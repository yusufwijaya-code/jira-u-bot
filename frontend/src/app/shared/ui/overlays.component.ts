import { Component, HostListener, inject } from '@angular/core';
import { ToastService } from '../../core/ui/toast.service';
import { ConfirmService } from '../../core/ui/confirm.service';
import { IconComponent } from './icon.component';
import { MoveFieldsDialogComponent } from './move-fields-dialog.component';

const TOAST_ICONS = { success: 'check-circle', error: 'alert-circle', info: 'info' } as const;

/** Hosts the global toast stack, the confirm dialog and the move-fields dialog. Mounted once in AppComponent. */
@Component({
  selector: 'app-overlays',
  standalone: true,
  imports: [IconComponent, MoveFieldsDialogComponent],
  template: `
    <app-move-fields-dialog />

    <div class="toast-stack" aria-live="polite">
      @for (toast of toastService.toasts(); track toast.id) {
        <div class="toast" [attr.data-tone]="toast.tone" [class.leaving]="toast.leaving" role="status">
          <span class="toast-icon"><app-icon [name]="toastIcons[toast.tone]" [size]="18" /></span>
          <div class="toast-body">
            <strong>{{ toast.title }}</strong>
            @if (toast.message) {
              <span>{{ toast.message }}</span>
            }
            @if (toast.action) {
              <button type="button" class="toast-action" data-toast-action (click)="toastService.runAction(toast.id)">{{ toast.action.label }}</button>
            }
          </div>
          <button type="button" class="toast-close" aria-label="Tutup" (click)="toastService.dismiss(toast.id)">
            <app-icon name="x" [size]="14" />
          </button>
          <span class="toast-progress"></span>
        </div>
      }
    </div>

    @if (confirmService.current(); as dialog) {
      <div class="modal-backdrop" (click)="confirmService.close(false)">
        <div
          class="modal"
          role="alertdialog"
          aria-modal="true"
          aria-labelledby="confirm-title"
          (click)="$event.stopPropagation()"
        >
          <div class="modal-icon" [class.danger]="dialog.tone === 'danger'">
            <app-icon [name]="dialog.icon" [size]="22" />
          </div>
          <h3 id="confirm-title">{{ dialog.title }}</h3>
          <p>{{ dialog.message }}</p>
          @if (dialog.points.length) {
            <ul class="modal-points">
              @for (point of dialog.points; track point) {
                <li><app-icon name="check-circle" [size]="15" /><span>{{ point }}</span></li>
              }
            </ul>
          }
          @if (dialog.note) {
            <p class="modal-note"><app-icon name="clock" [size]="13" /> {{ dialog.note }}</p>
          }
          <div class="modal-actions">
            <button type="button" class="btn btn-ghost" (click)="confirmService.close(false)">{{ dialog.cancelText }}</button>
            <button
              type="button"
              class="btn"
              [class.btn-danger]="dialog.tone === 'danger'"
              [class.btn-primary]="dialog.tone !== 'danger'"
              (click)="confirmService.close(true)"
              autofocus
            >
              {{ dialog.confirmText }}
            </button>
          </div>
        </div>
      </div>
    }
  `,
})
export class OverlaysComponent {
  protected toastService = inject(ToastService);
  protected confirmService = inject(ConfirmService);
  protected toastIcons = TOAST_ICONS;

  @HostListener('document:keydown.escape')
  protected onEscape(): void {
    this.confirmService.close(false);
  }
}
