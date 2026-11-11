import { useEffect, useImperativeHandle, useMemo, useRef, useState, type Ref } from "react";
import { Linking, ScrollView, StyleSheet, Text, View } from "react-native";
import { WebView, type WebViewMessageEvent } from "react-native-webview";
import type { ShouldStartLoadRequest } from "react-native-webview/lib/WebViewTypes";
import type { Spot } from "@cadenza/core";
import { jsCall, parseBridgeMessage, type HostMarker, type HostPost } from "../lib/bridge";
import { buildHostHtml } from "../lib/hostHtml";
import { htmlToText } from "../lib/text";
import { useTheme } from "../theme";

// The package's class defaults its extra props to `undefined`, which makes
// all props `never` under strict React 19 types; pin them to "none".
const Web = WebView<{}>;
type Web = WebView<{}>;

export interface PostWebViewHandle {
  /** Asks the owner to tap a spot on a post. */
  pick(postId: string): void;
  cancelPick(): void;
}

interface Props {
  serverUrl: string;
  posts: HostPost[];
  /** Comment markers per post id. */
  markers: Record<string, HostMarker[]>;
  resolveFile: (sha256: string) => Promise<string>;
  onSpot: (postId: string, spot: Spot | null) => void;
  onMarker?: (id: string) => void;
  ref?: Ref<PostWebViewHandle>;
}

/** Is this navigation the host page itself (allowed), rather than a link (opened outside)? */
function isHostLoad(url: string, serverUrl: string): boolean {
  return url === "about:blank" || url === "about:srcdoc" || url === serverUrl || url === `${serverUrl}/`;
}

/**
 * Shows posts with the shared viewer. The WebView is created after the first
 * frame so the screen appears at once; until then post titles show natively.
 */
export function PostWebView(props: Props) {
  const { colors, scheme } = useTheme();
  const web = useRef<Web>(null);
  const [mounted, setMounted] = useState(false);
  const [ready, setReady] = useState(false);
  const html = useMemo(() => buildHostHtml({ serverUrl: props.serverUrl, theme: scheme }), [props.serverUrl, scheme]);

  useEffect(() => {
    // Wait for the first frame to be drawn, then create the WebView.
    let inner = 0;
    const outer = requestAnimationFrame(() => {
      inner = requestAnimationFrame(() => setMounted(true));
    });
    return () => {
      cancelAnimationFrame(outer);
      cancelAnimationFrame(inner);
    };
  }, []);

  useEffect(() => {
    if (!ready) return;
    web.current?.injectJavaScript(jsCall("render", props.posts));
    for (const [postId, list] of Object.entries(props.markers)) web.current?.injectJavaScript(jsCall("markers", postId, list));
  }, [ready, props.posts, props.markers]);

  useImperativeHandle(props.ref, () => ({
    pick: (postId) => web.current?.injectJavaScript(jsCall("pick", postId)),
    cancelPick: () => web.current?.injectJavaScript(jsCall("cancelPick")),
  }));

  function onMessage(e: WebViewMessageEvent) {
    const m = parseBridgeMessage(e.nativeEvent.data);
    if (!m) return;
    switch (m.type) {
      case "ready":
        setReady(true);
        break;
      case "resolve":
        props.resolveFile(m.sha256).then(
          (url) => web.current?.injectJavaScript(jsCall("resolved", m.id, url)),
          () => web.current?.injectJavaScript(jsCall("resolved", m.id, null)),
        );
        break;
      case "open":
        void Linking.openURL(m.url);
        break;
      case "spot":
        props.onSpot(m.postId, m.spot);
        break;
      case "marker":
        props.onMarker?.(m.id);
        break;
      case "error":
        break;
    }
  }

  function onShouldStart(req: ShouldStartLoadRequest): boolean {
    if (isHostLoad(req.url, props.serverUrl)) return true;
    // Every other navigation is a link: never load it inside the viewer.
    if (/^https?:\/\//i.test(req.url) && req.isTopFrame) void Linking.openURL(req.url);
    return false;
  }

  return (
    <View style={{ flex: 1 }}>
      {!ready ? (
        <ScrollView style={StyleSheet.absoluteFill} contentContainerStyle={{ padding: 12 }}>
          {props.posts.map((p) => (
            <View key={p.id} style={{ marginBottom: 20 }}>
              <Text style={{ color: colors.text, fontSize: 18, fontWeight: "700" }}>{p.title}</Text>
              {p.summary_html ? <Text style={{ color: colors.muted, marginTop: 4 }}>{htmlToText(p.summary_html)}</Text> : null}
              <Text style={{ color: colors.muted, marginTop: 8 }}>Loading…</Text>
            </View>
          ))}
        </ScrollView>
      ) : null}
      {mounted ? (
        <Web
          ref={web}
          style={{ flex: 1, backgroundColor: colors.bg, opacity: ready ? 1 : 0 }}
          source={{ html, baseUrl: props.serverUrl }}
          originWhitelist={[props.serverUrl, "about:*"]}
          onMessage={onMessage}
          onShouldStartLoadWithRequest={onShouldStart}
          javaScriptEnabled
          domStorageEnabled={false}
          javaScriptCanOpenWindowsAutomatically={false}
          setSupportMultipleWindows={false}
          allowFileAccess={false}
          allowFileAccessFromFileURLs={false}
          allowUniversalAccessFromFileURLs={false}
          geolocationEnabled={false}
          mediaPlaybackRequiresUserAction
          allowsFullscreenVideo
          // Not incognito: the HTTP cache keeps /viewer/viewer.js between visits,
          // which makes opening a thread fast. The page holds no login token.
          incognito={false}
          cacheEnabled
          cacheMode="LOAD_DEFAULT"
          thirdPartyCookiesEnabled={false}
          sharedCookiesEnabled={false}
          mixedContentMode="never"
          webviewDebuggingEnabled={__DEV__}
        />
      ) : null}
    </View>
  );
}
