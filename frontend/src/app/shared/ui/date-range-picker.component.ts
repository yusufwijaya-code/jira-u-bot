import { Component, ElementRef, HostListener, computed, inject, input, output, signal } from '@angular/core';
import { IconComponent } from './icon.component';
import { shouldDropUp } from './popover-position';

/** Rendered height of the calendar popup (header, 6 week rows, footer). */
const POPUP_HEIGHT = 400;

const MONTHS = ['Januari', 'Februari', 'Maret', 'April', 'Mei', 'Juni', 'Juli', 'Agustus', 'September', 'Oktober', 'November', 'Desember'];
const MONTHS_SHORT = ['Jan', 'Feb', 'Mar', 'Apr', 'Mei', 'Jun', 'Jul', 'Agu', 'Sep', 'Okt', 'Nov', 'Des'];
const DAYS_SHORT = ['Min', 'Sen', 'Sel', 'Rab', 'Kam', 'Jum', 'Sab'];
/** Monday-first header, as on Indonesian calendars. */
const WEEK_HEAD = ['Sen', 'Sel', 'Rab', 'Kam', 'Jum', 'Sab', 'Min'];

const pad = (n: number) => String(n).padStart(2, '0');
const iso = (y: number, m: number, d: number) => `${y}-${pad(m + 1)}-${pad(d)}`;

/** Today in WIB as 2006-01-02. */
export function todayISO(): string {
  return new Date(Date.now() + 7 * 3600_000).toISOString().slice(0, 10);
}

function parts(date: string): [number, number, number] {
  const [y, m, d] = date.split('-').map(Number);
  return [y, m, d];
}

/** "2026-11-18" → "Rab, 18 Nov 2026". */
export function shortDateLabel(date: string): string {
  const [y, m, d] = parts(date);
  return `${DAYS_SHORT[new Date(Date.UTC(y, m - 1, d)).getUTCDay()]}, ${d} ${MONTHS_SHORT[m - 1]} ${y}`;
}

/** "Rab, 18 – Jum, 20 Nov 2026" (year/month only once when shared). */
export function rangeLabel(from: string, to: string): string {
  if (!to || to === from) return shortDateLabel(from);
  const [fy, fm, fd] = parts(from);
  const [ty, tm] = parts(to);
  const fromDay = DAYS_SHORT[new Date(Date.UTC(fy, fm - 1, fd)).getUTCDay()];
  if (fy !== ty) return `${shortDateLabel(from)} – ${shortDateLabel(to)}`;
  const head = fm === tm ? `${fromDay}, ${fd}` : `${fromDay}, ${fd} ${MONTHS_SHORT[fm - 1]}`;
  return `${head} – ${shortDateLabel(to)}`;
}

/** Calendar days in from..to, inclusive. */
export function daysInRange(from: string, to: string): number {
  const [fy, fm, fd] = parts(from);
  const [ty, tm, td] = parts(to || from);
  return Math.round((Date.UTC(ty, tm - 1, td) - Date.UTC(fy, fm - 1, fd)) / 86400_000) + 1;
}

export interface DateRange {
  from: string;
  /** '' = a single day. */
  to: string;
}

interface DayCell {
  date: string;
  day: number;
  outside: boolean;
  weekend: boolean;
}

/**
 * One-field date range picker (replaces two <input type="date">, whose popup
 * can't be styled). First click picks the start, second click the end; a
 * single day is one click + "Selesai". Dates are "YYYY-MM-DD".
 */
@Component({
  selector: 'app-date-range-picker',
  standalone: true,
  imports: [IconComponent],
  host: { class: 'ui-date', '[class.open]': 'open()', '[class.drop-up]': 'open() && dropUp()' },
  template: `
    <button
      type="button"
      class="ui-select-trigger ui-date-trigger"
      aria-haspopup="dialog"
      [attr.aria-expanded]="open()"
      [attr.aria-label]="ariaLabel() || null"
      (click)="toggle()"
      (keydown.escape)="close()"
    >
      <app-icon name="calendar" [size]="15" />
      <span class="ui-select-value" [class.placeholder]="!from()">{{ from() ? label(from(), to()) : placeholder() }}</span>
      @if (from()) {
        <span class="ui-date-count">{{ count(from(), to()) }} hari</span>
      }
      <app-icon class="ui-select-chevron" name="chevron-down" [size]="16" />
    </button>
    @if (open()) {
      <div class="ui-date-popup" role="dialog" [attr.aria-label]="ariaLabel() || 'Pilih rentang tanggal'" (keydown.escape)="close()">
        <div class="ui-date-head">
          <button type="button" class="ui-date-nav" aria-label="Bulan sebelumnya" data-prev (click)="shift(-1)">
            <app-icon name="chevron-left" [size]="16" />
          </button>
          <span class="ui-date-title">{{ title() }}</span>
          <button type="button" class="ui-date-nav" aria-label="Bulan berikutnya" data-next (click)="shift(1)">
            <app-icon name="chevron-right" [size]="16" />
          </button>
        </div>
        <p class="ui-date-step">{{ picking() ? 'Pilih tanggal selesai' : 'Pilih tanggal mulai' }}</p>
        <div class="ui-date-grid" role="grid" (mouseleave)="hover.set('')">
          @for (h of weekHead; track h) {
            <span class="ui-date-weekday" [class.weekend]="h === 'Sab' || h === 'Min'">{{ h }}</span>
          }
          @for (cell of cells(); track cell.date) {
            <button
              type="button"
              class="ui-date-cell"
              [attr.data-day]="cell.date"
              [class.outside]="cell.outside"
              [class.weekend]="cell.weekend"
              [class.today]="cell.date === today"
              [class.in-range]="inRange(cell.date)"
              [class.selected]="isEdge(cell.date)"
              [disabled]="isDisabled(cell.date)"
              (mouseenter)="hover.set(cell.date)"
              (click)="pick(cell.date)"
            >
              {{ cell.day }}
            </button>
          }
        </div>
        <div class="ui-date-footer">
          <button type="button" class="ui-date-link" data-today [disabled]="isDisabled(today)" (click)="pickToday()">Hari ini</button>
          <button type="button" class="btn btn-primary btn-sm" data-done [disabled]="!draftFrom()" (click)="commit()">Selesai</button>
        </div>
      </div>
    }
  `,
})
export class DateRangePickerComponent {
  from = input<string>('');
  to = input<string>('');
  /** Earliest selectable date; '' = none. */
  min = input<string>('');
  max = input<string>('');
  placeholder = input<string>('Pilih tanggal');
  ariaLabel = input<string>('');
  rangeChange = output<DateRange>();

