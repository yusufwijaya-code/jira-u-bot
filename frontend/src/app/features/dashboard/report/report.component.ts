import { Component, OnInit, inject } from '@angular/core';
import { NgFor, NgIf } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { ReportService, Report } from '../../../core/services/report.service';

const DEFAULT_CAP_PERCENT = 12;

@Component({
  selector: 'app-report',
  standalone: true,
  imports: [NgFor, NgIf, FormsModule],
  templateUrl: './report.component.html',
})
export class ReportComponent implements OnInit {
  protected availableSprints: string[] = [];
  protected selectedSprints = new Set<string>();
  protected capPercent = DEFAULT_CAP_PERCENT;
  protected report: Report | null = null;
  protected loading = true;
  protected generating = false;
  protected error: string | null = null;

  private reportService = inject(ReportService);

  ngOnInit(): void {
    this.reportService.getReport().subscribe({
      next: (res) => {
        this.availableSprints = res.availableSprints;
        this.capPercent = res.capPercent;
        this.loading = false;
      },
      error: () => {
        this.error = 'Gagal memuat daftar sprint, coba lagi';
        this.loading = false;
      },
    });
  }

  protected onSprintToggle(sprint: string, event: Event): void {
    const checked = (event.target as HTMLInputElement).checked;
    if (checked) {
      this.selectedSprints.add(sprint);
    } else {
      this.selectedSprints.delete(sprint);
    }
  }

  protected onGenerate(): void {
    this.generating = true;
    this.error = null;

    this.reportService.getReport(Array.from(this.selectedSprints), this.capPercent).subscribe({
      next: (res) => {
        this.report = res.report;
        this.generating = false;
      },
      error: () => {
        this.error = 'Gagal membuat report, coba lagi';
        this.generating = false;
      },
    });
  }
}
