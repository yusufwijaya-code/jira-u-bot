import { Component } from '@angular/core';
import { TestBed } from '@angular/core/testing';
import { Router, provideRouter } from '@angular/router';
import { scrollTopOnPageChange } from './scroll-top-on-page-change';

@Component({ standalone: true, template: '' })
class PageComponent {}

@Component({ standalone: true, template: '' })
class ShellComponent {
  constructor() {
    scrollTopOnPageChange();
  }
}

describe('scrollTopOnPageChange', () => {
  let router: Router;
  let scrollTo: jasmine.Spy;

  beforeEach(async () => {
    TestBed.configureTestingModule({
      providers: [provideRouter([{ path: 'a', component: PageComponent }, { path: 'b', component: PageComponent }])],
    });
    router = TestBed.inject(Router);
    await router.navigateByUrl('/a?board=1');
    TestBed.createComponent(ShellComponent);
    spyOnProperty(window, 'scrollY').and.returnValue(400);
    scrollTo = spyOn(window, 'scrollTo');
  });

  it('scrolls to the top when the page changes', async () => {
    await router.navigateByUrl('/b?board=1');
    expect(scrollTo).toHaveBeenCalledTimes(1);
  });

  it('keeps the scroll on a query-only navigation (board switch)', async () => {
    await router.navigateByUrl('/a?board=26');
    expect(scrollTo).not.toHaveBeenCalled();
  });
});
