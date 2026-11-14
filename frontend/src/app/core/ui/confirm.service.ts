import { Injectable, signal } from '@angular/core';

export interface ConfirmOptions {
  title: string;
  message: string;
  confirmText?: string;
  cancelText?: string;
  /** `danger` styles the confirm button red, for destructive actions. */
  tone?: 'danger' | 'primary';
  icon?: string;
}

export interface ConfirmRequest extends Required<ConfirmOptions> {
  resolve: (confirmed: boolean) => void;
}

/**
 * Promise-based replacement for window.confirm(), rendered as a modal by
 * <app-overlays> in the root component.
 */
@Injectable({ providedIn: 'root' })
export class ConfirmService {
  readonly current = signal<ConfirmRequest | null>(null);

  ask(options: ConfirmOptions): Promise<boolean> {
    // Only one dialog at a time: a newer request cancels the pending one.
    this.current()?.resolve(false);

    return new Promise<boolean>((resolve) => {
      this.current.set({
        confirmText: 'Ya, lanjutkan',
        cancelText: 'Batal',
        tone: 'primary',
        icon: options.tone === 'danger' ? 'trash' : 'info',
        ...options,
        resolve,
      });
    });
  }

  close(confirmed: boolean): void {
    const request = this.current();
    if (!request) {
      return;
    }
    this.current.set(null);
    request.resolve(confirmed);
  }
}
