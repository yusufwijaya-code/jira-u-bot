import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { CreateMeta } from '../../../../core/services/issue-create.service';
import { ComponentFixture } from '@angular/core/testing';
import { SingleCardFormComponent } from './single-card-form.component';

/** Opens the app-select under `host` and clicks the option whose label starts with `label`. */
function choose(f: ComponentFixture<unknown>, host: string, label: string) {
  const el: HTMLElement = f.nativeElement;
  (el.querySelector(`${host} .ui-select-trigger`) as HTMLButtonElement).click();
  f.detectChanges();
  const opt = Array.from(el.querySelectorAll(`${host} .ui-select-option`)).find((o) => o.textContent!.trim().startsWith(label)) as HTMLElement;
  opt.click();
  f.detectChanges();
}
const shown = (el: HTMLElement, host: string) => el.querySelector(`${host} .ui-select-value`)?.textContent?.trim();

const meta: CreateMeta = {
  projectKey: 'OMS', subtaskTypeId: '10143',
  types: [
    { id: '10139', name: 'Story', subtask: false, supported: true, parentAllowed: true, storyPointField: 'customfield_10016', fields: [
      { id: 'customfield_12001', name: 'Merge Request', kind: 'text', multi: false, required: true },
      { id: 'customfield_12002', name: 'Cloud Services', kind: 'select', multi: false, required: true, options: [{ id: '1', label: 'OMS SERVICE' }] },
    ] },
    { id: '9', name: 'Weird', subtask: false, supported: false, parentAllowed: false, fields: [] },
  ],
};

function setIn(el: HTMLElement, sel: string, v: string) {
  const i = el.querySelector(sel) as HTMLInputElement;
  i.value = v; i.dispatchEvent(new Event('input'));
}

