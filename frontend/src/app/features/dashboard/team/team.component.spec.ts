import { ComponentFixture, TestBed } from '@angular/core/testing';
import { of, throwError } from 'rxjs';
import { TeamComponent } from './team.component';
import { BoardService, TeamResponse } from '../../../core/services/board.service';

describe('TeamComponent', () => {
  let fixture: ComponentFixture<TeamComponent>;
  let boardService: jasmine.SpyObj<BoardService>;

  beforeEach(async () => {
    boardService = jasmine.createSpyObj('BoardService', ['getTeam']);

    await TestBed.configureTestingModule({
      imports: [TeamComponent],
      providers: [{ provide: BoardService, useValue: boardService }],
    }).compileComponents();

    fixture = TestBed.createComponent(TeamComponent);
  });

  it('renders one group per assignee with a ticket count heading', () => {
    const response: TeamResponse = {
      groups: [
        { assigneeEmail: 'a@esb.co.id', cards: [{ issueKey: 'OMS-1', summary: 'X', status: 'Code Process', hasBudgetDetail: false, storyPoints: 0, elapsedLabel: '', remainingLabel: '', deadlineLabel: '' }] },
      ],
    };
    boardService.getTeam.and.returnValue(of(response));
    fixture.detectChanges();

    const heading = fixture.nativeElement.querySelector('.team-group h3');
    expect(heading.textContent).toContain('a@esb.co.id');
    expect(heading.textContent).toContain('(1)');
  });

  it('shows the empty-state message when there are no groups', () => {
    boardService.getTeam.and.returnValue(of({ groups: [] }));
    fixture.detectChanges();

    expect(fixture.nativeElement.textContent).toContain('Tidak ada aktivitas Code Process saat ini');
  });

  it('shows an error message when loading fails', () => {
    boardService.getTeam.and.returnValue(throwError(() => ({ status: 500 })));
    fixture.detectChanges();

    expect(fixture.nativeElement.querySelector('.msg.error').textContent).toContain('Gagal memuat data team, coba lagi');
  });
});
