import { Component, HostListener, inject } from '@angular/core';
import { ToastService } from '../../core/ui/toast.service';
import { ConfirmService } from '../../core/ui/confirm.service';
import { IconComponent } from './icon.component';

const TOAST_ICONS = { success: 'check-circle', error: 'alert-circle', info: 'info' } as const;

/** Hosts the global toast stack and the confirm dialog. Mounted once in AppComponent. */
@Component({
  selector: 'app-overlays',
  standalone: true,
  imports: [IconComponent],
  template: `
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
