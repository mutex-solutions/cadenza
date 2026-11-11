import { parseDiff } from "./diff.ts";
import { h, isMarkdown, link, markdown, plain, whenVisible } from "./dom.ts";
import { loadChart, loadDiagram } from "./lazy.ts";
import { fileCard, image, isFileRef, video, webpage } from "./media.ts";
import type { Ctx, ViewSlot } from "./types.ts";

type Obj = Record<string, unknown>;
const obj = (v: unknown): Obj => (typeof v === "object" && v !== null && !Array.isArray(v) ? (v as Obj) : {});
const arr = (v: unknown): unknown[] => (Array.isArray(v) ? v : []);
const str = (v: unknown): string => (typeof v === "string" ? v : "");
const TONES = ["info", "success", "warning", "danger"];

let diagramIds = 0;

/** Draws one slot's value by its block. Unknown blocks show their data as text. */
export function renderBlock(ctx: Ctx, slot: ViewSlot): HTMLElement {
  const d = ctx.doc;
  const v = slot.value;
  switch (slot.block) {
    case "text":
      return markdown(ctx, v);
    case "callout": {
      const o = obj(v);
      const tone = TONES.includes(str(o.tone)) ? str(o.tone) : "info";
      const el = markdown(ctx, o.text, `cz-md cz-callout cz-tone-${tone}`);
      el.setAttribute("role", "note");
      el.dataset.tone = tone;
      return el;
    }
    case "checklist":
      return h(
        d,
        "ul",
        { class: "cz-checklist" },
        ...arr(v).map((it) => {
          const o = obj(it);
          const done = o.done === true;
          const li = h(
            d,
            "li",
            { attrs: { "data-done": String(done) } },
            h(d, "span", { class: "cz-check", text: done ? "✓" : "", attrs: { "aria-hidden": "true" } }),
            h(d, "span", { class: "cz-visually-hidden", text: done ? "Done: " : "To do: " }),
            markdown(ctx, o.text),
          );
          return li;
        }),
      );
    case "list":
      return h(d, "ul", { class: "cz-list" }, ...arr(v).map((it) => h(d, "li", {}, markdown(ctx, it))));
    case "image": {
      const f = obj(v).file;
      return isFileRef(f) ? image(ctx, f, slot.label) : invalid(ctx, slot);
    }
    case "video": {
      const f = obj(v).file;
      return isFileRef(f) ? video(ctx, f) : invalid(ctx, slot);
    }
    case "file": {
      const f = obj(v).file;
      return isFileRef(f) ? fileCard(ctx, f) : invalid(ctx, slot);
    }
    case "webpage": {
      const f = obj(v).file;
      return isFileRef(f) ? webpage(ctx, f, slot.label) : invalid(ctx, slot);
    }
    case "gallery":
      return h(
        d,
        "div",
        { class: "cz-gallery" },
        ...arr(v).map((it, i) => {
          const o = obj(it);
          const caption = str(o.caption);
          const f = o.file;
          return h(
            d,
            "figure",
            { class: "cz-gallery-item", attrs: { "data-item": String(i + 1) } },
            isFileRef(f) ? image(ctx, f, caption || `${slot.label} ${i + 1}`) : null,
            caption ? h(d, "figcaption", { text: caption }) : null,
          );
        }),
      );
    case "compare":
      return compare(ctx, slot);
    case "chart":
      return chart(ctx, v);
    case "diagram":
      return diagram(ctx, str(obj(v).mermaid));
    case "table":
      return table(ctx, obj(v));
    case "metrics":
      return metrics(ctx, arr(v));
    case "code":
      return code(ctx, obj(v));
    case "diff":
      return diff(ctx, obj(v));
    case "links":
      return h(
        d,
        "ul",
        { class: "cz-links" },
        ...arr(v).map((it) => {
          const o = obj(it);
          return h(d, "li", {}, link(ctx, o.url, str(o.title) || str(o.url)));
        }),
      );
    case "choices":
      return h(
        d,
        "div",
        { class: "cz-choices" },
        ...arr(v).map((it) => {
          const o = obj(it);
          const points = (key: "pros" | "cons", label: string) => {
            const items = arr(o[key]);
            if (!items.length) return null;
            return h(
              d,
              "div",
              { class: `cz-${key}` },
              h(d, "span", { class: "cz-points-label", text: label }),
              h(d, "ul", {}, ...items.map((p) => h(d, "li", { text: plain(p) }))),
            );
          };
          return h(
            d,
            "div",
            { class: "cz-choice" },
            h(d, "h4", { text: str(o.title) }),
            isMarkdown(o.text) ? markdown(ctx, o.text) : null,
            points("pros", "For"),
            points("cons", "Against"),
          );
        }),
      );
    default:
      return invalid(ctx, slot);
  }
}

