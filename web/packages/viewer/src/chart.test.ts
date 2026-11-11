import { describe, expect, it, vi } from "vitest";

const embed = vi.fn(async () => ({}));
vi.mock("vega-embed", () => ({ default: embed }));
vi.mock("vega", () => ({ loader: () => ({ load: null, sanitize: null }) }));

describe("lazy chart module", () => {
  it("draws without eval (ast: true), as SVG, with no actions and no network", async () => {
    const { renderChart } = await import("./lazy/chart.ts");
    const el = document.createElement("div");
    const spec = { mark: "bar" };
    await renderChart(el, spec);
    const calls = embed.mock.calls as unknown as [HTMLElement, unknown, Record<string, unknown>][];
    const [target, passed, opts] = calls[0]!;
    expect(target).toBe(el);
    expect(passed).toBe(spec);
    expect(opts).toMatchObject({ ast: true, actions: false, renderer: "svg" });
    const loader = opts.loader as { load: () => Promise<unknown>; sanitize: () => Promise<unknown> };
    await expect(loader.load()).rejects.toThrow(/network/);
    await expect(loader.sanitize()).rejects.toThrow(/network/);
  });
});
