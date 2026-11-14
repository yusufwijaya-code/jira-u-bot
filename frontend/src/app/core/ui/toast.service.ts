import { Injectable, signal } from '@angular/core';

export type ToastTone = 'success' | 'error' | 'info';

export interface Toast {
  id: number;
  tone: ToastTone;
  title: string;
  message?: string;
  leaving: boolean;
}

const DEFAULT_DURATION_MS = 4000;
const LEAVE_ANIMATION_MS = 220;

/** App-wide toast notifications, rendered by <app-overlays> in the root component. */
@Injectable({ providedIn: 'root' })
export class ToastService {
  readonly toasts = signal<Toast[]>([]);
  private nextId = 1;

  success(title: string, message?: string): void {
    this.show('success', title, message);
  }

  error(title: string, message?: string): void {
    this.show('error', title, message);
  }

  info(title: string, message?: string): void {
    this.show('info', title, message);
  }

  show(tone: ToastTone, title: string, message?: string, durationMs = DEFAULT_DURATION_MS): void {
    const id = this.nextId++;
    this.toasts.update((list) => [...list, { id, tone, title, message, leaving: false }]);
    setTimeout(() => this.dismiss(id), durationMs);
  }

  dismiss(id: number): void {
    const toast = this.toasts().find((t) => t.id === id);
    if (!toast || toast.leaving) {
      return;
    }
    // Play the exit animation before removing it from the DOM.
    this.toasts.update((list) => list.map((t) => (t.id === id ? { ...t, leaving: true } : t)));
    setTimeout(() => this.toasts.update((list) => list.filter((t) => t.id !== id)), LEAVE_ANIMATION_MS);
  }
}
