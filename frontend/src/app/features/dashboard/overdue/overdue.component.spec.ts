import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { OverdueComponent } from './overdue.component';
import { BoardService, OverdueResponse } from '../../../core/services/board.service';

describe('OverdueComponent', () => {
  let fixture: ComponentFixture<OverdueComponent>;
  let boardService: jasmine.SpyObj<BoardService>;

  beforeEach(async () => {
    boardService = jasmine.createSpyObj('BoardService', ['getOverdue']);

    await TestBed.configureTestingModule({
      imports: [OverdueComponent],
      providers: [{ provide: BoardService, useValue: boardService }],
    }).compileComponents();

    fixture = TestBed.createComponent(OverdueComponent);
  });

  it('renders one card per overdue item', () => {
    const response: OverdueResponse = {
      items: [
        { issueKey: 'OMS-9', summary: 'Stuck ticket', status: 'Code Process', hasBudgetDetail: true, storyPoints: 5, elapsedLabel: '10j', remainingLabel: '-2j', deadlineLabel: '', assigneeEmail: 'a@esb.co.id' },
      ],
    };
    boardService.getOverdue.and.returnValue(of(response));
    fixture.detectChanges();

    const card = fixture.nativeElement.querySelector('.overdue-card');
    expect(card.textContent).toContain('OMS-9');
    expect(card.textContent).toContain('a@esb.co.id');
    expect(card.textContent).toContain('5 SP');
  });

  it('shows the empty-state message when there are no overdue items', () => {
    boardService.getOverdue.and.returnValue(of({ items: [] }));
    fixture.detectChanges();

    expect(fixture.nativeElement.textContent).toContain('Semua tiket masih dalam budget');
  });

  it('shows an error message when loading fails', () => {
    boardService.getOverdue.and.returnValue(throwError(() => ({ status: 500 })));
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg.error').textContent).toContain('Gagal memuat data overdue, coba lagi');
  });
});
