import { Component, ElementRef, HostListener, computed, forwardRef, inject, input, signal } from '@angular/core';
import { ControlValueAccessor, NG_VALUE_ACCESSOR } from '@angular/forms';
import { IconComponent } from './icon.component';

export interface SelectOption<T = unknown> {
  value: T;
  label: string;
}

/**
 * Styled dropdown replacing the native <select> (whose popup list can't be
 * styled). Works with [(ngModel)] / formControl; keyboard: ↑ ↓ Home End to
 * move, Enter / Space to pick, Esc to close. Closes on any outside click.
 */
@Component({
  selector: 'app-select',
  standalone: true,
  imports: [IconComponent],
  providers: [{ provide: NG_VALUE_ACCESSOR, useExisting: forwardRef(() => SelectComponent), multi: true }],
  host: { class: 'ui-select', '[class.open]': 'open()', '[class.disabled]': 'disabled()' },
  template: `
    <button
      type="button"
      class="ui-select-trigger"
      role="combobox"
      aria-haspopup="listbox"
      [attr.aria-expanded]="open()"
      [attr.aria-label]="ariaLabel() || null"
      [disabled]="disabled()"
      (click)="toggle()"
      (keydown)="onKeydown($event)"
    >
      <span class="ui-select-value">{{ selectedLabel() }}</span>
      <app-icon class="ui-select-chevron" name="chevron-down" [size]="16" />
    </button>
    @if (open()) {
      <ul class="ui-select-list" role="listbox">
        @for (opt of options(); track $index; let i = $index) {
          <li
            role="option"
            class="ui-select-option"
            [class.selected]="isSelected(opt)"
            [class.active]="i === activeIndex()"
            [attr.aria-selected]="isSelected(opt)"
            (mouseenter)="activeIndex.set(i)"
            (mousedown)="$event.preventDefault()"
            (click)="pick(opt)"
          >
            <span>{{ opt.label }}</span>
            @if (isSelected(opt)) {
              <app-icon name="check" [size]="14" />
            }
          </li>
        }
      </ul>
    }
  `,
})
export class SelectComponent<T = unknown> implements ControlValueAccessor {
  options = input.required<SelectOption<T>[]>();
  ariaLabel = input<string>('');

  protected open = signal(false);
  protected disabled = signal(false);
  protected activeIndex = signal(0);
  private value = signal<T | null>(null);
  private host = inject(ElementRef<HTMLElement>);

  private onChange: (value: T) => void = () => undefined;
  private onTouched: () => void = () => undefined;

  protected selectedLabel = computed(() => this.options().find((o) => o.value === this.value())?.label ?? '');

  protected isSelected(opt: SelectOption<T>): boolean {
    return opt.value === this.value();
  }

  protected toggle(): void {
    if (this.open()) {
      this.close();
      return;
    }
    const current = this.options().findIndex((o) => o.value === this.value());
    this.activeIndex.set(Math.max(0, current));
    this.open.set(true);
  }

  protected pick(opt: SelectOption<T>): void {
    this.value.set(opt.value);
    this.onChange(opt.value);
    this.close();
  }

  protected onKeydown(event: KeyboardEvent): void {
    const count = this.options().length;
    if (!count) return;
    switch (event.key) {
      case 'ArrowDown':
      case 'ArrowUp': {
        event.preventDefault();
        if (!this.open()) {
          this.toggle();
          return;
        }
        const step = event.key === 'ArrowDown' ? 1 : -1;
        this.activeIndex.set((this.activeIndex() + step + count) % count);
        return;
      }
      case 'Home':
        event.preventDefault();
        this.activeIndex.set(0);
        return;
      case 'End':
        event.preventDefault();
        this.activeIndex.set(count - 1);
        return;
      case 'Enter':
      case ' ':
        if (this.open()) {
          event.preventDefault();
          this.pick(this.options()[this.activeIndex()]);
        }
        return;
      case 'Escape':
      case 'Tab':
        this.close();
        return;
    }
  }

  @HostListener('document:pointerdown', ['$event'])
  protected onOutside(event: Event): void {
    if (this.open() && !(event.target instanceof Node && this.host.nativeElement.contains(event.target))) {
      this.close();
    }
  }

  private close(): void {
    this.open.set(false);
    this.onTouched();
  }

  writeValue(value: T): void {
    this.value.set(value);
  }

  registerOnChange(fn: (value: T) => void): void {
    this.onChange = fn;
  }

  registerOnTouched(fn: () => void): void {
    this.onTouched = fn;
  }

  setDisabledState(disabled: boolean): void {
    this.disabled.set(disabled);
  }
}
