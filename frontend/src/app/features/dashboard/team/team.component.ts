import { Component, OnInit, inject } from '@angular/core';
import { NgFor, NgIf } from '@angular/common';
import { BoardService, TeamGroup } from '../../../core/services/board.service';

@Component({
  selector: 'app-team',
  standalone: true,
  imports: [NgFor, NgIf],
  templateUrl: './team.component.html',
})
export class TeamComponent implements OnInit {
  protected groups: TeamGroup[] = [];
  protected loading = true;
  protected error: string | null = null;

  private boardService = inject(BoardService);

  ngOnInit(): void {
    this.boardService.getTeam().subscribe({
      next: (res) => {
        this.groups = res.groups;
        this.loading = false;
      },
      error: () => {
        this.error = 'Gagal memuat data team, coba lagi';
        this.loading = false;
      },
    });
  }
}
