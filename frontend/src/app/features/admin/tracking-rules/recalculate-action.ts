import { HttpErrorResponse } from '@angular/common/http';
import { TrackingRuleService } from '../../../core/services/tracking-rule.service';
import { ToastAction, ToastService } from '../../../core/ui/toast.service';

export const RECALCULATE_LABEL = 'Hitung ulang semua user';

/** Toast action after a rule change (AC3.4): rebuilds every user's active-sprint time from Jira. */
export function recalculateAction(api: TrackingRuleService, toast: ToastService): ToastAction {
  return {
    label: RECALCULATE_LABEL,
    run: () => {
      api.recalculate().subscribe({
        next: () => toast.info('Hitung ulang berjalan', 'Waktu sprint aktif semua user dihitung ulang dari changelog Jira di background.'),
        error: (err: HttpErrorResponse) =>
          err?.status === 409
            ? toast.info('Hitung ulang masih berjalan', 'Tunggu sampai selesai, lalu coba lagi.')
            : toast.error('Gagal memulai hitung ulang', 'Coba lagi sebentar.'),
      });
    },
  };
}
