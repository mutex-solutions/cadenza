import { renderDiagram } from "./diagram.ts";

const w = window as unknown as { CadenzaViewerLazy?: Record<string, unknown> };
w.CadenzaViewerLazy = { ...w.CadenzaViewerLazy, diagram: { renderDiagram } };
