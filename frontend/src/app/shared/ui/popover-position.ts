/** Gap between the trigger and its popover (matches the CSS `100% + 6px`) plus breathing room. */
const GAP = 14;

/**
 * Whether a popover of `popupHeight` px should open above `trigger`: only when
 * it doesn't fit below inside the viewport and there is more room above.
 * Opening downward past the viewport would make the page taller and scroll.
 */
export function shouldDropUp(trigger: Element, popupHeight: number, viewportHeight = window.innerHeight): boolean {
  const rect = trigger.getBoundingClientRect();
  const below = viewportHeight - rect.bottom;
  const above = rect.top;
  return below < popupHeight + GAP && above > below;
}
