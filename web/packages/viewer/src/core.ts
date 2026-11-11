// Everything except the default lazy loaders. The IIFE build starts here so
// that chart and diagram code stays out of viewer.js.
export { renderAttachment, renderPost, VERSION } from "./viewer.ts";
export { injectStyles } from "./styles.ts";
export { WEBPAGE_CSP, sandboxSrcdoc } from "./sandbox.ts";
export { setLazyLoaders } from "./lazy.ts";
export { parseDiff } from "./diff.ts";
export type { DiffRow } from "./diff.ts";
export type {
  AttachmentView,
  BlockName,
  ChartModule,
  DiagramModule,
  FileRef,
  LazyLoaders,
  Markdown,
  Marker,
  PostViewer,
  Spot,
  View,
  ViewerOptions,
  ViewSlot,
} from "./types.ts";
