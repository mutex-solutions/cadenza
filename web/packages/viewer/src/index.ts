import { setLazyLoaders } from "./lazy.ts";

export * from "./core.ts";

// On the web, Vite splits these into their own chunks, loaded only when a
// chart or diagram scrolls into view.
setLazyLoaders({
  chart: () => import("./lazy/chart.ts"),
  diagram: () => import("./lazy/diagram.ts"),
});
