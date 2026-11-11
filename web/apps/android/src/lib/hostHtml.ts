// The page the viewer WebView loads. It is given as an HTML string with
// baseUrl = the server, so "/viewer/viewer.js" (the shared viewer, served by
// the Cadenza server next to the web app) loads from the server and the HTTP
// cache keeps it. The page holds no login token: file links are asked from
// the app over the bridge.

export interface HostOptions {
  /** The Cadenza server, e.g. "https://cadenza.example.com". */
  serverUrl: string;
  theme: "light" | "dark";
}

/**
 * The host page's Content-Security-Policy.
 *
 * Agent webpages are drawn by the viewer in `<iframe sandbox="allow-scripts"
 * srcdoc>`. A srcdoc frame inherits this policy on top of its own injected
 * one (default-src 'none'…), and the stricter of the two wins. So this policy
 * must allow inline scripts and data:/blob: media, or agent pages break; the
 * agent page's own policy still blocks every network request. No nonce is
 * used, because a nonce would turn 'unsafe-inline' off for those frames.
 */
export function hostCsp(): string {
  return [
    "default-src 'none'",
    "script-src 'self' 'unsafe-inline'",
    "style-src 'self' 'unsafe-inline'",
    "img-src 'self' https: data: blob:",
    "media-src 'self' https: data: blob:",
    "font-src 'self' data:",
    "connect-src 'self' https:",
    "frame-src 'self' about: data: blob:",
    "base-uri 'none'",
    "form-action 'none'",
  ].join("; ");
}

/** The bridge script: window.__cz, called by the app with injectJavaScript. */
const BRIDGE = `
(function () {
  var RN = window.ReactNativeWebView;
  function send(m) { if (RN) RN.postMessage(JSON.stringify(m)); }
  var root = document.getElementById("posts");
  var viewers = {};
  var pending = {};
  var nextId = 1;
  var V = window.CadenzaViewer;
  if (V && V.injectStyles) V.injectStyles();
  var opts = {
    resolveFile: function (sha256) {
      var id = nextId++;
      return new Promise(function (resolve, reject) {
        pending[id] = { resolve: resolve, reject: reject };
        send({ type: "resolve", id: id, sha256: sha256 });
      });
    },
    openLink: function (url) { send({ type: "open", url: String(url) }); },
    onMarkerClick: function (id) { send({ type: "marker", id: String(id) }); }
  };
  function clear() {
    for (var k in viewers) { try { viewers[k].destroy(); } catch (e) {} }
    viewers = {};
    root.textContent = "";
  }
  window.__cz = {
    render: function (posts) {
      clear();
      if (!V) {
        var p = document.createElement("p");
        p.className = "cz-host-error";
        p.textContent = "Couldn't load the viewer from the server. Check your connection and open the thread again.";
        root.appendChild(p);
        send({ type: "error", message: "viewer not loaded" });
        return;
      }
      posts.forEach(function (post) {
        var sec = document.createElement("section");
        sec.className = "cz-host-post";
        sec.id = "post-" + post.id;
        var h = document.createElement("h2");
        h.textContent = post.title;
        sec.appendChild(h);
        if (post.summary_html) {
          var s = document.createElement("div");
          s.className = "cz-host-summary";
          s.innerHTML = post.summary_html; // sanitized by the server
          sec.appendChild(s);
        }
        var box = document.createElement("div");
        sec.appendChild(box);
        root.appendChild(sec);
        try { viewers[post.id] = V.renderPost(box, post.attachments, opts); }
        catch (e) { box.textContent = "This post could not be shown."; }
      });
    },
    resolved: function (id, url) {
      var p = pending[id];
      if (!p) return;
      delete pending[id];
      if (url) p.resolve(url); else p.reject(new Error("file unavailable"));
    },
    pick: function (postId) {
      var v = viewers[postId];
      if (!v) { send({ type: "spot", postId: postId, spot: null }); return; }
      var el = document.getElementById("post-" + postId);
      if (el) el.scrollIntoView({ block: "start" });
      v.pickSpot().then(function (spot) {
        send({ type: "spot", postId: postId, spot: spot || null });
      }, function () { send({ type: "spot", postId: postId, spot: null }); });
    },
    cancelPick: function () {
      for (var k in viewers) { try { viewers[k].cancelPick(); } catch (e) {} }
    },
    markers: function (postId, list) {
      var v = viewers[postId];
      if (v) v.setMarkers(list);
    }
  };
  send({ type: "ready" });
})();
`;

const COLORS = {
  light: { bg: "#ffffff", text: "#111827", muted: "#4b5563" },
  dark: { bg: "#0b0f14", text: "#e8eef6", muted: "#a2b0c1" },
} as const;

/** The full host page. */
export function buildHostHtml(o: HostOptions): string {
  const c = COLORS[o.theme === "dark" ? "dark" : "light"];
  return `<!doctype html>
<html lang="en" data-theme="${o.theme === "dark" ? "dark" : "light"}">
<head>
<meta http-equiv="Content-Security-Policy" content="${hostCsp()}">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<meta name="referrer" content="no-referrer">
<style>
html,body{margin:0;background:${c.bg};color:${c.text};font:16px/1.5 system-ui,sans-serif;color-scheme:${o.theme === "dark" ? "dark" : "light"}}
#posts{padding:12px 12px 96px}
.cz-host-post{margin:0 0 28px}
.cz-host-post h2{font-size:18px;margin:0 0 6px}
.cz-host-summary{color:${c.muted};margin:0 0 10px}
.cz-host-error{color:${c.muted}}
</style>
</head>
<body>
<main id="posts"></main>
<script src="/viewer/viewer.js"></script>
<script>${BRIDGE}</script>
</body>
</html>`;
}
