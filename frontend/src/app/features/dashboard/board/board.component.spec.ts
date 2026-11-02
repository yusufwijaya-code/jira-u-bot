import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { BoardComponent } from './board.component';
import { BoardService, BoardResponse } from '../../../core/services/board.service';

const SAMPLE_BOARD: BoardResponse = {
  columns: [
    {
      status: 'Code Process',
      cards: [
        { issueKey: 'OMS-1', summary: 'Fix bug', status: 'Code Process', hasBudgetDetail: true, storyPoints: 3, elapsedLabel: '2j', remainingLabel: '4j', deadlineLabel: '' },
      ],
    },
    {
      status: 'Todo',
      cards: [
        { issueKey: 'OMS-2', summary: 'New feature', status: 'Todo', hasBudgetDetail: false, storyPoints: 0, elapsedLabel: '', remainingLabel: '', deadlineLabel: '' },
      ],
    },
  ],
  tierPreference: { tiers: ['2h', '1m'] },
  credential: { hasCredential: false, maskedPreview: '' },
};

describe('BoardComponent', () => {
  let fixture: ComponentFixture<BoardComponent>;
  let boardService: jasmine.SpyObj<BoardService>;

  beforeEach(async () => {
    boardService = jasmine.createSpyObj('BoardService', [
      'getBoard', 'updatePreference', 'saveCredential', 'deleteCredential', 'transitionTicket',
    ]);
    boardService.getBoard.and.returnValue(of(SAMPLE_BOARD));

    await TestBed.configureTestingModule({
      imports: [BoardComponent],
      providers: [{ provide: BoardService, useValue: boardService }],
    }).compileComponents();

    fixture = TestBed.createComponent(BoardComponent);
    fixture.detectChanges();
  });

  it('loads the board on init and renders one row per card, flattened across columns in order', () => {
    expect(boardService.getBoard).toHaveBeenCalled();
    const rows = fixture.nativeElement.querySelectorAll('tbody tr');
    expect(rows.length).toBe(2);
    expect(rows[0].getAttribute('data-issue-key')).toBe('OMS-1');
    expect(rows[1].getAttribute('data-issue-key')).toBe('OMS-2');
  });

  it('shows budget columns for hasBudgetDetail cards and dashes otherwise', () => {
    const rows = fixture.nativeElement.querySelectorAll('tbody tr');
    const firstRowBudgetCells = rows[0].querySelectorAll('.budget');
    expect(Array.from(firstRowBudgetCells).map((td: any) => td.textContent.trim())).toEqual(['3', '2j', '4j']);

    const secondRowBudgetCells = rows[1].querySelectorAll('.budget');
    expect(Array.from(secondRowBudgetCells).map((td: any) => td.textContent.trim())).toEqual(['—', '—', '—']);
  });

  it('shows an error message when loading the board fails', () => {
    boardService.getBoard.and.returnValue(throwError(() => ({ status: 500 })));
    fixture = TestBed.createComponent(BoardComponent);
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg.error').textContent).toContain('Gagal memuat board, coba lagi');
  });

  it('transitions a ticket and reloads the board on success', () => {
    boardService.transitionTicket.and.returnValue(of({ message: 'OMS-1 dipindahkan ke Todo' }));

    const rows = fixture.nativeElement.querySelectorAll('tbody tr');
    const todoButton: HTMLButtonElement = rows[0].querySelectorAll('.actions button')[0];
    todoButton.click();

    expect(boardService.transitionTicket).toHaveBeenCalledWith('OMS-1', 'todo');
    expect(boardService.getBoard).toHaveBeenCalledTimes(2);
  });

  it('shows an error message and re-enables the button when a transition fails', () => {
    boardService.transitionTicket.and.returnValue(throwError(() => ({ error: { error: 'Tiket tidak bisa dipindahkan' } })));

    const rows = fixture.nativeElement.querySelectorAll('tbody tr');
    const todoButton: HTMLButtonElement = rows[0].querySelectorAll('.actions button')[0];
    todoButton.click();
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg.error').textContent).toContain('Tiket tidak bisa dipindahkan');
    expect(todoButton.disabled).toBe(false);
  });
});