function invalid(ctx: Ctx, slot: ViewSlot): HTMLElement {
  return h(ctx.doc, "pre", { class: "cz-source", text: JSON.stringify(slot.value, null, 2) ?? "" });
}

function compare(ctx: Ctx, slot: ViewSlot): HTMLElement {
  const d = ctx.doc;
  const o = obj(slot.value);
  const before = obj(o.before).file;
  const after = obj(o.after).file;
  const area = h(d, "div", { class: "cz-pick-area cz-compare" });
  const beforeEl = isFileRef(before) ? image(ctx, before, "Before") : h(d, "div");
  const afterEl = isFileRef(after) ? image(ctx, after, "After") : h(d, "div");
  // The images sit inside the compare box, which is the pick area as a whole.
  beforeEl.classList.remove("cz-pick-area");
  afterEl.classList.remove("cz-pick-area");
  beforeEl.classList.add("cz-compare-before");
  afterEl.classList.add("cz-compare-after");
  const range = h(d, "input", {
    class: "cz-compare-range",
    attrs: { type: "range", min: "0", max: "100", value: "50", "aria-label": "Before and after: slide to compare" },
  });
  const set = (n: number) => {
    afterEl.style.clipPath = `inset(0 0 0 ${n}%)`;
    area.style.setProperty("--cz-split", `${n}%`);
  };
  set(50);
  range.addEventListener("input", () => set(Number(range.value)));
  area.append(beforeEl, afterEl, h(d, "span", { class: "cz-tag cz-tag-before", text: "Before" }), h(d, "span", { class: "cz-tag cz-tag-after", text: "After" }), h(d, "div", { class: "cz-compare-line", attrs: { "aria-hidden": "true" } }));
  const caption = str(o.caption);
  return h(d, "figure", { class: "cz-compare-block" }, area, range, caption ? h(d, "figcaption", { text: caption }) : null);
}

/** Shows the source until the heavy code loads; keeps it, with a note, if loading fails. */
function lazyBlock(ctx: Ctx, cls: string, source: string, what: string, draw: (canvas: HTMLElement) => Promise<void>): HTMLElement {
  const d = ctx.doc;
  const pre = h(d, "pre", { class: "cz-source", text: source });
  const canvas = h(d, "div", { class: `${cls}-canvas` });
  const note = h(d, "p", { class: "cz-note", text: `Drawing ${what}…`, attrs: { role: "status" } });
  const el = h(d, "div", { class: cls }, canvas, note, pre);
  whenVisible(ctx, el, () => {
    draw(canvas).then(
      () => {
        pre.remove();
        note.remove();
      },
      () => {
        canvas.replaceChildren();
        note.textContent = `Couldn't draw the ${what}; showing its source.`;
      },
    );
  });
  return el;
}

function chart(ctx: Ctx, spec: unknown): HTMLElement {
  return lazyBlock(ctx, "cz-chart", JSON.stringify(spec, null, 2) ?? "", "chart", async (canvas) => {
    const m = await loadChart();
    await m.renderChart(canvas, spec);
  });
}

function diagram(ctx: Ctx, source: string): HTMLElement {
  return lazyBlock(ctx, "cz-diagram", source, "diagram", async (canvas) => {
    const m = await loadDiagram();
    await m.renderDiagram(canvas, source, `cz-mermaid-${++diagramIds}`);
  });
}

function table(ctx: Ctx, o: Obj): HTMLElement {
  const d = ctx.doc;
  const cols = arr(o.columns);
  const rows = arr(o.rows);
  const cell = (tag: "td" | "th", v: unknown) => h(d, tag, { text: plain(v), class: typeof v === "number" ? "cz-num" : "" });
  return h(
    d,
    "div",
    { class: "cz-table-wrap" },
    h(
      d,
      "table",
      { class: "cz-table" },
      h(d, "thead", {}, h(d, "tr", {}, ...cols.map((c) => h(d, "th", { text: plain(c), attrs: { scope: "col" } })))),
      h(d, "tbody", {}, ...rows.map((r) => h(d, "tr", {}, ...arr(r).map((c) => cell("td", c))))),
    ),
  );
}

/** A number from a metric value: 120, "3.1%", "812 KB". */
function num(v: unknown): number | undefined {
  if (typeof v === "number") return v;
  if (typeof v === "string") {
    const n = parseFloat(v.replace(/,/g, ""));
    return Number.isFinite(n) ? n : undefined;
  }
  return undefined;
}

