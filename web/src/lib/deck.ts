// Deck is the slide position of a Focus deck: which slide is up, and the
// moves a remote, a keyboard or a timer can make. Pure, so it's testable.
export type Deck = { index: number; count: number; playing: boolean };

export const AUTO_ADVANCE_MS = 9000;

export function next(d: Deck): Deck {
  return d.count === 0 ? d : { ...d, index: (d.index + 1) % d.count };
}

export function previous(d: Deck): Deck {
  return d.count === 0 ? d : { ...d, index: (d.index - 1 + d.count) % d.count };
}

export function jump(d: Deck, index: number): Deck {
  return index >= 0 && index < d.count ? { ...d, index } : d;
}

// A key press on the deck. Space pauses; arrows move and keep the timer
// running; Home and End jump.
export function onKey(d: Deck, key: string): Deck {
  switch (key) {
    case "ArrowRight":
    case "PageDown":
      return next(d);
    case "ArrowLeft":
    case "PageUp":
      return previous(d);
    case "Home":
      return jump(d, 0);
    case "End":
      return jump(d, d.count - 1);
    case " ":
      return { ...d, playing: !d.playing };
    default:
      return d;
  }
}
