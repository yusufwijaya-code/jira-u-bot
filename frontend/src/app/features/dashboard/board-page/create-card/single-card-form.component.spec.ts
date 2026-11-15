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
});
