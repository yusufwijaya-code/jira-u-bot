import { DestroyRef, Directive, ElementRef, afterNextRender, inject } from '@angular/core';

/**
 * Publishes the navbar's real rendered height as `--topbar-actual` on <html>,
 * so sticky bars below it (e.g. the Pengaturan tabs) sit just under it even
 * when the heading wraps or the board picker makes it taller than --topbar-h.
 */
@Directive({ selector: '[appTopbarHeight]', standalone: true })
export class TopbarHeightDirective {
  constructor() {
    const el = inject<ElementRef<HTMLElement>>(ElementRef).nativeElement;
    const root = document.documentElement;
    const publish = () => root.style.setProperty('--topbar-actual', `${Math.ceil(el.getBoundingClientRect().height)}px`);
    let observer: ResizeObserver | undefined;
    afterNextRender(() => {
      publish();
      if (typeof ResizeObserver !== 'undefined') {
        observer = new ResizeObserver(publish);
        observer.observe(el);
      }
    });
    inject(DestroyRef).onDestroy(() => {
      observer?.disconnect();
      root.style.removeProperty('--topbar-actual');
    });
  }
}
