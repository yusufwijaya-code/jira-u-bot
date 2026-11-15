import { fakeAsync, tick } from '@angular/core/testing';
import { ToastService } from './toast.service';

describe('ToastService action', () => {
  it('keeps the action and runs it once, dismissing the toast', fakeAsync(() => {
    const svc = new ToastService();
    const run = jasmine.createSpy('run');
    svc.show('success', 'Dipindah', 'A → B', 6000, { label: 'Batalkan', run });
    const t = svc.toasts()[0];
    expect(t.action?.label).toBe('Batalkan');
    svc.runAction(t.id);
    svc.runAction(t.id);
    expect(run).toHaveBeenCalledTimes(1);
    tick(6500);
    expect(svc.toasts().length).toBe(0);
  }));
});
