import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { of, throwError } from 'rxjs';
import { SettingsComponent } from './settings.component';
import { BoardService, SettingsResponse } from '../../../core/services/board.service';

const SAMPLE_SETTINGS: SettingsResponse = {
  tierPreference: { tiers: ['2h', '1m'] },
  credential: { hasCredential: false, maskedPreview: '' },
};

describe('SettingsComponent', () => {
  let fixture: ComponentFixture<SettingsComponent>;
  let boardService: jasmine.SpyObj<BoardService>;

  beforeEach(async () => {
    boardService = jasmine.createSpyObj('BoardService', [
      'getSettings', 'updatePreference', 'saveCredential', 'deleteCredential',
    ]);
    boardService.getSettings.and.returnValue(of(SAMPLE_SETTINGS));

    await TestBed.configureTestingModule({
      imports: [SettingsComponent],
      providers: [
        provideHttpClient(),
        provideHttpClientTesting(),
        { provide: BoardService, useValue: boardService },
      ],
    }).compileComponents();

    fixture = TestBed.createComponent(SettingsComponent);
    fixture.detectChanges();
  });

  it('loads settings on init and renders the reminder preference and credential sections', () => {
    expect(boardService.getSettings).toHaveBeenCalled();
    const el: HTMLElement = fixture.nativeElement;
    expect(el.querySelector('app-reminder-preference')).not.toBeNull();
    expect(el.querySelector('app-jira-credential')).not.toBeNull();
  });

  it('pre-checks only the saved reminder tiers', () => {
    const checked = Array.from(
      fixture.nativeElement.querySelectorAll('app-reminder-preference input[type=checkbox]'),
    ).map((el: any) => el.checked);
    expect(checked).toEqual([true, false, false, false, false, true]);
  });

  it('shows an error message and hides both sections when loading fails', () => {
    boardService.getSettings.and.returnValue(throwError(() => ({ status: 500 })));
    fixture = TestBed.createComponent(SettingsComponent);
    fixture.detectChanges();

    const el: HTMLElement = fixture.nativeElement;
    expect(el.querySelector('.msg.error')?.textContent).toContain('Gagal memuat pengaturan, coba lagi');
    expect(el.querySelector('app-reminder-preference')).toBeNull();
  });
});
