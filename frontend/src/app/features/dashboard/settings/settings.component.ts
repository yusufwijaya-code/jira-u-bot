import { Component, OnInit, inject } from '@angular/core';
import { BoardService } from '../../../core/services/board.service';
import { IconComponent } from '../../../shared/ui/icon.component';
import { ReminderPreferenceComponent } from './reminder-preference.component';
import { JiraCredentialComponent } from './jira-credential.component';

/** Personal settings: reminder preference and Jira API key. */
@Component({
  selector: 'app-settings',
  standalone: true,
  imports: [IconComponent, ReminderPreferenceComponent, JiraCredentialComponent],
  templateUrl: './settings.component.html',
})
export class SettingsComponent implements OnInit {
  protected loading = true;
  protected error: string | null = null;
  protected tiers: string[] = [];
  protected hasCredential = false;
  protected credentialMasked: string | null = null;

  private boardService = inject(BoardService);

  ngOnInit(): void {
    this.loadSettings();
  }

  protected loadSettings(): void {
    this.loading = true;
    this.error = null;

    this.boardService.getSettings().subscribe({
      next: (res) => {
        this.tiers = res.tierPreference.tiers;
        this.hasCredential = res.credential.hasCredential;
        this.credentialMasked = res.credential.maskedPreview;
        this.loading = false;
      },
      error: () => {
        this.error = 'Gagal memuat pengaturan, coba lagi';
        this.loading = false;
      },
    });
  }

  protected onCredentialChanged(): void {
    this.loadSettings();
  }
}
