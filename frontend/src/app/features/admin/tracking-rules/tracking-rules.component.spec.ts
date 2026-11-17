import { ComponentFixture, TestBed } from '@angular/core/testing';
import { provideRouter } from '@angular/router';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting } from '@angular/common/http/testing';
import { of, throwError } from 'rxjs';
import { TrackingRulesComponent } from './tracking-rules.component';
import { TrackingRule, TrackingRuleService } from '../../../core/services/tracking-rule.service';
import { ConfirmService } from '../../../core/ui/confirm.service';
import { ToastService } from '../../../core/ui/toast.service';

const CORE: TrackingRule = {
  projectKey: 'CORE', projectName: 'ESB Core', trackedStatuses: ['Code Process', 'CODE REVIEW'], doneStatus: 'Code Completed', reviewStatus: 'CODE REVIEW',
  storyPointFields: [{ id: 'customfield_10125', name: 'Story Point Estimate' }, { id: 'customfield_11053', name: 'Story Points' }],
  storyPointWriteField: 'customfield_10125', storyPointWriteFieldSubtask: 'customfield_11053', isActive: true, editedBy: 'seed', editedDate: '2026-11-16T09:00:00+07:00',
};
const OMS: TrackingRule = {
  ...CORE, projectKey: 'OMS', projectName: 'ESB OMS', trackedStatuses: ['Code Process'], doneStatus: 'Code Review', reviewStatus: null,
  storyPointFields: [{ id: 'customfield_10016', name: 'Story point estimate' }], storyPointWriteField: 'customfield_10016', storyPointWriteFieldSubtask: 'customfield_10016',
  createFields: { '10206': ['duedate', 'priority'], '10249': ['duedate'], '10143': [] },
};
const POS: TrackingRule = { ...OMS, projectKey: 'POS', projectName: 'ESB POS', isActive: false };

describe('TrackingRulesComponent', () => {
  let fixture: ComponentFixture<TrackingRulesComponent>;
  let api: jasmine.SpyObj<TrackingRuleService>;
  let confirm: jasmine.SpyObj<ConfirmService>;
  let toast: jasmine.SpyObj<ToastService>;
  const el = () => fixture.nativeElement as HTMLElement;
  const component = () => fixture.componentInstance as any;

  beforeEach(async () => {
    api = jasmine.createSpyObj('TrackingRuleService', ['list', 'setActive', 'recalculate']);
    api.list.and.returnValue(of({ rules: [CORE, OMS, POS] }));
    confirm = jasmine.createSpyObj('ConfirmService', ['ask']);
    toast = jasmine.createSpyObj('ToastService', ['show', 'success', 'error', 'info']);
    await TestBed.configureTestingModule({
      imports: [TrackingRulesComponent],
      providers: [
        provideRouter([]),
        provideHttpClient(),
        provideHttpClientTesting(),
        { provide: TrackingRuleService, useValue: api },
        { provide: ConfirmService, useValue: confirm },
        { provide: ToastService, useValue: toast },
      ],
    }).compileComponents();
    fixture = TestBed.createComponent(TrackingRulesComponent);
    fixture.detectChanges();
  });

  it('lists every rule with its statuses and Story Point fields in priority order (AC2.2)', () => {
    const rows = el().querySelectorAll('[data-rule]');
    expect(rows.length).toBe(3);
    const core = el().querySelector('[data-rule="CORE"]')!;
    for (const text of ['ESB Core', 'Code Process', 'CODE REVIEW', 'Code Completed', 'Story Point Estimate', 'customfield_10125', 'seed']) {
      expect(core.textContent).toContain(text);
    }
    const order = Array.from(core.querySelectorAll('[data-sp]')).map((li) => li.getAttribute('data-sp'));
    expect(order).toEqual(['customfield_10125', 'customfield_11053']);
    expect(el().querySelector('[data-add-rule]')?.getAttribute('href')).toBe('/admin/tracking-rules/new');
  });

  it('summarises the extra Buat Card fields of a rule', () => {
    expect(el().querySelector('[data-rule="OMS"] [data-create-fields]')?.textContent).toContain('3 field di 2 tipe card');
    expect(el().querySelector('[data-rule="CORE"] [data-create-fields]')).toBeNull();
  });

  it('labels the write field and the fallback per card type', () => {
    const core = el().querySelector('[data-rule="CORE"]')!;
    const tags = (id: string) => Array.from(core.querySelectorAll(`[data-sp="${id}"] [data-tag]`)).map((t) => t.getAttribute('data-tag'));
    expect(tags('customfield_10125')).toEqual(['card', 'subtask-backup']);
    expect(tags('customfield_11053')).toEqual(['card-backup', 'subtask']);
    const oms = el().querySelector('[data-rule="OMS"]')!;
    expect(Array.from(oms.querySelectorAll('[data-tag]')).map((t) => t.getAttribute('data-tag'))).toEqual(['card', 'subtask']);
  });

  it('shows the Jira project picture when a board avatar is known, else the key badge', () => {
    api.list.and.returnValue(of({ rules: [{ ...CORE, boardId: 26 }, OMS] }));
    component().load();
    fixture.detectChanges();
    const img = el().querySelector('[data-rule="CORE"] [data-avatar] img') as HTMLImageElement;
    expect(img?.getAttribute('src')).toContain('/boards/26/avatar');
    expect(el().querySelector('[data-rule="OMS"] .tr-key-badge')?.textContent).toContain('OMS');
  });

  it('dims an inactive rule and offers Aktifkan (AC2.4)', () => {
    const pos = el().querySelector('[data-rule="POS"]')!;
    expect(pos.classList).toContain('inactive');
    expect(pos.querySelector('[data-activate="POS"]')).not.toBeNull();
    expect(pos.textContent).toContain('Nonaktif');
  });

  it('asks before deactivating and toasts with the recalculation action (AC4.2, AC3.4)', async () => {
    confirm.ask.and.resolveTo(true);
    api.setActive.and.returnValue(of({ ...CORE, isActive: false }));
    api.recalculate.and.returnValue(of({ started: true }));

    await component().deactivate(CORE);
    fixture.detectChanges();

    expect(confirm.ask.calls.mostRecent().args[0].message).toBe('Tiket CORE berhenti dipantau. Riwayat tetap tersimpan.');
    expect(confirm.ask.calls.mostRecent().args[0].tone).toBe('danger');
    expect(api.setActive).toHaveBeenCalledWith('CORE', false);
    expect(el().querySelector('[data-rule="CORE"]')!.classList).toContain('inactive');
    const action = toast.show.calls.mostRecent().args[4]!;
    expect(action.label).toBe('Hitung ulang semua user');
    action.run();
    expect(api.recalculate).toHaveBeenCalled();
  });

  it('does nothing when the confirmation is cancelled', async () => {
    confirm.ask.and.resolveTo(false);
    await component().deactivate(CORE);
    expect(api.setActive).not.toHaveBeenCalled();
  });

  it('explains the last-active-rule refusal (AC4.3)', async () => {
    confirm.ask.and.resolveTo(true);
    api.setActive.and.returnValue(throwError(() => ({ status: 422, error: { error: 'last_active_rule' } })));
    await component().deactivate(CORE);
    expect(toast.error).toHaveBeenCalledWith('Minimal satu board harus aktif', jasmine.any(String));
  });
});
