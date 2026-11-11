// Heavy: vega-embed, vega and vega-lite. Loaded only when a chart scrolls into view.
import embed from "vega-embed";
import { loader } from "vega";

/** Charts draw only the data inside the spec; nothing is fetched from the network. */
function offlineLoader() {
  const l = loader();
  const refuse = async (): Promise<never> => {
    throw new Error("charts can't load data from the network; put the data in the spec");
  };
  l.load = refuse;
  l.sanitize = refuse;
  return l;
}

export async function renderChart(el: HTMLElement, spec: unknown): Promise<void> {
  const dark = el.ownerDocument.defaultView?.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false;
  await embed(el, spec as Parameters<typeof embed>[1], {
    // No eval: expressions run through vega's interpreter, so charts work under a
    // CSP without 'unsafe-eval' (the Android WebView). With ast: true, vega-embed
    // uses its own bundled expressionInterpreter (vega-interpreter) as `expr`.
    ast: true,
    actions: false,
    renderer: "svg",
    loader: offlineLoader(),
    ...(dark ? { theme: "dark" as const } : {}),
  });
}
