// Heavy: mermaid. Loaded only when a diagram scrolls into view.
import mermaid from "mermaid";

let ready = false;

export async function renderDiagram(el: HTMLElement, source: string, id: string): Promise<void> {
  if (!ready) {
    const dark = el.ownerDocument.defaultView?.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false;
    mermaid.initialize({ startOnLoad: false, securityLevel: "strict", theme: dark ? "dark" : "default" });
    ready = true;
  }
  const { svg } = await mermaid.render(id, source);
  // securityLevel "strict" makes mermaid sanitize its SVG output.
  el.innerHTML = svg;
}
