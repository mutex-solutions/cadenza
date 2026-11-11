import type { AttachmentView, FileRef, Spot } from "@cadenza/core";

export type { AttachmentView, FileRef, Spot };

/** The fixed set of blocks templates are built from (plan D7). */
export type BlockName =
  | "text"
  | "callout"
  | "checklist"
  | "list"
  | "image"
  | "gallery"
  | "compare"
  | "video"
  | "chart"
  | "table"
  | "metrics"
  | "code"
  | "diff"
  | "diagram"
  | "file"
  | "webpage"
  | "links"
  | "choices";

/** A markdown field as the server sends it: the source and sanitized HTML. */
export interface Markdown {
  md: string;
  html: string;
}

/** One filled slot of a template attachment. */
export interface ViewSlot {
  name: string;
  block: BlockName | string;
  label: string;
  value: unknown;
}

/** A template attachment's data (internal/template/render.go View). */
export interface View {
  template: string;
  version?: number;
  slots: ViewSlot[];
}

export interface ViewerOptions {
  /** Returns a signed URL for a stored file. */
  resolveFile: (sha256: string) => Promise<string>;
  /** Downloads text (webpages). Default: fetch(url).then(r => r.text()). */
  fetchText?: (url: string) => Promise<string>;
  /** Opens a link or file. Default: window.open(url, "_blank", "noopener,noreferrer"). */
  openLink?: (url: string) => void;
  /** Called when a comment marker is tapped. */
  onMarkerClick?: (id: string) => void;
  /** Document to build elements in. Default: the container's document. */
  document?: Document;
}

/** A pinned comment drawn on the post. */
export interface Marker {
  id: string;
  spot: Spot;
  /** Short text on the pin, e.g. "1". */
  label: string;
}

export interface PostViewer {
  readonly element: HTMLElement;
  /** Waits for the owner to tap a spot; null when cancelled (Escape, Cancel, cancelPick). */
  pickSpot(): Promise<Spot | null>;
  cancelPick(): void;
  setMarkers(markers: Marker[]): void;
  /** Scrolls attachment `index` into view. */
  show(index: number): void;
  destroy(): void;
}

export interface ChartModule {
  renderChart(el: HTMLElement, spec: unknown): Promise<void>;
}
export interface DiagramModule {
  renderDiagram(el: HTMLElement, source: string, id: string): Promise<void>;
}

/** How the heavy chart and diagram code is loaded, only when needed. */
export interface LazyLoaders {
  chart(): Promise<ChartModule>;
  diagram(): Promise<DiagramModule>;
}

/** What renderers share while drawing one attachment. */
export interface Ctx {
  opts: ViewerOptions;
  doc: Document;
  /** Attachment index. */
  index: number;
  /** Run on destroy: observers, listeners. */
  cleanups: (() => void)[];
}
