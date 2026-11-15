import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ConfirmService } from '../../../../core/ui/confirm.service';
import { BulkSubtasksComponent, titleKey } from './bulk-subtasks.component';

const meta = { projectKey: 'CORE', subtaskTypeId: '10133', types: [{ id: '10133', name: 'Subtask', subtask: true, supported: true, parentAllowed: true, storyPointField: 'customfield_11053', fields: [] }] };

describe('BulkSubtasksComponent', () => {
  let http: HttpTestingController;

  function render(aiReady = true) {
    TestBed.configureTestingModule({ imports: [BulkSubtasksComponent], providers: [provideHttpClient(), provideHttpClientTesting()] });
    http = TestBed.inject(HttpTestingController);
    const f = TestBed.createComponent(BulkSubtasksComponent);
    f.componentRef.setInput('boardId', 26);
    f.componentRef.setInput('meta', meta);
    f.componentRef.setInput('aiReady', aiReady);
    f.componentRef.setInput('initialParent', 'CORE-10923');
    f.detectChanges();
    http.expectOne((r) => r.url.endsWith('/api/dashboard/board')).flush({ columns: [], related: [] });
    http.expectOne((r) => r.url.endsWith('/create/parent/CORE-10923')).flush({ key: 'CORE-10923', summary: 'Required Date', subtasks: [{ key: 'CORE-10961', summary: '[Docs] Release Document' }] });
    f.detectChanges();
    return f;
  }

  function normalize(f: ReturnType<typeof render>) {
    const el: HTMLElement = f.nativeElement;
    const ta = el.querySelector('[data-paste]') as HTMLTextAreaElement;
    ta.value = '| # | Task |'; ta.dispatchEvent(new Event('input')); f.detectChanges();
    (el.querySelector('[data-normalize]') as HTMLButtonElement).click();
    f.detectChanges();
    expect(el.querySelector('[data-normalizing]')).withContext('AI loading panel while Gemini works').not.toBeNull();
    http.expectOne((r) => r.url.endsWith('/create/normalize')).flush({
      rows: [
        { title: '[Exec Plan] Implementasi', storyPoints: 4.5, details: 'Repo: semua' },
        { title: '[Docs]  release document', storyPoints: 1, details: '' },
        { title: 'Tanpa SP', storyPoints: null, details: '' },
      ], warnings: ['Ditemukan 2 tabel'], cached: false,
    });
    f.detectChanges();
  }

  it('normalizes, marks duplicates unchecked and totals SP', () => {
    const f = render();
    normalize(f);
    const el: HTMLElement = f.nativeElement;
    expect(el.querySelectorAll('[data-row]').length).toBe(3);
    expect((el.querySelector('[data-row="1"] [data-check]') as HTMLInputElement).checked).toBeFalse();
    expect(el.querySelector('[data-row="1"]')?.textContent).toContain('CORE-10961');
    expect(el.querySelector('[data-total]')?.textContent).toContain('2 baris');
    expect(el.querySelector('[data-total]')?.textContent).toContain('4.5 SP');
    expect(el.textContent).toContain('Ditemukan 2 tabel');
  });

  it('creates checked rows in one request and retries only the failed ones', () => {
    const f = render();
    normalize(f);
    const el: HTMLElement = f.nativeElement;
    (el.querySelector('[data-create]') as HTMLButtonElement).click();
    const req = http.expectOne((r) => r.url.endsWith('/create/subtasks'));
    expect(req.request.body.parentKey).toBe('CORE-10923');
    expect(req.request.body.rows.map((r: { title: string }) => r.title)).toEqual(['[Exec Plan] Implementasi', 'Tanpa SP']);
    const ids = req.request.body.rows.map((r: { clientId: string }) => r.clientId);
    req.flush({ results: [{ clientId: ids[0], key: 'CORE-1' }, { clientId: ids[1], error: 'SP: bad' }] });
    f.detectChanges();
    expect(el.querySelector('[data-summary-result]')?.textContent).toContain('1 dibuat, 1 gagal');
    http.expectOne((r) => r.url.endsWith('/create/parent/CORE-10923')).flush({ key: 'CORE-10923', summary: 'Required Date', subtasks: [{ key: 'CORE-10961', summary: '[Docs] Release Document' }] });
    f.detectChanges();
    (el.querySelector('[data-retry]') as HTMLButtonElement).click();
    const retry = http.expectOne((r) => r.url.endsWith('/create/subtasks'));
    expect(retry.request.body.rows.length).toBe(1);
    retry.flush({ results: [{ clientId: ids[1], key: 'CORE-2' }] });
  });

  it('works without AI: Normalize disabled, rows can be added by hand', () => {
    const f = render(false);
    const el: HTMLElement = f.nativeElement;
    expect((el.querySelector('[data-normalize]') as HTMLButtonElement).disabled).toBeTrue();
    (el.querySelector('[data-add-row]') as HTMLButtonElement).click();
    f.detectChanges();
    expect(el.querySelectorAll('[data-row]').length).toBe(1);
  });

  it('explains a daily quota error and keeps the pasted text', () => {
    const f = render();
    const el: HTMLElement = f.nativeElement;
    const ta = el.querySelector('[data-paste]') as HTMLTextAreaElement;
    ta.value = 'abc'; ta.dispatchEvent(new Event('input')); f.detectChanges();
    (el.querySelector('[data-normalize]') as HTMLButtonElement).click();
    http.expectOne((r) => r.url.endsWith('/create/normalize')).flush({ error: 'rate_limited', scope: 'daily', retryAfterSeconds: 60 }, { status: 429, statusText: 'Too Many' });
    f.detectChanges();
    expect(el.textContent).toContain('Kuota AI hari ini habis');
    expect((el.querySelector('[data-paste]') as HTMLTextAreaElement).value).toBe('abc');
  });

  it('normalizes titles for duplicate checks', () => {
    expect(titleKey('[Docs]  Release-Document!')).toBe(titleKey('[docs] release document'));
  });

  it('on a transport error re-reads the parent and adopts already-created titles', () => {
    const f = render();
    normalize(f);
    const el: HTMLElement = f.nativeElement;
    (el.querySelector('[data-create]') as HTMLButtonElement).click();
    const req = http.expectOne((r) => r.url.endsWith('/create/subtasks'));
    req.error(new ProgressEvent('error'), { status: 0 });
    http.expectOne((r) => r.url.endsWith('/create/parent/CORE-10923')).flush({
      key: 'CORE-10923', summary: 'Required Date',
      subtasks: [{ key: 'CORE-10961', summary: '[Docs] Release Document' }, { key: 'CORE-20000', summary: '[Exec Plan] Implementasi' }],
    });
    f.detectChanges();
    expect(el.textContent).toContain('Koneksi terputus');
    expect(el.querySelector('[data-row="0"]')?.textContent).toContain('✅ CORE-20000');
    expect((el.querySelector('[data-row="0"] [data-check]') as HTMLInputElement).checked).toBeFalse();
  });

  it('keeps a deliberately re-checked duplicate checked while its title is edited', () => {
    const f = render();
    normalize(f);
    const el: HTMLElement = f.nativeElement;
    const check = el.querySelector('[data-row="1"] [data-check]') as HTMLInputElement;
    check.click(); f.detectChanges();
    expect(check.checked).toBeTrue();
    const title = el.querySelector('[data-row="1"] .bs-title') as HTMLInputElement;
    title.value = '[Docs] Release  document'; title.dispatchEvent(new Event('input')); f.detectChanges();
    expect((el.querySelector('[data-row="1"] [data-check]') as HTMLInputElement).checked).toBeTrue();
  });

  it('a 422 from our API marks rows failed with the server message and does not re-read the parent', () => {
    const f = render();
    normalize(f);
    const el: HTMLElement = f.nativeElement;
    (el.querySelector('[data-create]') as HTMLButtonElement).click();
    http.expectOne((r) => r.url.endsWith('/create/subtasks')).flush({ error: 'SP harus 0–100' }, { status: 422, statusText: 'x' });
    f.detectChanges();
    http.expectNone((r) => r.url.includes('/create/parent/'));
    expect(el.textContent).toContain('SP harus 0–100');
    expect(el.textContent).not.toContain('Koneksi terputus');
  });

  it('adopts rows found in Jira after a transport error and retries only the rest', () => {
    const f = render();
    normalize(f);
    const el: HTMLElement = f.nativeElement;
    (el.querySelector('[data-create]') as HTMLButtonElement).click();
    http.expectOne((r) => r.url.endsWith('/create/subtasks')).error(new ProgressEvent('error'), { status: 0 });
    http.expectOne((r) => r.url.endsWith('/create/parent/CORE-10923')).flush({
      key: 'CORE-10923', summary: 'Required Date', subtasks: [{ key: 'CORE-20000', summary: '[Exec Plan] Implementasi' }],
    });
    f.detectChanges();
    expect(el.querySelector('[data-row="0"]')?.textContent).toContain('✅ CORE-20000');
    (el.querySelector('[data-retry]') as HTMLButtonElement).click();
    const retry = http.expectOne((r) => r.url.endsWith('/create/subtasks'));
    expect(retry.request.body.rows.map((r: { title: string }) => r.title)).toEqual(['Tanpa SP']);
    retry.flush({ results: [] });
  });

  it('asks before replacing an existing preview and keeps it when declined', async () => {
    const f = render();
    normalize(f);
    const confirm = TestBed.inject(ConfirmService);
    const ask = spyOn(confirm, 'ask').and.resolveTo(false);
    const el: HTMLElement = f.nativeElement;
    (el.querySelector('[data-normalize]') as HTMLButtonElement).click();
    await f.whenStable();
    expect(ask).toHaveBeenCalled();
    http.expectNone((r) => r.url.endsWith('/create/normalize'));
    expect(el.querySelectorAll('[data-row]').length).toBe(3);
  });

  it('blocks create and explains when SP is out of range', () => {
    const f = render();
    normalize(f);
    const el: HTMLElement = f.nativeElement;
    const sp = el.querySelector('[data-row="0"] .bs-sp') as HTMLInputElement;
    sp.value = '101'; sp.dispatchEvent(new Event('input')); f.detectChanges();
    expect((el.querySelector('[data-create]') as HTMLButtonElement).disabled).toBeTrue();
    expect(el.querySelector('[data-blocker]')?.textContent).toContain('SP harus 0–100');
  });
});
