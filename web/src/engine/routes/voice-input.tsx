import { useEffect, useRef, useState } from "react";
import { Button } from "@/components/ui/button";

// Speech in the browser (Web Speech API) to text. The text is shown before
// it is sent, so a misheard figure is seen, not acted on. Where the browser
// has no recognition, the button says so.
type Recognition = {
  lang: string;
  interimResults: boolean;
  continuous: boolean;
  onresult: ((e: { results: ArrayLike<ArrayLike<{ transcript: string }>> }) => void) | null;
  onend: (() => void) | null;
  onerror: ((e: { error: string }) => void) | null;
  start: () => void;
  stop: () => void;
};

function recognitionFactory(): (() => Recognition) | null {
  const w = window as unknown as {
    SpeechRecognition?: new () => Recognition;
    webkitSpeechRecognition?: new () => Recognition;
  };
  const Ctor = w.SpeechRecognition ?? w.webkitSpeechRecognition;
  return Ctor ? () => new Ctor() : null;
}

export function VoiceInput({
  onTranscript,
  language,
}: {
  onTranscript: (text: string) => void;
  language: string;
}) {
  const [listening, setListening] = useState(false);
  const [error, setError] = useState("");
  const rec = useRef<Recognition | null>(null);
  const factory = recognitionFactory();
  useEffect(() => () => rec.current?.stop(), []);
  if (!factory)
    return (
      <span className="text-xs text-muted-foreground">
        Voice input needs a browser with speech recognition.
      </span>
    );
  function toggle() {
    if (listening) {
      rec.current?.stop();
      return;
    }
    const r = factory?.();
    if (!r) return;
    r.lang = language;
    r.interimResults = false;
    r.continuous = false;
    r.onresult = (e) => {
      const text = Array.from(e.results)
        .map((res) => res[0]?.transcript ?? "")
        .join(" ")
        .trim();
      if (text) onTranscript(text);
    };
    r.onerror = (e) => setError(`Speech recognition: ${e.error}`);
    r.onend = () => setListening(false);
    rec.current = r;
    setError("");
    setListening(true);
    r.start();
  }
  return (
    <span className="flex items-center gap-2">
      <Button
        type="button"
        size="sm"
        variant={listening ? "default" : "outline"}
        onClick={toggle}
        aria-pressed={listening}
      >
        {listening ? "Listening… (stop)" : "Speak"}
      </Button>
      {error && <span className="text-xs text-destructive">{error}</span>}
    </span>
  );
}
