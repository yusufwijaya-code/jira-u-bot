import { AfterViewChecked, Component, ElementRef, HostListener, computed, inject, input, output, signal } from '@angular/core';
import { IconComponent } from './icon.component';

const pad = (n: number) => String(n).padStart(2, '0');

/**
 * Styled "HH:MM" picker replacing <input type="time"> (whose popup can't be
 * styled): two scrollable columns, hours 00–23 and minutes in `step`
 * increments. The current value is kept even if it isn't on the step grid.
 */
@Component({
  selector: 'app-time-picker',
  standalone: true,
  imports: [IconComponent],
  host: { class: 'ui-time', '[class.open]': 'open()' },
  template: `
    <button
      type="button"
      class="ui-select-trigger ui-time-trigger"
      aria-haspopup="dialog"
      [attr.aria-expanded]="open()"
      [attr.aria-label]="ariaLabel() || null"
      (click)="toggle()"
      (keydown.escape)="open.set(false)"
    >
      <app-icon name="clock" [size]="15" />
      <span class="ui-select-value">{{ value() || '--:--' }}</span>
      <app-icon class="ui-select-chevron" name="chevron-down" [size]="16" />
    </button>
    @if (open()) {
      <div class="ui-time-popup" role="dialog" [attr.aria-label]="ariaLabel() || 'Pilih jam'">
        <div class="ui-time-cols">
          <div class="ui-time-col" role="listbox" aria-label="Jam">
            <span class="ui-time-head">Jam</span>
            @for (h of hours; track h) {
              <button type="button" class="ui-time-cell" [class.selected]="h === hour()" (click)="setHour(h)">{{ h }}</button>
            }
          </div>
          <div class="ui-time-col" role="listbox" aria-label="Menit">
            <span class="ui-time-head">Menit</span>
            @for (m of minutes(); track m) {
              <button type="button" class="ui-time-cell" [class.selected]="m === minute()" (click)="setMinute(m)">{{ m }}</button>
            }
          </div>
        </div>
        <div class="ui-time-footer">
          <span>{{ hour() }}:{{ minute() }} WIB</span>
          <button type="button" class="btn btn-primary btn-sm" (click)="open.set(false)">Selesai</button>
        </div>
      </div>
    }
  `,
})
export class TimePickerComponent implements AfterViewChecked {
  value = input<string>('');
  step = input<number>(5);
  ariaLabel = input<string>('');
  valueChange = output<string>();

  protected open = signal(false);
  protected readonly hours = Array.from({ length: 24 }, (_, h) => pad(h));
  protected hour = computed(() => (this.value() || '00:00').slice(0, 2));
  protected minute = computed(() => (this.value() || '00:00').slice(3, 5));
  protected minutes = computed(() => {
    const grid = Array.from({ length: Math.ceil(60 / this.step()) }, (_, i) => pad(i * this.step()));
    return grid.includes(this.minute()) ? grid : [...grid, this.minute()].sort();
  });

  private host = inject(ElementRef<HTMLElement>);
  private scrollPending = false;

  protected toggle(): void {
    this.open.set(!this.open());
    this.scrollPending = this.open();
  }

  protected setHour(h: string): void {
    this.valueChange.emit(`${h}:${this.minute()}`);
  }

  protected setMinute(m: string): void {
    this.valueChange.emit(`${this.hour()}:${m}`);
  }

  /** Bring the selected cells into view when the popup opens. */
  ngAfterViewChecked(): void {
    if (!this.scrollPending) return;
    this.scrollPending = false;
    for (const cell of this.host.nativeElement.querySelectorAll('.ui-time-cell.selected')) {
      const el = cell as HTMLElement;
      const col = el.parentElement;
      if (col) col.scrollTop = el.offsetTop - col.clientHeight / 2 + el.clientHeight / 2;
    }
  }

  @HostListener('document:pointerdown', ['$event'])
  protected onOutside(event: Event): void {
    if (this.open() && !(event.target instanceof Node && this.host.nativeElement.contains(event.target))) {
      this.open.set(false);
    }
  }
}
