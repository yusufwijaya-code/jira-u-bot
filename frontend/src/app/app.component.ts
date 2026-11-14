import { Component } from '@angular/core';
import { RouterOutlet } from '@angular/router';
import { OverlaysComponent } from './shared/ui/overlays.component';

@Component({
  selector: 'app-root',
  imports: [RouterOutlet, OverlaysComponent],
  templateUrl: './app.component.html',
  styleUrl: './app.component.scss',
})
export class AppComponent {
  title = 'Nudge';
}
