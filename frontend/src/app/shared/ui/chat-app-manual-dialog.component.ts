import { Component, EventEmitter, Input, Output } from '@angular/core';
import { IconComponent } from './icon.component';

/** Steps to install the Nudge app by hand in Google Chat (AC3.5). */
@Component({
  selector: 'app-chat-app-manual-dialog',
  standalone: true,
  imports: [IconComponent],
  template: `
    @if (open) {
      <div class="modal-backdrop" (click)="closed.emit()">
        <div class="modal chat-app-manual" role="dialog" aria-modal="true" aria-labelledby="chat-app-manual-title" (click)="$event.stopPropagation()">
          <div class="modal-icon"><app-icon name="bell" [size]="22" /></div>
          <h3 id="chat-app-manual-title">Aktifkan Nudge secara manual</h3>
          <ol>
            <li>Buka <a href="https://mail.google.com/chat/" target="_blank" rel="noopener">Google Chat</a>.</li>
            <li>Klik <strong>New chat</strong>, lalu ketik <strong>Nudge</strong>.</li>
            <li>Pilih <strong>Nudge</strong> (App), lalu klik <strong>Install app</strong>.</li>
            <li>Kembali ke halaman ini — statusnya diperbarui otomatis.</li>
          </ol>
          <button type="button" class="btn btn-primary btn-block" (click)="closed.emit()">Mengerti</button>
        </div>
      </div>
    }
  `,
})
export class ChatAppManualDialogComponent {
  @Input() open = false;
  @Output() closed = new EventEmitter<void>();
}
