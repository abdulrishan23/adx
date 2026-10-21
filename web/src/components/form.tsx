import {
  type ComponentProps,
  createContext,
  type FormEvent,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";
import { cn } from "@/lib/utils";

// Form replaces the browser's validation popups with ours. Fields keep their
// native rules (required, min, type="email", pattern) so the rules live in
// one place, but the form never shows the browser's bubble: on submit it
// checks every field, marks the bad ones aria-invalid (the inputs draw that
// in the destructive colour), focuses the first, and writes the message
// under the field when a FieldError is placed for it, or in a line at the
// end of the form when none is. onSubmit only runs when everything passes.

type Errors = Record<string, string>;

type Ctx = {
  errors: Errors;
  register: (id: string) => () => void;
};

const FormContext = createContext<Ctx>({ errors: {}, register: () => () => {} });

type Control = HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement;

const isControl = (el: Element): el is Control =>
  el instanceof HTMLInputElement ||
  el instanceof HTMLSelectElement ||
  el instanceof HTMLTextAreaElement;

// labelOf names a field for its message: its label, its aria-label, its
// placeholder, or "This field".
function labelOf(el: Control): string {
  const fromLabel = el.labels?.[0]?.textContent?.trim();
  const name =
    fromLabel ||
    el.getAttribute("aria-label") ||
    ("placeholder" in el ? el.placeholder : "") ||
    "This field";
  return name.replace(/[:*]\s*$/, "");
}

// messageFor says what's wrong in our words, never the browser's.
export function messageFor(el: Control): string {
  const v = el.validity;
  const custom = el.dataset.error;
  if (custom && !v.valid) return custom;
  const label = labelOf(el);
  if (v.valueMissing) return `${label} is required.`;
  if (v.typeMismatch && el.type === "email")
    return "Enter an email address, like name@company.com.";
  if (v.typeMismatch && el.type === "url") return "Enter a full link starting with https://.";
  if (v.badInput) return `${label} needs a number.`;
  if (v.rangeUnderflow && "min" in el) return `${label} can't be below ${el.min}.`;
  if (v.rangeOverflow && "max" in el) return `${label} can't be above ${el.max}.`;
  if (v.stepMismatch) return `${label} has to be a whole step.`;
  if (v.tooShort && "minLength" in el) return `${label} needs at least ${el.minLength} characters.`;
  if (v.tooLong && "maxLength" in el) return `${label} can be up to ${el.maxLength} characters.`;
  if (v.patternMismatch) return el.title || `${label} isn't in the right format.`;
  if (v.customError) return el.validationMessage;
  return `Check ${label.toLowerCase()}.`;
}

const keyOf = (el: Control) => el.id || el.name;

export function Form({
  onSubmit,
  children,
  className,
  ...props
}: Omit<ComponentProps<"form">, "onSubmit" | "noValidate"> & {
  onSubmit?: (e: FormEvent<HTMLFormElement>) => void;
}) {
  const [errors, setErrors] = useState<Errors>({});
  const [placed, setPlaced] = useState<Record<string, number>>({});
  const register = useCallback((id: string) => {
    setPlaced((p) => ({ ...p, [id]: (p[id] ?? 0) + 1 }));
    return () => setPlaced((p) => ({ ...p, [id]: Math.max(0, (p[id] ?? 1) - 1) }));
  }, []);
  const ctx = useMemo(() => ({ errors, register }), [errors, register]);

  function check(form: HTMLFormElement): boolean {
    const next: Errors = {};
    let first: Control | null = null;
    for (const el of Array.from(form.elements)) {
      if (!isControl(el) || el.disabled || el.type === "hidden") continue;
      if (el.checkValidity()) {
        el.removeAttribute("aria-invalid");
        continue;
      }
      el.setAttribute("aria-invalid", "true");
      next[keyOf(el) || `field-${Object.keys(next).length}`] = messageFor(el);
      first ??= el;
    }
    setErrors(next);
    first?.focus();
    return first === null;
  }

  const loose = Object.entries(errors).filter(([id]) => !placed[id]);

  return (
    <FormContext.Provider value={ctx}>
      <form
        {...props}
        noValidate
        className={className}
        onSubmit={(e) => {
          e.preventDefault();
          if (!check(e.currentTarget)) return;
          onSubmit?.(e);
        }}
        onInput={(e) => {
          // A field the person is fixing stops being marked.
          const el = e.target as Element;
          if (!isControl(el)) return;
          const key = keyOf(el);
          if (key && errors[key] && el.checkValidity()) {
            el.removeAttribute("aria-invalid");
            setErrors(({ [key]: _, ...rest }) => rest);
          }
          props.onInput?.(e);
        }}
      >
        {children}
        {loose.length > 0 && (
          <p role="alert" className="basis-full text-sm text-destructive">
            {loose.map(([, m]) => m).join(" ")}
          </p>
        )}
      </form>
    </FormContext.Provider>
  );
}

// FieldError shows the message for one field, by its id, under it.
export function FieldError({ htmlFor, className }: { htmlFor: string; className?: string }) {
  const { errors, register } = useContext(FormContext);
  useEffect(() => register(htmlFor), [htmlFor, register]);
  const message = errors[htmlFor];
  if (!message) return null;
  return (
    <p id={`${htmlFor}-error`} role="alert" className={cn("text-sm text-destructive", className)}>
      {message}
    </p>
  );
}