  protected readonly weekHead = WEEK_HEAD;
  protected readonly today = todayISO();
  protected label = rangeLabel;
  protected count = daysInRange;

  protected open = signal(false);
  /** Opens above the trigger when there's no room below in the viewport. */
  protected dropUp = signal(false);
  /** The range being picked inside the open popup. */
  protected draftFrom = signal('');
  protected draftTo = signal('');
  /** Start picked, waiting for the end. */
  protected picking = signal(false);
  protected hover = signal('');
  private view = signal({ y: 0, m: 0 });

  protected title = computed(() => `${MONTHS[this.view().m]} ${this.view().y}`);

  protected cells = computed<DayCell[]>(() => {
    const { y, m } = this.view();
    const lead = (new Date(Date.UTC(y, m, 1)).getUTCDay() + 6) % 7;
    const out: DayCell[] = [];
    for (let i = 0; i < 42; i++) {
      const d = new Date(Date.UTC(y, m, 1 - lead + i));
      const wd = d.getUTCDay();
      out.push({
        date: iso(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate()),
        day: d.getUTCDate(),
        outside: d.getUTCMonth() !== m,
        weekend: wd === 0 || wd === 6,
      });
    }
    return out.slice(35).every((c) => c.outside) ? out.slice(0, 35) : out;
  });

  /** The span to highlight: the draft, or start → hovered day while picking. */
  private span = computed<[string, string]>(() => {
    const from = this.draftFrom();
    let to = this.draftTo();
    if (this.picking() && this.hover() > from) to = this.hover();
    return [from, to && to > from ? to : from];
  });

  private host = inject(ElementRef<HTMLElement>);

  protected toggle(): void {
    if (this.open()) {
      this.close();
      return;
    }
    this.draftFrom.set(this.from());
    this.draftTo.set(this.to());
    this.picking.set(false);
    const [y, m] = parts(this.from() || this.today);
    this.view.set({ y, m: m - 1 });
    this.dropUp.set(shouldDropUp(this.host.nativeElement, POPUP_HEIGHT));
    this.open.set(true);
  }

  protected close(): void {
    this.open.set(false);
    this.hover.set('');
  }

  protected shift(months: number): void {
    const { y, m } = this.view();
    const d = new Date(Date.UTC(y, m + months, 1));
    this.view.set({ y: d.getUTCFullYear(), m: d.getUTCMonth() });
  }

  protected isDisabled(date: string): boolean {
    return (!!this.min() && date < this.min()) || (!!this.max() && date > this.max());
  }

  protected inRange(date: string): boolean {
    const [from, to] = this.span();
    return !!from && date > from && date < to;
  }

  protected isEdge(date: string): boolean {
    const [from, to] = this.span();
    return !!from && (date === from || date === to);
  }

  protected pick(date: string): void {
    if (!this.picking() || date < this.draftFrom()) {
      // New start (also when the second click lands before the start).
      this.draftFrom.set(date);
      this.draftTo.set('');
      this.picking.set(true);
      return;
    }
    this.draftTo.set(date === this.draftFrom() ? '' : date);
    this.commit();
  }

  protected pickToday(): void {
    this.draftFrom.set(this.today);
    this.draftTo.set('');
    this.commit();
  }

  protected commit(): void {
    if (!this.draftFrom()) return;
    this.rangeChange.emit({ from: this.draftFrom(), to: this.draftTo() });
    this.picking.set(false);
    this.close();
  }

  @HostListener('document:pointerdown', ['$event'])
  protected onOutside(event: Event): void {
    if (this.open() && !(event.target instanceof Node && this.host.nativeElement.contains(event.target))) {
      this.close();
    }
  }
}
