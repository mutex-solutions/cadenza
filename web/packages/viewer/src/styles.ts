/** The viewer's styles. Colors come from --cz-* variables the app may set; light and dark defaults otherwise. */
export const CSS = `
.cz-root{--_bg:var(--cz-bg,#fff);--_surface:var(--cz-surface,#f6f7f9);--_line:var(--cz-line,#dfe3e8);--_text:var(--cz-text,#1b1f24);--_muted:var(--cz-muted,#5d6875);--_accent:var(--cz-accent,#0b6bcb);--_good:var(--cz-good,#1a7f4b);--_bad:var(--cz-bad,#c62828);--_warn:var(--cz-warn,#9a6700);--_mono:var(--cz-mono,ui-monospace,SFMono-Regular,Menlo,Consolas,monospace);--_r:var(--cz-radius,10px);--_add:rgba(26,127,75,.12);--_del:rgba(198,40,40,.12);color:var(--_text);font:inherit;line-height:1.5;min-width:0}
@media (prefers-color-scheme:dark){.cz-root{--_bg:var(--cz-bg,#0f141b);--_surface:var(--cz-surface,#161d27);--_line:var(--cz-line,#2a3646);--_text:var(--cz-text,#e6edf5);--_muted:var(--cz-muted,#9aa7b6);--_accent:var(--cz-accent,#7cc4fa);--_good:var(--cz-good,#5fd3a0);--_bad:var(--cz-bad,#f28b8b);--_warn:var(--cz-warn,#f0c36a);--_add:rgba(95,211,160,.14);--_del:rgba(242,139,139,.14)}}
.cz-root *{box-sizing:border-box}
.cz-post{container-type:inline-size;position:relative}
.cz-track{display:flex;flex-direction:column;gap:20px}
.cz-attachment{min-width:0}
.cz-att-head{font-size:12px;color:var(--_muted);margin:0 0 8px;text-transform:uppercase;letter-spacing:.04em}
.cz-nav{display:none;align-items:center;justify-content:space-between;gap:8px;margin:0 0 8px}
.cz-counter{font-size:13px;color:var(--_muted)}
.cz-nav button,.cz-button{font:inherit;font-size:14px;border:1px solid var(--_line);background:var(--_surface);color:var(--_text);border-radius:8px;padding:6px 12px;min-height:36px;cursor:pointer}
.cz-nav button:disabled{opacity:.4;cursor:default}
.cz-root :focus-visible{outline:2px solid var(--_accent);outline-offset:2px}
@container (max-width:719px){.cz-nav{display:flex}.cz-track{flex-direction:row;overflow-x:auto;scroll-snap-type:x mandatory;gap:16px;scrollbar-width:none;overscroll-behavior-x:contain}.cz-track::-webkit-scrollbar{display:none}.cz-attachment{flex:0 0 100%;scroll-snap-align:start}}
@supports not (container-type:inline-size){@media (max-width:719px){.cz-nav{display:flex}.cz-track{flex-direction:row;overflow-x:auto;scroll-snap-type:x mandatory;gap:16px}.cz-attachment{flex:0 0 100%;scroll-snap-align:start}}}
.cz-post[data-count="1"] .cz-nav{display:none}
.cz-slot{margin:0 0 18px;position:relative}
.cz-slot>h3{font-size:13px;font-weight:600;color:var(--_muted);margin:0 0 6px;display:flex;align-items:center;gap:6px}
.cz-md>:first-child{margin-top:0}.cz-md>:last-child{margin-bottom:0}
.cz-md a,.cz-links a{color:var(--_accent)}
.cz-md pre,.cz-md code{font-family:var(--_mono);font-size:.9em}
.cz-md img{max-width:100%}
.cz-callout{border:1px solid var(--_line);border-left:4px solid var(--_accent);background:var(--_surface);border-radius:var(--_r);padding:10px 12px}
.cz-tone-success{border-left-color:var(--_good)}.cz-tone-warning{border-left-color:var(--_warn)}.cz-tone-danger{border-left-color:var(--_bad)}
.cz-list,.cz-checklist,.cz-links{margin:0;padding-left:20px}
.cz-list li+li,.cz-checklist li+li,.cz-links li+li{margin-top:4px}
.cz-checklist{list-style:none;padding:0}
.cz-checklist li{display:flex;gap:8px;align-items:flex-start}
.cz-check{flex:0 0 18px;height:18px;border:1.5px solid var(--_line);border-radius:5px;font-size:12px;line-height:15px;text-align:center;margin-top:3px;color:var(--_bg)}
.cz-checklist li[data-done="true"] .cz-check{background:var(--_good);border-color:var(--_good)}
.cz-checklist li[data-done="true"] .cz-md{color:var(--_muted)}
.cz-visually-hidden{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0);white-space:nowrap}
.cz-pick-area{position:relative;display:block;max-width:100%}
.cz-image img{display:block;max-width:100%;height:auto;border-radius:var(--_r);border:1px solid var(--_line);background:var(--_surface);cursor:zoom-in}
.cz-gallery{display:flex;gap:12px;overflow-x:auto;scroll-snap-type:x mandatory;padding-bottom:4px}
.cz-gallery-item{margin:0;flex:0 0 min(85%,420px);scroll-snap-align:start}
.cz-gallery-item img{width:100%}
figcaption{font-size:13px;color:var(--_muted);margin-top:6px}
.cz-compare-block{margin:0}
.cz-compare{display:grid;overflow:hidden;border-radius:var(--_r);border:1px solid var(--_line)}
.cz-compare>.cz-image{grid-area:1/1}
.cz-compare img{border:0;border-radius:0;width:100%;cursor:default}
.cz-compare-line{grid-area:1/1;justify-self:start;margin-left:var(--cz-split,50%);width:2px;background:#fff;box-shadow:0 0 0 1px rgba(0,0,0,.3);pointer-events:none}
.cz-tag{grid-area:1/1;align-self:start;margin:8px;font-size:11px;padding:2px 6px;border-radius:4px;background:rgba(0,0,0,.6);color:#fff;pointer-events:none}
.cz-tag-after{justify-self:end}
.cz-compare-range{width:100%;margin:8px 0 0;accent-color:var(--_accent)}
.cz-video-wrap video{display:block;width:100%;max-height:70vh;border-radius:var(--_r);background:#000}
.cz-chips{display:flex;flex-wrap:wrap;gap:6px;margin-top:6px}
.cz-source{font-family:var(--_mono);font-size:12px;background:var(--_surface);border:1px solid var(--_line);border-radius:var(--_r);padding:10px;overflow:auto;max-height:320px;margin:0;white-space:pre}
.cz-note{font-size:12px;color:var(--_muted);margin:4px 0}
.cz-chart-canvas,.cz-diagram-canvas{overflow-x:auto}
.cz-chart-canvas svg,.cz-diagram-canvas svg{max-width:100%;height:auto}
.cz-table-wrap{overflow-x:auto;border:1px solid var(--_line);border-radius:var(--_r)}
.cz-table{border-collapse:collapse;width:100%;font-size:14px}
.cz-table th,.cz-table td{padding:6px 10px;border-bottom:1px solid var(--_line);text-align:left;vertical-align:top}
.cz-table th{background:var(--_surface);font-weight:600}
.cz-table tr:last-child td{border-bottom:0}
.cz-table .cz-num{text-align:right;font-variant-numeric:tabular-nums}
.cz-metrics{display:grid;grid-template-columns:repeat(auto-fill,minmax(140px,1fr));gap:10px}
.cz-metric{border:1px solid var(--_line);border-radius:var(--_r);padding:10px 12px;background:var(--_surface);display:flex;flex-direction:column;gap:2px}
.cz-metric-label{font-size:12px;color:var(--_muted)}
.cz-metric-value{font-size:22px;font-weight:650;font-variant-numeric:tabular-nums}
.cz-unit{font-size:14px;font-weight:500;color:var(--_muted)}
.cz-metric-change{font-size:12px;color:var(--_muted);display:flex;gap:6px;flex-wrap:wrap}
.cz-good .cz-delta{color:var(--_good);font-weight:600}.cz-bad .cz-delta{color:var(--_bad);font-weight:600}
.cz-code{border:1px solid var(--_line);border-radius:var(--_r);overflow:hidden;background:var(--_surface)}
.cz-code-head{display:flex;justify-content:space-between;gap:8px;padding:6px 10px;border-bottom:1px solid var(--_line);font-size:12px;color:var(--_muted);font-family:var(--_mono)}
.cz-code-body{overflow-x:auto;max-height:70vh}
.cz-lines{display:table;min-width:100%;font-family:var(--_mono);font-size:12.5px;line-height:1.6}
.cz-line,.cz-diff-sep{display:table-row;position:relative}
.cz-line>span{display:table-cell;white-space:pre;padding:0 8px}
.cz-ln{text-align:right;color:var(--_muted);user-select:none;width:1%;opacity:.7}
.cz-sign{width:1%;padding:0 2px!important;user-select:none}
.cz-line-text{width:100%;position:relative}
.cz-diff-add{background:var(--_add)}.cz-diff-del{background:var(--_del)}
.cz-diff-sep{display:block;padding:2px 10px;color:var(--_muted);background:var(--_bg);white-space:pre;font-family:var(--_mono);font-size:12px}
.cz-file{display:flex;align-items:center;gap:12px;flex-wrap:wrap;border:1px solid var(--_line);border-radius:var(--_r);padding:10px 12px;background:var(--_surface)}
.cz-file-icon{font:600 11px var(--_mono);border:1px solid var(--_line);border-radius:6px;padding:8px 6px;color:var(--_muted)}
.cz-file-info{display:flex;flex-direction:column;min-width:0;flex:1}
.cz-file-name{overflow-wrap:anywhere}
.cz-file-meta,.cz-file-status{font-size:12px;color:var(--_muted)}
.cz-webpage{border:1px solid var(--_line);border-radius:var(--_r);overflow:hidden;background:var(--_bg)}
.cz-toolbar{display:flex;justify-content:flex-end;padding:6px;border-bottom:1px solid var(--_line);background:var(--_surface)}
.cz-frame-wrap{height:70vh}
.cz-frame-wrap iframe{display:block;width:100%;height:100%;border:0;background:#fff}
.cz-webpage.cz-fullscreen{position:fixed;inset:0;z-index:1000;border-radius:0;display:flex;flex-direction:column}
.cz-webpage.cz-fullscreen .cz-frame-wrap{flex:1;height:auto}
.cz-overlay{position:absolute;inset:0;display:none;cursor:crosshair}
.cz-loading,.cz-unavailable{color:var(--_muted);font-size:14px;padding:12px;margin:0}
.cz-choices{display:grid;gap:10px}
.cz-choice{border:1px solid var(--_line);border-radius:var(--_r);padding:10px 12px}
.cz-choice h4{margin:0 0 4px;font-size:15px}
.cz-choice ul{margin:2px 0 0;padding-left:18px}
.cz-points-label{font-size:12px;font-weight:600;color:var(--_muted)}
.cz-pros .cz-points-label{color:var(--_good)}.cz-cons .cz-points-label{color:var(--_bad)}
.cz-pickbar{display:none;position:sticky;top:0;z-index:20;align-items:center;justify-content:space-between;gap:8px;padding:8px 12px;margin:0 0 8px;border-radius:var(--_r);background:var(--_accent);color:var(--_bg);font-size:14px}
.cz-pickbar button{background:var(--_bg);color:var(--_text);border:0}
.cz-picking .cz-pickbar{display:flex}
.cz-picking .cz-overlay{display:block}
.cz-picking .cz-slot,.cz-picking .cz-attachment{cursor:crosshair}
.cz-picking .cz-slot:hover{outline:1px dashed var(--_accent);outline-offset:4px}
.cz-picking .cz-line:hover{background:rgba(127,127,127,.18)}
.cz-picking img{cursor:crosshair}
.cz-marker{font:600 11px/1 system-ui,sans-serif;background:var(--_accent);color:var(--_bg);border:2px solid var(--_bg);border-radius:999px;min-width:22px;height:22px;padding:0 6px;display:inline-flex;align-items:center;justify-content:center;cursor:pointer;box-shadow:0 1px 3px rgba(0,0,0,.3)}
.cz-pin{position:absolute;transform:translate(-50%,-50%);z-index:5}
.cz-pin-rect{position:absolute;border:2px solid var(--_accent);border-radius:4px;pointer-events:none;z-index:4}
.cz-line-text .cz-marker{position:absolute;right:4px;top:50%;transform:translateY(-50%);height:18px;min-width:18px}
`;

/** Adds the viewer's styles to the document once. */
export function injectStyles(doc: Document = document): void {
  if (doc.getElementById("cz-viewer-styles")) return;
  const style = doc.createElement("style");
  style.id = "cz-viewer-styles";
  style.textContent = CSS;
  (doc.head ?? doc.documentElement).appendChild(style);
}
