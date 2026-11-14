import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { ReminderPreferenceComponent } from './reminder-preference.component';
import { BoardService } from '../../../core/services/board.service';

describe('ReminderPreferenceComponent', () => {
  let fixture: ComponentFixture<ReminderPreferenceComponent>;
  let boardService: jasmine.SpyObj<BoardService>;

  beforeEach(async () => {
    boardService = jasmine.createSpyObj('BoardService', ['updatePreference']);

    await TestBed.configureTestingModule({
      imports: [ReminderPreferenceComponent],
      providers: [{ provide: BoardService, useValue: boardService }],
    }).compileComponents();

    fixture = TestBed.createComponent(ReminderPreferenceComponent);
    fixture.componentRef.setInput('tiers', ['2h', '1m']);
    fixture.detectChanges();
  });

  it('pre-checks only the checkboxes matching the tiers input', () => {
    expect(fixture.componentInstance['form'].value).toEqual({
      '2h': true, '1h': false, '30m': false, '15m': false, '5m': false, '1m': true,
    });
    const checkboxes: HTMLInputElement[] = Array.from(fixture.nativeElement.querySelectorAll('input[type=checkbox]'));
    expect(checkboxes.map((cb) => cb.checked)).toEqual([true, false, false, false, false, true]);
  });

  it('submits only the checked tier codes and shows a success message', () => {
    boardService.updatePreference.and.returnValue(of({ status: 'ok' }));

    fixture.componentInstance['form'].patchValue({ '2h': false });
    fixture.nativeElement.querySelector('form').dispatchEvent(new Event('submit'));
    fixture.detectChanges();

    expect(boardService.updatePreference).toHaveBeenCalledWith(['1m']);
    expect(fixture.nativeElement.querySelector('.msg').textContent).toContain('Preference tersimpan');
  });

  it('submits an empty array when every tier is unchecked (AC5.1 allows zero selected)', () => {
    boardService.updatePreference.and.returnValue(of({ status: 'ok' }));

    fixture.componentInstance['form'].patchValue({ '2h': false, '1m': false });
    fixture.nativeElement.querySelector('form').dispatchEvent(new Event('submit'));

    expect(boardService.updatePreference).toHaveBeenCalledWith([]);
  });

  it('shows an error message and keeps the checkbox state when the save fails', () => {
    boardService.updatePreference.and.returnValue(throwError(() => ({ error: { error: 'failed to save preference' } })));

    fixture.nativeElement.querySelector('form').dispatchEvent(new Event('submit'));
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg').textContent).toContain('failed to save preference');
    expect(fixture.componentInstance['form'].value['2h']).toBe(true);
  });
});
