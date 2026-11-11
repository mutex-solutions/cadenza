import { useEffect, useRef, useState, type PointerEvent } from "react";

type Point = [number, number];

/** Splits typed labels ("login button, error text") into a clean list. */
export function parseLabels(s: string): string[] {
  return s
    .split(/[,\n]/)
    .map((x) => x.trim())
    .filter(Boolean)
    .slice(0, 50);
}

/**
 * A simple freehand sketch: draw with a finger or mouse, type the words in it,
 * and send it as a picture. (Built in instead of Excalidraw to keep the app small.)
 */
export default function Sketch({ onDone, onCancel }: { onDone: (png: Blob, labels: string[]) => void; onCancel: () => void }) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const strokes = useRef<Point[][]>([]);
  const [count, setCount] = useState(0);
  const [labels, setLabels] = useState("");

  const redraw = () => {
    const c = canvas.current;
    const ctx = c?.getContext("2d");
    if (!c || !ctx) return;
    ctx.fillStyle = "#ffffff";
    ctx.fillRect(0, 0, c.width, c.height);
    ctx.strokeStyle = "#111111";
    ctx.lineWidth = 3 * (window.devicePixelRatio || 1);
    ctx.lineCap = "round";
    ctx.lineJoin = "round";
    for (const s of strokes.current) {
      const first = s[0];
      if (!first) continue;
      ctx.beginPath();
      ctx.moveTo(first[0], first[1]);
      for (const [x, y] of s.slice(1)) ctx.lineTo(x, y);
      if (s.length === 1) ctx.lineTo(first[0] + 0.1, first[1]);
      ctx.stroke();
    }
  };

  useEffect(() => {
    const c = canvas.current;
    if (!c) return;
    const dpr = window.devicePixelRatio || 1;
    const r = c.getBoundingClientRect();
    c.width = Math.max(1, Math.round(r.width * dpr));
    c.height = Math.max(1, Math.round(r.height * dpr));
    redraw();
    const esc = (e: KeyboardEvent) => e.key === "Escape" && onCancel();
    window.addEventListener("keydown", esc);
    return () => window.removeEventListener("keydown", esc);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const point = (e: PointerEvent<HTMLCanvasElement>): Point => {
    const c = e.currentTarget;
    const r = c.getBoundingClientRect();
    return [((e.clientX - r.left) / (r.width || 1)) * c.width, ((e.clientY - r.top) / (r.height || 1)) * c.height];
  };

  const done = () => {
    const c = canvas.current;
    if (!c) return;
    c.toBlob((b) => b && onDone(b, parseLabels(labels)), "image/png");
  };

  return (
    <div className="sketch" role="dialog" aria-modal="true" aria-label="Sketch">
      <div className="bar">
        <strong>Sketch</strong>
        <span className="spacer" />
        <button
          type="button"
          onClick={() => {
            strokes.current.pop();
            setCount(strokes.current.length);
            redraw();
          }}
          disabled={count === 0}
        >
          Undo
        </button>
        <button
          type="button"
          onClick={() => {
            strokes.current = [];
            setCount(0);
            redraw();
          }}
          disabled={count === 0}
        >
          Clear
        </button>
      </div>
      <canvas
        ref={canvas}
        aria-label="Drawing area"
        onPointerDown={(e) => {
          e.currentTarget.setPointerCapture?.(e.pointerId);
          strokes.current.push([point(e)]);
          setCount(strokes.current.length);
          redraw();
        }}
        onPointerMove={(e) => {
          if (e.buttons === 0 && e.pointerType === "mouse") return;
          const s = strokes.current[strokes.current.length - 1];
          if (!s || !e.currentTarget.hasPointerCapture?.(e.pointerId)) return;
          s.push(point(e));
          redraw();
        }}
      />
      <div className="bar">
        <label htmlFor="sketch-labels" className="sr-only">
          Words in your sketch
        </label>
        <input id="sketch-labels" type="text" value={labels} onChange={(e) => setLabels(e.target.value)} placeholder="Words in your sketch, separated by commas (optional)" />
        <button type="button" onClick={onCancel}>
          Cancel
        </button>
        <button type="button" className="primary" onClick={done} disabled={count === 0}>
          Send sketch
        </button>
      </div>
    </div>
  );
}
