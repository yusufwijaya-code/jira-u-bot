import { DestroyRef, inject } from '@angular/core';
import { NavigationEnd, Router } from '@angular/router';
import { filter } from 'rxjs';

/**
 * Opens every page at the top (call from a shell's injection context). The
 * router keeps the old scroll offset by default, so arriving from a scrolled
 * page landed mid-page and the new page jumped as it grew. Only a change of
 * PATH counts: query-only navigations (?board= from the board picker) keep
 * the scroll where it is.
 */
export function scrollTopOnPageChange(): void {
  const router = inject(Router);
  let path = pathOf(router.url);
  const sub = router.events.pipe(filter((e): e is NavigationEnd => e instanceof NavigationEnd)).subscribe((e) => {
    const next = pathOf(e.urlAfterRedirects);
    if (next === path) return;
    path = next;
    if (typeof window !== 'undefined' && window.scrollY > 0) {
      window.scrollTo({ top: 0, left: 0, behavior: 'instant' as ScrollBehavior });
    }
  });
  inject(DestroyRef).onDestroy(() => sub.unsubscribe());
}

function pathOf(url: string): string {
  return url.split(/[?#]/)[0];
}
