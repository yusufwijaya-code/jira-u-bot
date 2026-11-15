import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { Router, provideRouter } from '@angular/router';
import { Component } from '@angular/core';
import { BOARD_STORAGE_KEY, BoardContextService, BoardSummary } from './board-context.service';
import { ToastService } from '../ui/toast.service';

@Component({ standalone: true, template: '' })
class BlankComponent {}

const OMS: BoardSummary = {
  boardId: 28,
  name: 'ESB OMS Board',
  projectKey: 'OMS',
  activeSprint: { id: 8294, name: 'OMS Sprint 23 - 2026', startDate: '', endDate: '' },
  ticketCount: 17,
  hasCustomSettings: false,
  muted: { active: false, until: null },
};

describe('BoardContextService', () => {
  let http: HttpTestingController;
  let toast: jasmine.SpyObj<ToastService>;

  function setup(): BoardContextService {
    return TestBed.inject(BoardContextService);
  }

  beforeEach(() => {
    localStorage.removeItem(BOARD_STORAGE_KEY);
    toast = jasmine.createSpyObj('ToastService', ['success', 'error', 'info']);
    TestBed.configureTestingModule({
      providers: [
        provideHttpClient(),
        provideHttpClientTesting(),
        provideRouter([{ path: '**', component: BlankComponent }]),
        { provide: ToastService, useValue: toast },
      ],
    });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => localStorage.removeItem(BOARD_STORAGE_KEY));

  it('starts on "Semua board" and loads the boards', () => {
    const svc = setup();
    expect(svc.selectedId()).toBeNull();

    svc.load();
    http.expectOne('/api/dashboard/boards').flush({ boards: [OMS] });

    expect(svc.boards().length).toBe(1);
    expect(svc.loaded()).toBeTrue();
  });

  it('restores the board from localStorage', () => {
    localStorage.setItem(BOARD_STORAGE_KEY, '28');
    const svc = setup();
    expect(svc.selectedId()).toBe(28);

    svc.load();
    http.expectOne('/api/dashboard/boards').flush({ boards: [OMS] });
    expect(svc.selected()?.name).toBe('ESB OMS Board');
  });

  it('select() stores the choice and puts it in the URL', async () => {
    const svc = setup();
    const router = TestBed.inject(Router);
    await router.navigateByUrl('/dashboard');

    svc.select(28);
    await new Promise((r) => setTimeout(r));

    expect(localStorage.getItem(BOARD_STORAGE_KEY)).toBe('28');
    expect(router.url).toContain('board=28');

    svc.select(null);
    await new Promise((r) => setTimeout(r));
    expect(localStorage.getItem(BOARD_STORAGE_KEY)).toBeNull();
    expect(router.url).not.toContain('board=');
  });

  it('follows ?board= in the URL', async () => {
    const svc = setup();
    await TestBed.inject(Router).navigateByUrl('/dashboard?board=26');
    expect(svc.selectedId()).toBe(26);
  });

  it('falls back to "Semua board" with a toast when the selected board is gone', () => {
    localStorage.setItem(BOARD_STORAGE_KEY, '99');
    const svc = setup();

    svc.load();
    http.expectOne('/api/dashboard/boards').flush({ boards: [OMS] });

    expect(svc.selectedId()).toBeNull();
    expect(toast.info).toHaveBeenCalled();
  });

  it('builds the avatar proxy URL', () => {
    expect(setup().avatarUrl(28)).toBe('/api/dashboard/boards/28/avatar');
  });
});