describe('SingleCardFormComponent', () => {
  let http: HttpTestingController;
  function render() {
    TestBed.configureTestingModule({ imports: [SingleCardFormComponent], providers: [provideHttpClient(), provideHttpClientTesting()] });
    http = TestBed.inject(HttpTestingController);
    const f = TestBed.createComponent(SingleCardFormComponent);
    f.componentRef.setInput('boardId', 28);
    f.componentRef.setInput('meta', meta);
    f.detectChanges();
    return f;
  }

  it("renders the type's required fields and submits them", () => {
    const f = render();
    const el: HTMLElement = f.nativeElement;
    expect(el.querySelector('select')).toBeNull(); // styled dropdowns only
    choose(f, '[data-type]', 'Story');
    expect(el.querySelector('[data-field="customfield_12001"]')).not.toBeNull();
    (el.querySelector('[data-field="customfield_12002"] .ui-select-trigger') as HTMLButtonElement).click(); f.detectChanges();
    expect(el.querySelectorAll('[data-field="customfield_12002"] .ui-select-option').length).toBe(2); // placeholder + 1
    (el.querySelector('[data-field="customfield_12002"] .ui-select-trigger') as HTMLButtonElement).click(); f.detectChanges();
    setIn(el, '[data-summary]', 'New story'); setIn(el, '[data-field="customfield_12001"]', 'https://mr'); choose(f, '[data-field="customfield_12002"]', 'OMS SERVICE');
    (el.querySelector('[data-submit]') as HTMLButtonElement).click();
    const req = http.expectOne((r) => r.url.endsWith('/api/dashboard/create/issue'));
    expect(req.request.body).toEqual(jasmine.objectContaining({ boardId: 28, typeId: '10139', summary: 'New story', fields: { customfield_12001: 'https://mr', customfield_12002: '1' } }));
    req.flush({ key: 'OMS-77', url: 'https://x/browse/OMS-77' });
    f.detectChanges();
    expect(el.querySelector('[data-created]')?.textContent).toContain('OMS-77');
  });

  function fillAll(f: ComponentFixture<unknown>) { choose(f, '[data-type]', 'Story'); }

  it('clears the DOM after "Buat lagi" and blocks a duplicate submit after success', async () => {
    const f = render();
    const el: HTMLElement = f.nativeElement;
    fillAll(f);
    setIn(el, '[data-summary]', 'S'); setIn(el, '[data-field="customfield_12001"]', 'mr'); choose(f, '[data-field="customfield_12002"]', 'OMS SERVICE');
    f.detectChanges();
    (el.querySelector('[data-submit]') as HTMLButtonElement).click();
    http.expectOne((r) => r.url.endsWith('/create/issue')).flush({ key: 'OMS-1', url: 'u' });
    f.detectChanges();
    expect((el.querySelector('[data-submit]') as HTMLButtonElement).disabled).toBeTrue();
    (el.querySelector('[data-created] button') as HTMLButtonElement).click();
    f.detectChanges();
    await f.whenStable(); f.detectChanges(); // ngModel writes into app-select asynchronously
    expect((el.querySelector('[data-summary]') as HTMLInputElement).value).toBe('');
    expect((el.querySelector('[data-field="customfield_12001"]') as HTMLInputElement).value).toBe('');
    expect(shown(el, '[data-field="customfield_12002"]')).toBe('Pilih…');
  });

  it('clears dynamic inputs on a type switch and sends no parentKey for a type without parent', () => {
    const f = render();
    const el: HTMLElement = f.nativeElement;
    const m = meta.types[0];
    f.componentRef.setInput('meta', { ...meta, types: [{ ...m, parentAllowed: false }, meta.types[1]] });
    f.detectChanges();
    fillAll(f);
    setIn(el, '[data-field="customfield_12001"]', 'x');
    choose(f, '[data-type]', 'Weird');
    choose(f, '[data-type]', 'Story');
    expect((el.querySelector('[data-field="customfield_12001"]') as HTMLInputElement).value).toBe('');
    setIn(el, '[data-summary]', 'S'); setIn(el, '[data-field="customfield_12001"]', 'mr'); choose(f, '[data-field="customfield_12002"]', 'OMS SERVICE');
    f.detectChanges();
    (el.querySelector('[data-submit]') as HTMLButtonElement).click();
    const req = http.expectOne((r) => r.url.endsWith('/create/issue'));
    expect(req.request.body.parentKey).toBeNull();
    req.flush({ key: 'OMS-2', url: 'u' });
  });

  it('renders a Jira 422 field error under Judul', () => {
    const f = render();
    const el: HTMLElement = f.nativeElement;
    fillAll(f);
    setIn(el, '[data-summary]', 'S'); setIn(el, '[data-field="customfield_12001"]', 'mr'); choose(f, '[data-field="customfield_12002"]', 'OMS SERVICE');
    f.detectChanges();
    (el.querySelector('[data-submit]') as HTMLButtonElement).click();
    http.expectOne((r) => r.url.endsWith('/create/issue')).flush(
      { error: 'jira_rejected', fields: { summary: 'Summary terlalu panjang' }, messages: [] }, { status: 422, statusText: 'Unprocessable' });
    f.detectChanges();
    expect(el.querySelector('.cc-err')?.textContent).toContain('Summary terlalu panjang');
    expect((el.querySelector('[data-submit]') as HTMLButtonElement).disabled).toBeFalse();
  });

  it('shows unslotted Jira field errors as a message and never the raw jira_rejected', () => {
    const f = render();
    const el: HTMLElement = f.nativeElement;
    fillAll(f);
    setIn(el, '[data-summary]', 'S'); setIn(el, '[data-field="customfield_12001"]', 'mr'); choose(f, '[data-field="customfield_12002"]', 'OMS SERVICE');
    f.detectChanges();
    (el.querySelector('[data-submit]') as HTMLButtonElement).click();
    http.expectOne((r) => r.url.endsWith('/create/issue')).flush(
      { error: 'jira_rejected', fields: { customfield_10016: 'Number value expected' }, messages: [] }, { status: 422, statusText: 'Unprocessable' });
    f.detectChanges();
    expect(el.textContent).toContain('customfield_10016: Number value expected');
    expect(el.textContent).not.toContain('jira_rejected');
  });

  it('toggles multi-select options as chips', () => {
    const f = render();
    const el: HTMLElement = f.nativeElement;
    f.componentRef.setInput('meta', { ...meta, types: [{ ...meta.types[0], fields: [
      { id: 'cf_multi', name: 'Labels', kind: 'select', multi: true, required: true, options: [{ id: 'a', label: 'A' }, { id: 'b', label: 'B' }] },
    ] }] });
    f.detectChanges();
    choose(f, '[data-type]', 'Story');
    const chip = (id: string) => el.querySelector(`[data-field="cf_multi"] [data-option="${id}"]`) as HTMLButtonElement;
    chip('a').click(); chip('b').click(); chip('a').click(); f.detectChanges();
    expect(chip('a').classList).not.toContain('on');
    expect(chip('b').getAttribute('aria-pressed')).toBe('true');
    setIn(el, '[data-summary]', 'S'); f.detectChanges();
    (el.querySelector('[data-submit]') as HTMLButtonElement).click();
    const req = http.expectOne((r) => r.url.endsWith('/create/issue'));
    expect(req.request.body.fields).toEqual({ cf_multi: ['b'] });
    req.flush({ key: 'OMS-3', url: 'u' });
  });

  it('shows an empty state when the project has no creatable types', () => {
    TestBed.configureTestingModule({ imports: [SingleCardFormComponent], providers: [provideHttpClient(), provideHttpClientTesting()] });
    const f = TestBed.createComponent(SingleCardFormComponent);
    f.componentRef.setInput('boardId', 28);
    f.componentRef.setInput('meta', { projectKey: 'OMS', subtaskTypeId: '', types: null } as unknown as CreateMeta);
    f.detectChanges();
    expect(f.nativeElement.textContent).toContain('Tidak ada tipe card yang bisa kamu buat di project ini.');
    expect(f.nativeElement.querySelector('[data-type]')).toBeNull();
  });

  it('disables submit for an unsupported type and shows Jira field errors', () => {
    const f = render();
    const el: HTMLElement = f.nativeElement;
    choose(f, '[data-type]', 'Weird');
    expect((el.querySelector('[data-submit]') as HTMLButtonElement).disabled).toBeTrue();
    expect(el.textContent).toContain('Tidak didukung');
  });

  it('lays out main fields and a Detail panel, with a hint while submit is blocked', () => {
    const f = render();
    const el: HTMLElement = f.nativeElement;
    expect(el.querySelector('[data-type-empty]')).not.toBeNull();
    choose(f, '[data-type]', 'Story');
    expect(el.querySelector('.scf-main [data-summary]')).not.toBeNull();
    expect(el.querySelector('.scf-side [data-field="customfield_12001"]')).not.toBeNull();
    expect(el.querySelector('.scf-side [data-parent-key]')).withContext('Story keeps the plain Epic key input').not.toBeNull();
    expect(el.querySelector('[data-submit-hint]')?.textContent).toContain('Isi judul dulu');
  });

  it('uses the parent picker for a subtask type and requires a parent', () => {
    const f = render();
    const el: HTMLElement = f.nativeElement;
    f.componentRef.setInput('meta', { ...meta, types: [...meta.types, { id: '10143', name: 'SUB Story', subtask: true, supported: true, parentAllowed: true, fields: [] }] });
    f.detectChanges();
    choose(f, '[data-type]', 'SUB Story');
    expect(el.querySelector('[data-parent-key]')).toBeNull();
    setIn(el, '[data-summary]', 'Sub'); f.detectChanges();
    expect((el.querySelector('[data-submit]') as HTMLButtonElement).disabled).toBeTrue();
    expect(el.querySelector('[data-submit-hint]')?.textContent).toContain('Pilih parent dulu');
    const input = el.querySelector('[data-parent-field] [data-parent-input]') as HTMLInputElement;
    input.value = 'OMS-5'; input.dispatchEvent(new Event('input'));
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' }));
    http.expectOne((r) => r.url.endsWith('/create/parent/OMS-5')).flush({ key: 'OMS-5', summary: 'Parent story', subtasks: [] });
    f.detectChanges();
    (el.querySelector('[data-submit]') as HTMLButtonElement).click();
    const req = http.expectOne((r) => r.url.endsWith('/create/issue'));
    expect(req.request.body).toEqual(jasmine.objectContaining({ typeId: '10143', parentKey: 'OMS-5' }));
    req.flush({ key: 'OMS-6', url: 'u' });
  });

  describe('field kinds', () => {
    const kinds: CreateMeta = { projectKey: 'OMS', subtaskTypeId: '', types: [{ id: '10206', name: 'Bug', subtask: false, supported: true, parentAllowed: false, fields: [
      { id: 'cf_mr', name: 'Merge Request', kind: 'textarea', multi: false, required: true },
      { id: 'cf_service', name: 'Service', kind: 'cascade', multi: false, required: true, options: [
        { id: '10200', label: 'Order', children: [{ id: '11099', label: 'ESO' }, { id: '11100', label: 'Kiosk' }] },
        { id: '10201', label: 'Other' },
      ] },
      { id: 'duedate', name: 'Due date', kind: 'date', multi: false, required: false },
      { id: 'labels', name: 'Labels', kind: 'labels', multi: true, required: false },
      { id: 'versions', name: 'Affects versions', kind: 'select', multi: true, required: false,
        options: Array.from({ length: 20 }, (_, i) => ({ id: `v${i}`, label: `POS v.2.${100 + i}.0` })) },
    ] }] };
    function renderKinds() {
      const f = render();
      f.componentRef.setInput('meta', kinds);
      f.detectChanges();
      choose(f, '[data-type]', 'Bug');
      return f;
    }

    it('puts textarea fields in the main column and the rest in Detail', () => {
      const el: HTMLElement = renderKinds().nativeElement;
      expect(el.querySelector('.scf-main textarea[data-field="cf_mr"]')).not.toBeNull();
      expect(el.querySelector('.scf-side [data-field="cf_mr"]')).toBeNull();
      expect(el.querySelector('.scf-side [data-field="cf_service"]')).not.toBeNull();
      expect(el.querySelector('.scf-side app-date-range-picker[data-field="duedate"]')).not.toBeNull();
      expect(el.textContent).toContain('Merge Request *');
      expect(el.querySelector('select')).toBeNull();
    });

    it('cascade: child select appears for a parent with children and resets when the parent changes', async () => {
      const f = renderKinds();
      const el: HTMLElement = f.nativeElement;
      expect(el.querySelector('[data-cascade-child]')).toBeNull();
      choose(f, '[data-cascade-parent]', 'Order');
      expect(el.querySelector('[data-cascade-child]')).not.toBeNull();
      choose(f, '[data-cascade-child]', 'Kiosk');
      choose(f, '[data-cascade-parent]', 'Other');
      expect(el.querySelector('[data-cascade-child]')).withContext('Other has no children').toBeNull();
      choose(f, '[data-cascade-parent]', 'Order');
      await f.whenStable(); f.detectChanges();
      expect(shown(el, '[data-cascade-child]')).toBe('Pilih…');
      choose(f, '[data-cascade-child]', 'ESO');
      setIn(el, '[data-summary]', 'Bug'); setIn(el, 'textarea[data-field="cf_mr"]', 'https://mr'); f.detectChanges();
      (el.querySelector('[data-submit]') as HTMLButtonElement).click();
      const req = http.expectOne((r) => r.url.endsWith('/create/issue'));
      expect(req.request.body.fields).toEqual({ cf_service: { id: '10200', child: '11099' }, cf_mr: 'https://mr' });
      req.flush({ key: 'OMS-9', url: 'u' });
    });

    it('cascade: the parent alone satisfies a required field', () => {
      const f = renderKinds();
      const el: HTMLElement = f.nativeElement;
      setIn(el, '[data-summary]', 'Bug'); setIn(el, 'textarea[data-field="cf_mr"]', 'mr'); f.detectChanges();
      expect(el.querySelector('[data-submit-hint]')?.textContent).toContain('Service wajib diisi');
      choose(f, '[data-cascade-parent]', 'Order');
      expect((el.querySelector('[data-submit]') as HTMLButtonElement).disabled).toBeFalse();
    });

    it('date: the single-date picker sets YYYY-MM-DD and Hapus clears it', () => {
      const f = renderKinds();
      const el: HTMLElement = f.nativeElement;
      const trigger = () => el.querySelector('[data-field="duedate"] .ui-date-trigger') as HTMLButtonElement;
      trigger().click(); f.detectChanges();
      expect(el.querySelector('[data-field="duedate"] [data-done]')).withContext('single mode has no Selesai').toBeNull();
      const day = el.querySelector('[data-field="duedate"] .ui-date-cell:not(.outside)') as HTMLButtonElement;
      const date = day.getAttribute('data-day')!;
      day.click(); f.detectChanges();
      expect(el.querySelector('[data-field="duedate"] .ui-date-popup')).toBeNull();
      expect(el.querySelector('[data-field="duedate"] .ui-date-count')).toBeNull();
      expect(date).toMatch(/^\d{4}-\d{2}-\d{2}$/);
      trigger().click(); f.detectChanges();
      (el.querySelector('[data-field="duedate"] [data-clear]') as HTMLButtonElement).click(); f.detectChanges();
      expect(el.querySelector('[data-field="duedate"] .ui-select-value')?.textContent?.trim()).toBe('Pilih tanggal');
      trigger().click(); f.detectChanges();
      (el.querySelector(`[data-field="duedate"] [data-day="${date}"]`) as HTMLButtonElement).click(); f.detectChanges();
      choose(f, '[data-cascade-parent]', 'Other');
      setIn(el, '[data-summary]', 'Bug'); setIn(el, 'textarea[data-field="cf_mr"]', 'mr'); f.detectChanges();
      (el.querySelector('[data-submit]') as HTMLButtonElement).click();
      const req = http.expectOne((r) => r.url.endsWith('/create/issue'));
      expect(req.request.body.fields.duedate).toBe(date);
      req.flush({ key: 'OMS-10', url: 'u' });
    });

    it('labels: space or Enter commits a chip, x removes it, and a pending draft is sent', () => {
      const f = renderKinds();
      const el: HTMLElement = f.nativeElement;
      const input = () => el.querySelector('[data-field="labels"] [data-label-input]') as HTMLInputElement;
      setIn(el, '[data-field="labels"] [data-label-input]', 'backend '); f.detectChanges();
      expect(input().value).toBe('');
      setIn(el, '[data-field="labels"] [data-label-input]', 'hotfix');
      input().dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter' })); f.detectChanges();
      (el.querySelector('[data-field="labels"] .scf-label button') as HTMLButtonElement).click(); f.detectChanges();
      setIn(el, '[data-field="labels"] [data-label-input]', 'pending'); f.detectChanges();
      choose(f, '[data-cascade-parent]', 'Other');
      setIn(el, '[data-summary]', 'Bug'); setIn(el, 'textarea[data-field="cf_mr"]', 'mr'); f.detectChanges();
      (el.querySelector('[data-submit]') as HTMLButtonElement).click();
      const req = http.expectOne((r) => r.url.endsWith('/create/issue'));
      expect(req.request.body.fields.labels).toEqual(['hotfix', 'pending']);
      req.flush({ key: 'OMS-11', url: 'u' });
    });

    it('long multi-select lists get a search box and a scrollable wrap', () => {
      const f = renderKinds();
      const el: HTMLElement = f.nativeElement;
      expect(el.querySelector('[data-field="versions"]')?.classList).toContain('scf-scroll');
      expect(el.querySelectorAll('[data-field="versions"] .cc-opt').length).toBe(20);
      (el.querySelector('[data-field="versions"] [data-option="v3"]') as HTMLButtonElement).click(); f.detectChanges();
      setIn(el, '[data-filter="versions"]', '2.115'); f.detectChanges();
      const visible = Array.from(el.querySelectorAll('[data-field="versions"] .cc-opt')).map((o) => o.getAttribute('data-option'));
      expect(visible).withContext('the match plus what is already picked').toEqual(['v3', 'v15']);
      expect(el.textContent).toContain('1 dipilih');
      setIn(el, '[data-filter="versions"]', 'zzz'); f.detectChanges();
      expect(el.querySelectorAll('[data-field="versions"] .cc-opt').length).toBe(1);
    });
  });
});
