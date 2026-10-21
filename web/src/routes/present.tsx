import { Link, useNavigate } from "@tanstack/react-router";
import { ChevronLeft, ChevronRight, Pause, Play, X } from "lucide-react";
import { useEffect, useState } from "react";
import { type Card, useHomeDesk } from "@/api/fronts";
import { CountUp } from "@/components/front/count-up";
import { EvidenceView } from "@/components/front/evidence";
import { Pie } from "@/components/front/pictures";
import { ErrorNote } from "@/components/page";
import { Button } from "@/components/ui/button";
import { StatusMark } from "@/components/ui/status-chip";
import { AUTO_ADVANCE_MS, type Deck, jump, next, onKey, previous } from "@/lib/deck";
import { cn } from "@/lib/utils";

// Focus plays the same briefing Home streams, on a wall: one beat per slide,
// always ink. It advances on its own, and a keyboard or a clicker moves it.
// Present plays Home's cards on a wall: one card per slide, large, advancing
// on its own. The same cards as the carousel, so a wall and a desk agree.
export function PresentPage() {
  const desk = useHomeDesk();
  const deck = {
    error: desk.error,
    data: desk.data ? { slides: desk.data.cards } : undefined,
  };
  const navigate = useNavigate();
  const count = deck.data?.slides.length ?? 0;
  const [d, setD] = useState<Deck>({ index: 0, count: 0, playing: true });
  useEffect(
    () => setD((s) => ({ ...s, count, index: Math.min(s.index, Math.max(0, count - 1)) })),
    [count],
  );

  useEffect(() => {
    const onDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        navigate({ to: "/" });
        return;
      }
      const after = onKey(d, e.key);
      if (after !== d) {
        e.preventDefault();
        setD(after);
      }
    };
    window.addEventListener("keydown", onDown);
    return () => window.removeEventListener("keydown", onDown);
  }, [d, navigate]);

  useEffect(() => {
    if (!d.playing || d.count === 0) {
      return;
    }
    const t = setInterval(() => setD((s) => next(s)), AUTO_ADVANCE_MS);
    return () => clearInterval(t);
  }, [d.playing, d.count]);

  const slide = deck.data?.slides[d.index];

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-background px-4 pb-6 text-foreground md:px-16 md:pb-10">
      <div className="flex gap-1 pt-4" aria-hidden="true">
        {Array.from({ length: count }, (_, i) => (
          <button
            // biome-ignore lint/suspicious/noArrayIndexKey: slides are positional
            key={i}
            type="button"
            tabIndex={-1}
            onClick={() => setD((s) => jump(s, i))}
            className={cn(
              "h-[3px] flex-1 transition-colors",
              i < d.index ? "bg-chart-4" : i === d.index ? "bg-foreground" : "bg-muted",
            )}
          />
        ))}
      </div>
      <div className="flex items-center justify-between py-4">
        <p className="font-mono text-[0.68rem] font-medium tracking-[0.18em] text-muted-foreground uppercase">
          Present{count > 0 && ` · ${d.index + 1} of ${count}`}
        </p>
        <div className="flex gap-1.5">
          <Button
            variant="outline"
            size="icon-sm"
            aria-label="Previous"
            onClick={() => setD((s) => previous(s))}
          >
            <ChevronLeft />
          </Button>
          <Button
            variant="outline"
            size="icon-sm"
            aria-label="Next"
            onClick={() => setD((s) => next(s))}
          >
            <ChevronRight />
          </Button>
          <Button
            variant="outline"
            size="icon-sm"
            aria-label={d.playing ? "Pause" : "Play"}
            onClick={() => setD((s) => ({ ...s, playing: !s.playing }))}
          >
            {d.playing ? <Pause /> : <Play />}
          </Button>
          <Link
            to="/"
            className="ml-2 inline-flex h-8 items-center gap-1.5 border px-2.5 text-sm hover:bg-muted"
            aria-label="Exit"
          >
            <X className="size-4" /> Exit
          </Link>
        </div>
      </div>
      {/* The slide is a page: the left quarter goes back, the rest goes on.
          Two invisible buttons over the slide, out to the screen's edges. */}
      <div className="relative -mx-4 -mb-6 flex min-h-0 flex-1 overflow-hidden px-4 pb-6 md:-mx-16 md:-mb-10 md:px-16 md:pb-10">
        {deck.error && <ErrorNote error={deck.error} />}
        {slide && deck.data && <SlideView key={d.index} card={slide} />}
        <button
          type="button"
          aria-label="Previous slide"
          tabIndex={-1}
          onClick={() => setD((s) => previous(s))}
          className="absolute inset-y-0 left-0 w-1/4 cursor-default"
        />
        <button
          type="button"
          aria-label="Next slide"
          tabIndex={-1}
          onClick={() => setD((s) => next(s))}
          className="absolute inset-y-0 right-0 w-3/4 cursor-default"
        />
      </div>
    </div>
  );
}

const DOOR_LABEL = { studio: "Projects", perform: "Perform", convert: "Convert" } as const;

function SlideView({ card: c }: { card: Card }) {
  return (
    <section className="mx-auto flex w-full max-w-[1180px] flex-1 animate-rise flex-col justify-center gap-7">
      <p className="flex items-center gap-2.5 font-mono text-[0.72rem] font-medium tracking-[0.18em] text-muted-foreground uppercase">
        <StatusMark status={c.status} className="size-3" />
        {DOOR_LABEL[c.door]}
      </p>
      <h1 className="max-w-[24ch] font-heading text-4xl font-semibold leading-[1.08] tracking-[-0.02em] md:text-5xl lg:text-6xl">
        {c.title}
      </h1>
      <p className="max-w-[60ch] text-lg text-muted-foreground md:text-xl">{c.why}</p>
      {c.figure && (
        <p className="flex items-baseline gap-3 font-heading text-[6rem] font-semibold leading-none tracking-[-0.04em] tabular-nums md:text-[8rem]">
          <CountUp value={c.figure.value} to={c.figure.count} />
          {c.figure.unit && (
            <span className="text-2xl font-medium tracking-normal text-muted-foreground md:text-3xl">
              {c.figure.unit}
            </span>
          )}
        </p>
      )}
      {c.evidence && (
        <div className="min-h-0 max-w-5xl overflow-hidden">
          {c.evidence.kind === "pie" ? (
            <Pie slices={c.evidence.slices} size="wall" />
          ) : (
            <EvidenceView evidence={c.evidence} size="wall" />
          )}
        </div>
      )}
    </section>
  );
}
