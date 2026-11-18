import { AfterViewChecked, AfterViewInit, Directive, ElementRef, OnDestroy, inject, input } from '@angular/core';

const UNSET = Symbol('unset');

/**
 * Sliding active indicator for a tab bar / segmented control: one pill
 * behind the items that moves and resizes (transform + width) to the item
 * carrying `.active`, instead of the active background jumping.
 *
 * `[appSegIndicator]="activeKey"` — a change of the key re-measures after
 * the view updated the `.active` class; host resizes re-measure too. The
 * first placement doesn't animate (`.seg-ready` comes one frame later).
 * Styles: `.seg`, `.seg-ind`, `.seg-on`, `.seg-ready` in styles.scss.
 */
@Directive({
  selector: '[appSegIndicator]',
  standalone: true,
  host: { class: 'seg' },
})
export class SegIndicatorDirective implements AfterViewInit, AfterViewChecked, OnDestroy {
  appSegIndicator = input<unknown>();

  private host: HTMLElement = inject(ElementRef<HTMLElement>).nativeElement;
  private indicator?: HTMLElement;
  private placedKey: unknown = UNSET;
  private resize?: ResizeObserver;
  private readyFrame = 0;

  ngAfterViewInit(): void {
    const ind = document.createElement('span');
    ind.className = 'seg-ind';
    ind.setAttribute('aria-hidden', 'true');
    this.host.insertBefore(ind, this.host.firstChild);
    this.indicator = ind;
    if (typeof ResizeObserver === 'function') {
      this.resize = new ResizeObserver(() => this.place());
      this.resize.observe(this.host);
    }
  }

  ngAfterViewChecked(): void {
    const key = this.appSegIndicator();
    if (key === this.placedKey) return;
    this.placedKey = key;
    this.place();
  }

  ngOnDestroy(): void {
    this.resize?.disconnect();
    cancelAnimationFrame(this.readyFrame);
    this.indicator?.remove();
  }

  /** Moves the pill onto the `.active` item (hidden when none is active). */
  place(): void {
    const active = this.host.querySelector<HTMLElement>(':scope > .active');
    if (!this.indicator || !active || !active.offsetWidth) {
      this.host.classList.remove('seg-on');
      return;
    }
    const style = this.host.style;
    style.setProperty('--seg-x', `${active.offsetLeft}px`);
    style.setProperty('--seg-y', `${active.offsetTop}px`);
    style.setProperty('--seg-w', `${active.offsetWidth}px`);
    style.setProperty('--seg-h', `${active.offsetHeight}px`);
    this.host.classList.add('seg-on');
    if (!this.host.classList.contains('seg-ready') && !this.readyFrame) {
      this.readyFrame = requestAnimationFrame(() => this.host.classList.add('seg-ready'));
    }
  }
}
