import { renderChart } from "./chart.ts";

const w = window as unknown as { CadenzaViewerLazy?: Record<string, unknown> };
w.CadenzaViewerLazy = { ...w.CadenzaViewerLazy, chart: { renderChart } };
