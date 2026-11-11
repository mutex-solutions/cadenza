import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderAttachment, setLazyLoaders } from "./core.ts";
import { slotAttachment } from "./test/fixtures.ts";
import { FakeIO, flush, installIO, options } from "./test/setup.ts";

const spec = { mark: "bar", data: { values: [{ x: "a", y: 1 }] } };

beforeEach(() => installIO(false));

describe("lazy charts and diagrams", () => {
  it("loads the chart code only when the chart scrolls into view, once", async () => {
    const renderChart = vi.fn(async (el: HTMLElement, _spec: unknown) => {
      el.innerHTML = "<svg></svg>";
    });
    const chart = vi.fn(async () => ({ renderChart }));
    const diagram = vi.fn(async () => ({ renderDiagram: vi.fn(async () => undefined) }));
    setLazyLoaders({ chart, diagram });
    const a = renderAttachment(slotAttachment("chart", spec), options());
    const b = renderAttachment(slotAttachment("chart", spec), options());
    await flush();
    expect(chart).not.toHaveBeenCalled();
    expect(a.querySelector("pre.cz-source")!.textContent).toContain('"mark": "bar"');
    FakeIO.fireAll();
    await flush();
    await flush();
    expect(chart).toHaveBeenCalledTimes(1);
    expect(renderChart).toHaveBeenCalledTimes(2);
    expect(renderChart.mock.calls[0]![1]).toEqual(spec);
    expect(a.querySelector("pre.cz-source")).toBeNull();
    expect(b.querySelector(".cz-chart-canvas svg")).not.toBeNull();
    expect(diagram).not.toHaveBeenCalled();
    expect(FakeIO.all.every((io) => io.disconnected)).toBe(true);
  });

  it("draws diagrams with mermaid source and a unique id", async () => {
    const renderDiagram = vi.fn(async () => undefined);
    setLazyLoaders({ chart: vi.fn(), diagram: vi.fn(async () => ({ renderDiagram })) } as never);
    renderAttachment(slotAttachment("diagram", { mermaid: "graph TD; A-->B" }), options());
    renderAttachment(slotAttachment("diagram", { mermaid: "graph TD; C-->D" }), options());
    FakeIO.fireAll();
    await flush();
    await flush();
    const calls = renderDiagram.mock.calls as unknown as [HTMLElement, string, string][];
    expect(calls.map((c) => c[1])).toEqual(["graph TD; A-->B", "graph TD; C-->D"]);
    expect(calls[0]![2]).not.toBe(calls[1]![2]);
  });

  it("keeps the source with a note when loading fails, and retries next time", async () => {
    const diagram = vi.fn(async () => Promise.reject(new Error("offline")));
    setLazyLoaders({ chart: diagram, diagram } as never);
    const el = renderAttachment(slotAttachment("diagram", { mermaid: "graph TD; A-->B" }), options());
    FakeIO.fireAll();
    await flush();
    await flush();
    expect(el.querySelector("pre.cz-source")!.textContent).toBe("graph TD; A-->B");
    expect(el.querySelector(".cz-note")!.textContent).toMatch(/Couldn't draw the diagram/);
    renderAttachment(slotAttachment("diagram", { mermaid: "x" }), options());
    FakeIO.fireAll();
    await flush();
    expect(diagram).toHaveBeenCalledTimes(2);
  });

  it("loads at once when there's no IntersectionObserver", async () => {
    vi.stubGlobal("IntersectionObserver", undefined);
    (window as unknown as { IntersectionObserver: unknown }).IntersectionObserver = undefined;
    const chart = vi.fn(async () => ({ renderChart: vi.fn(async () => undefined) }));
    setLazyLoaders({ chart, diagram: chart } as never);
    renderAttachment(slotAttachment("chart", spec), options());
    await flush();
    expect(chart).toHaveBeenCalledTimes(1);
    vi.unstubAllGlobals();
  });
});
