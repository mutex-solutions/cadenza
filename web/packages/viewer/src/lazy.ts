import type { LazyLoaders } from "./types.ts";

const missing = () => Promise.reject(new Error("no loader for charts and diagrams"));
let loaders: LazyLoaders = { chart: missing, diagram: missing };
let cache: { chart?: ReturnType<LazyLoaders["chart"]>; diagram?: ReturnType<LazyLoaders["diagram"]> } = {};

/** Sets how chart and diagram code is loaded (dynamic import on the web, script tags in the Android WebView). */
export function setLazyLoaders(l: LazyLoaders): void {
  loaders = l;
  cache = {};
}

/** Loads the chart code once; a failed load is retried next time. */
export function loadChart(): ReturnType<LazyLoaders["chart"]> {
  cache.chart ??= loaders.chart().catch((e: unknown) => {
    cache.chart = undefined;
    throw e;
  });
  return cache.chart;
}

export function loadDiagram(): ReturnType<LazyLoaders["diagram"]> {
  cache.diagram ??= loaders.diagram().catch((e: unknown) => {
    cache.diagram = undefined;
    throw e;
  });
  return cache.diagram;
}
