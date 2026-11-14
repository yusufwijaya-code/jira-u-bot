import { TestBed } from '@angular/core/testing';
import { BoardService } from './board.service';
import { BoardContextService } from './board-context.service';
import { SETTINGS_MODE, SettingsApi, SettingsScope } from './settings-api';

describe('SettingsApi / SettingsScope defaults', () => {
  it('resolve to BoardService, BoardContextService and self mode', () => {
    const boardService = { tag: 'board' };
    const context = { tag: 'context' };
    TestBed.configureTestingModule({
      providers: [
        { provide: BoardService, useValue: boardService },
        { provide: BoardContextService, useValue: context },
      ],
    });

    expect(TestBed.inject(SettingsApi) as unknown).toBe(boardService);
    expect(TestBed.inject(SettingsScope) as unknown).toBe(context);
    expect(TestBed.inject(SETTINGS_MODE)).toBe('self');
  });
});
