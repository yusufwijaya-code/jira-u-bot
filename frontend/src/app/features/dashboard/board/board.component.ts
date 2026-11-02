import { Component, OnInit, inject } from '@angular/core';
import { NgFor, NgIf } from '@angular/common';
import { BoardService, Card } from '../../../core/services/board.service';
import { ReminderPreferenceComponent } from './reminder-preference.component';
import { JiraCredentialComponent } from './jira-credential.component';

@Component({
  selector: 'app-board',
  standalone: true,
  imports: [NgFor, NgIf, ReminderPreferenceComponent, JiraCredentialComponent],
  templateUrl: './board.component.html',
})
export class BoardComponent implements OnInit {
  protected cards: Card[] = [];
  protected loading = true;
  protected error: string | null = null;
  protected tiers: string[] = [];
  protected hasCredential = false;
  protected credentialMasked: string | null = null;
  protected transitioningIssueKey: string | null = null;
  protected actionMessage: { text: string; isError: boolean } | null = null;

  private boardService = inject(BoardService);

  ngOnInit(): void {
    this.loadBoard();
  }

  protected loadBoard(): void {
    this.loading = true;
    this.error = null;

    this.boardService.getBoard().subscribe({
      next: (res) => {
        this.cards = res.columns.flatMap((col) => col.cards);
        this.tiers = res.tierPreference.tiers;
        this.hasCredential = res.credential.hasCredential;
        this.credentialMasked = res.credential.maskedPreview;
        this.loading = false;
      },
      error: () => {
        this.error = 'Gagal memuat board, coba lagi';
        this.loading = false;
      },
    });
  }

  protected onTransition(issueKey: string, target: 'todo' | 'code-review'): void {
    this.transitioningIssueKey = issueKey;

    this.boardService.transitionTicket(issueKey, target).subscribe({
      next: (res) => {
        this.actionMessage = { text: res.message, isError: false };
        this.transitioningIssueKey = null;
        this.loadBoard();
      },
      error: (err) => {
        this.actionMessage = { text: err?.error?.error ?? 'Gagal memindahkan tiket', isError: true };
        this.transitioningIssueKey = null;
      },
    });
  }

  protected onCredentialChanged(): void {
    this.loadBoard();
  }
}
