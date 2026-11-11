// Entry for dist/viewer.js (one file, global CadenzaViewer), used inside the
// Android WebView. Charts and diagrams load from viewer-chart.js and
// viewer-diagram.js next to it, only when needed.
import * as api from "./core.ts";
import type { ChartModule, DiagramModule } from "./types.ts";

type Lazy = { chart?: ChartModule; diagram?: DiagramModule };
const w = window as unknown as { CadenzaViewer?: unknown; CadenzaViewerLazy?: Lazy };

const script = document.currentScript as HTMLScriptElement | null;
const base = script?.src || document.baseURI;

function load<K extends keyof Lazy>(name: K): Promise<NonNullable<Lazy[K]>> {
  const have = w.CadenzaViewerLazy?.[name];
  if (have) return Promise.resolve(have as NonNullable<Lazy[K]>);
  return new Promise((resolve, reject) => {
    const s = document.createElement("script");
    s.src = new URL(`viewer-${name}.js`, base).href;
    s.async = true;
    s.onload = () => {
      const m = w.CadenzaViewerLazy?.[name];
      if (m) resolve(m as NonNullable<Lazy[K]>);
      else reject(new Error(`viewer-${name}.js did not load`));
    };
    s.onerror = () => {
      s.remove();
      reject(new Error(`viewer-${name}.js did not load`));
    };
    document.head.appendChild(s);
  });
}

api.setLazyLoaders({ chart: () => load("chart"), diagram: () => load("diagram") });

w.CadenzaViewer = {
  renderPost: api.renderPost,
  renderAttachment: api.renderAttachment,
  injectStyles: api.injectStyles,
  sandboxSrcdoc: api.sandboxSrcdoc,
  WEBPAGE_CSP: api.WEBPAGE_CSP,
  setLazyLoaders: api.setLazyLoaders,
  VERSION: api.VERSION,
};
