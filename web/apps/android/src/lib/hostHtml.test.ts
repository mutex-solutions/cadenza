import { describe, expect, it } from "vitest";
import { buildHostHtml, hostCsp } from "./hostHtml";

describe("buildHostHtml", () => {
  const html = buildHostHtml({ serverUrl: "https://cz.example.com", theme: "dark" });

  it("puts the CSP meta first in head", () => {
    const head = html.slice(html.indexOf("<head>") + 6).trimStart();
    expect(head.startsWith('<meta http-equiv="Content-Security-Policy"')).toBe(true);
    expect(html).toContain(hostCsp());
  });

  it("CSP blocks everything by default and allows only what agent pages and the viewer need", () => {
    const csp = hostCsp();
    expect(csp).toMatch(/^default-src 'none'/);
    expect(csp).toContain("script-src 'self' 'unsafe-inline'");
    expect(csp).not.toMatch(/nonce-|unsafe-eval/);
    expect(csp).toContain("img-src 'self' https: data: blob:");
    expect(csp).toContain("base-uri 'none'");
    expect(csp).toContain("form-action 'none'");
  });

  it("loads the viewer from the server and defines the bridge", () => {
    expect(html).toContain('<script src="/viewer/viewer.js"></script>');
    expect(html.indexOf("/viewer/viewer.js")).toBeLessThan(html.indexOf("window.__cz ="));
    expect(html).toContain('send({ type: "ready" })');
    expect(html).toContain('<meta name="referrer" content="no-referrer">');
  });

  it("never contains a token or the server URL in the page", () => {
    expect(html).not.toMatch(/token|Bearer|Authorization/i);
    expect(html).not.toContain("cz.example.com");
  });

  it("applies the theme", () => {
    expect(html).toContain('data-theme="dark"');
    expect(buildHostHtml({ serverUrl: "x", theme: "light" })).toContain("color-scheme:light");
  });
});
