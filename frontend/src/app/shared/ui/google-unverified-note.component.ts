import { Component } from '@angular/core';

/**
 * What to do on Google's "Google hasn't verified this app" screen during the
 * one-click activation: Nudge's OAuth app asks for the sensitive
 * chat.spaces.create scope and is not verified by Google.
 */
@Component({
  selector: 'app-google-unverified-note',
  standalone: true,
  template: `
    <p class="google-unverified-note" data-unverified-note>
      Kalau Google menampilkan <strong>"Google hasn't verified this app"</strong>, klik <strong>Advanced</strong>
      lalu <strong>Go to labcup.tech (unsafe)</strong>. Ini aman: app internal Nudge memang belum melalui verifikasi Google.
    </p>
  `,
})
export class GoogleUnverifiedNoteComponent {}