function metrics(ctx: Ctx, items: unknown[]): HTMLElement {
  const d = ctx.doc;
  return h(
    d,
    "div",
    { class: "cz-metrics" },
    ...items.map((it) => {
      const o = obj(it);
      const unit = str(o.unit);
      const withUnit = (v: unknown) => (unit ? `${plain(v)} ${unit}` : plain(v));
      let change: HTMLElement | null = null;
      if (o.before !== undefined || o.delta !== undefined) {
        let deltaText = o.delta !== undefined ? plain(o.delta) : "";
        let dir = 0;
        const now = num(o.value);
        const was = num(o.before);
        if (o.delta !== undefined) {
          const dn = num(o.delta);
          dir = dn === undefined ? 0 : Math.sign(dn);
        } else if (now !== undefined && was !== undefined && was !== 0) {
          const pct = ((now - was) / Math.abs(was)) * 100;
          dir = Math.sign(pct);
          deltaText = `${pct > 0 ? "+" : pct < 0 ? "−" : ""}${Math.abs(pct) < 10 ? Math.abs(pct).toFixed(1) : Math.round(Math.abs(pct))}%`;
        }
        let tone = "";
        if (dir !== 0 && (o.good === "up" || o.good === "down")) tone = (dir > 0) === (o.good === "up") ? "cz-good" : "cz-bad";
        change = h(
          d,
          "span",
          { class: `cz-metric-change ${tone}`.trim() },
          o.before !== undefined ? h(d, "span", { text: `was ${withUnit(o.before)}` }) : null,
          deltaText ? h(d, "span", { class: "cz-delta", text: deltaText }) : null,
        );
      }
      return h(
        d,
        "div",
        { class: "cz-metric" },
        h(d, "span", { class: "cz-metric-label", text: str(o.label) }),
        h(d, "span", { class: "cz-metric-value" }, h(d, "span", { text: plain(o.value) }), unit ? h(d, "span", { class: "cz-unit", text: ` ${unit}` }) : null),
        change,
      );
    }),
  );
}

function codeFrame(ctx: Ctx, cls: string, filename: string, extra: string, rows: HTMLElement[]): HTMLElement {
  const d = ctx.doc;
  return h(
    d,
    "div",
    { class: cls },
    filename || extra ? h(d, "div", { class: "cz-code-head" }, h(d, "span", { class: "cz-filename", text: filename }), extra ? h(d, "span", { class: "cz-lang", text: extra }) : null) : null,
    h(d, "div", { class: "cz-code-body", attrs: { tabindex: "0" } }, h(d, "div", { class: "cz-lines" }, ...rows)),
  );
}

function code(ctx: Ctx, o: Obj): HTMLElement {
  const d = ctx.doc;
  const lines = str(o.code).replace(/\n$/, "").split("\n");
  const rows = lines.map((text, i) =>
    h(
      d,
      "div",
      { class: "cz-line", attrs: { "data-line": String(i + 1), "data-side": "new" } },
      h(d, "span", { class: "cz-ln", text: String(i + 1), attrs: { "aria-hidden": "true" } }),
      h(d, "span", { class: "cz-line-text", text }),
    ),
  );
  return codeFrame(ctx, "cz-code", str(o.filename), str(o.language), rows);
}

function diff(ctx: Ctx, o: Obj): HTMLElement {
  const d = ctx.doc;
  const rows = parseDiff(str(o.diff)).map((r) => {
    if (r.kind === "file" || r.kind === "hunk" || r.kind === "meta") {
      return h(d, "div", { class: `cz-diff-sep cz-diff-${r.kind}`, text: r.text });
    }
    const side = r.kind === "del" ? "old" : "new";
    const line = r.kind === "del" ? r.old : r.new;
    const sign = r.kind === "add" ? "+" : r.kind === "del" ? "-" : " ";
    return h(
      d,
      "div",
      { class: `cz-line cz-diff-${r.kind}`, attrs: { "data-line": String(line), "data-side": side } },
      h(d, "span", { class: "cz-ln", text: r.old === undefined ? "" : String(r.old), attrs: { "aria-hidden": "true" } }),
      h(d, "span", { class: "cz-ln", text: r.new === undefined ? "" : String(r.new), attrs: { "aria-hidden": "true" } }),
      h(d, "span", { class: "cz-sign", text: sign, attrs: r.kind === "ctx" ? { "aria-hidden": "true" } : { "aria-label": r.kind === "add" ? "added" : "removed" } }),
      h(d, "span", { class: "cz-line-text", text: r.text }),
    );
  });
  return codeFrame(ctx, "cz-code cz-diff", str(o.filename), "", rows);
}

export { whenVisible };
