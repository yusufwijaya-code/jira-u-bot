import { Component, OnInit, inject } from '@angular/core';
import { NgFor, NgIf } from '@angular/common';
import { BoardService, OverdueItem } from '../../../core/services/board.service';

@Component({
  selector: 'app-overdue',
  standalone: true,
  imports: [NgFor, NgIf],
  templateUrl: './overdue.component.html',
})
export class OverdueComponent implements OnInit {
  protected items: OverdueItem[] = [];
  protected loading = true;
  protected error: string | null = null;

  private boardService = inject(BoardService);

  ngOnInit(): void {
    this.boardService.getOverdue().subscribe({
      next: (res) => {
        this.items = res.items;
        this.loading = false;
      },
      error: () => {
        this.error = 'Gagal memuat data overdue, coba lagi';
        this.loading = false;
      },
    });
  }
}
