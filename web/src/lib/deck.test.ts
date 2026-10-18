import { describe, expect, it } from "vitest";
import { type Deck, jump, next, onKey, previous } from "./deck";

const deck: Deck = { index: 0, count: 5, playing: true };

describe("deck", () => {
  // Catches a wall screen that runs off the end of the story and goes blank.
  it("wraps at both ends", () => {
    expect(next({ ...deck, index: 4 }).index).toBe(0);
    expect(previous(deck).index).toBe(4);
  });

  // Catches a divide-by-zero or NaN index on a front with nothing to tell.
  it("stays put on an empty deck", () => {
    const empty: Deck = { index: 0, count: 0, playing: true };
    expect(next(empty)).toEqual(empty);
    expect(previous(empty)).toEqual(empty);
  });

  it("ignores a jump outside the deck", () => {
    expect(jump(deck, 9)).toEqual(deck);
    expect(jump(deck, -1)).toEqual(deck);
    expect(jump(deck, 3).index).toBe(3);
  });

  // Catches a key that stops the timer by mistake, or space that doesn't.
  it("maps keys to moves and keeps the timer running on arrows", () => {
    expect(onKey(deck, "ArrowRight")).toEqual({ ...deck, index: 1 });
    expect(onKey(deck, "ArrowLeft")).toEqual({ ...deck, index: 4 });
    expect(onKey(deck, "End").index).toBe(4);
    expect(onKey(deck, "Home").index).toBe(0);
    expect(onKey(deck, " ").playing).toBe(false);
    expect(onKey(deck, "x")).toEqual(deck);
  });
});
