import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { ReportComponent } from './report.component';
import { ReportService, GetReportResponse } from '../../../core/services/report.service';

describe('ReportComponent', () => {
  let fixture: ComponentFixture<ReportComponent>;
  let reportService: jasmine.SpyObj<ReportService>;

  beforeEach(async () => {
    reportService = jasmine.createSpyObj('ReportService', ['getReport']);
    reportService.getReport.and.returnValue(of({ availableSprints: ['Sprint 42', 'Sprint 43'], capPercent: 12, report: null }));

    await TestBed.configureTestingModule({
      imports: [ReportComponent],
      providers: [{ provide: ReportService, useValue: reportService }],
    }).compileComponents();

    fixture = TestBed.createComponent(ReportComponent);
    fixture.detectChanges();
  });

  it('loads the available sprints on init and shows the picker without a table', () => {
    expect(reportService.getReport).toHaveBeenCalledWith();
    const labels = fixture.nativeElement.querySelectorAll('.sprint-picker label');
    expect(labels.length).toBeGreaterThanOrEqual(2);
    expect(fixture.nativeElement.querySelector('.report-table')).toBeNull();
  });

  it('shows an error message when loading the initial sprint list fails', () => {
    reportService.getReport.and.returnValue(throwError(() => ({ status: 500 })));
    fixture = TestBed.createComponent(ReportComponent);
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg.error').textContent).toContain('Gagal memuat daftar sprint, coba lagi');
  });

  it('generates a report for the selected sprints and cap, and renders the table plus summary', () => {
    const response: GetReportResponse = {
      availableSprints: ['Sprint 42'],
      capPercent: 12,
      report: {
        rows: [
          { sprintName: 'Sprint 42', issueKey: 'OMS-1', task: 'Fix bug', estimatedHours: 8, actualHours: 7, deviationPercent: -12.5, within: true },
          { sprintName: 'Sprint 42', issueKey: 'OMS-2', task: 'New feature', estimatedHours: 8, actualHours: 20, deviationPercent: 150, within: false },
        ],
        sumEstimatedHours: 16,
        sumActualHours: 27,
        aggregateDeviationPercent: 68.75,
        achievementPercent: 50,
        capPercent: 12,
        withinCount: 1,
        totalCount: 2,
      },
    };
    reportService.getReport.and.returnValue(of(response));

    fixture.componentInstance['onSprintToggle']('Sprint 42', { target: { checked: true } } as unknown as Event);
    fixture.componentInstance['onGenerate']();
    fixture.detectChanges();

    expect(reportService.getReport).toHaveBeenCalledWith(['Sprint 42'], 12);
    const rows = fixture.nativeElement.querySelectorAll('.report-table tbody tr');
    expect(rows.length).toBe(2);
    expect(rows[1].classList).toContain('not-within');
    expect(fixture.nativeElement.querySelector('.summary').textContent).toContain('68.75');
  });

  it('keeps the previous report and shows an error message when a later generate fails', () => {
    const okResponse: GetReportResponse = {
      availableSprints: ['Sprint 42'],
      capPercent: 12,
      report: { rows: [], sumEstimatedHours: 0, sumActualHours: 0, aggregateDeviationPercent: 0, achievementPercent: 0, capPercent: 12, withinCount: 0, totalCount: 0 },
    };
    reportService.getReport.and.returnValue(of(okResponse));
    fixture.componentInstance['onGenerate']();
    fixture.detectChanges();

    reportService.getReport.and.returnValue(throwError(() => ({ status: 500 })));
    fixture.componentInstance['onGenerate']();
    fixture.detectChanges();

    expect(fixture.componentInstance['report']).toEqual(okResponse.report);
    expect(fixture.nativeElement.querySelector('.msg.error').textContent).toContain('Gagal membuat report, coba lagi');
  });
});
