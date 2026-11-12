# Cadenza for Android

The Android app for Cadenza. Log in to your Cadenza server, see every thread
from every machine, look at what your agents showed, answer their questions
(with photos), and get a notification when an agent needs you.

Built with Expo (SDK 58, New Architecture). Agent work is drawn by the same
viewer the web app uses: the app loads `/viewer/viewer.js` from your server
inside a locked-down WebView. Agent webpages run in a sandboxed frame inside
it, with no network and no way to reach the app.

## Commands (from `web/`)

```sh
pnpm --filter @cadenza/android typecheck
pnpm --filter @cadenza/android test      # plain logic in src/lib, runs in Node
pnpm --filter @cadenza/android start     # Expo dev server
```

## Build your own app (self-hosting)

Notifications go through Expo's push service, which needs an Expo project and
a Firebase project that **you** own. Nothing goes through ours.

1. **Expo project.** Install the EAS CLI (`npm i -g eas-cli`), log in
   (`eas login`), then in `web/apps/android` run `eas init`. Put the project id
   it prints into `app.json` → `expo.extra.eas.projectId`.
2. **App id.** Change `expo.android.package` in `app.json` from
   `dev.cadenza.app` to your own, e.g. `com.yourname.cadenza`.
3. **Firebase.** Create a Firebase project, add an Android app with the same
   package name, download `google-services.json` into `web/apps/android`, and
   set `expo.android.googleServicesFile` to `"./google-services.json"`.
4. **Push key.** In Firebase → Project settings → Service accounts, create a
   private key (FCM V1). Upload it with `eas credentials` → Android →
   your build profile → Push Notifications → FCM V1 service account key.
5. **Server.** Create an Expo access token (expo.dev → Account settings →
   Access tokens) and give it to your Cadenza server as its Expo access token
   (the server's Expo push setting, `expopush.Config.AccessToken`). Without
   it, Android notifications are off; everything else still works.
6. **Build.** `eas build -p android --profile preview` makes an APK you can
   install directly. Use `--profile production` for a Play Store bundle.

Then open the app, enter your server address and password, and turn on
notifications in Settings. "Send a test" checks the whole chain; the log
below it shows every notification and any error.

## Notes

- Use an `https://` server address. Plain `http://` is accepted only for
  your own network or Tailscale (10.x, 192.168.x, 172.16–31.x, 100.64–127.x,
  `*.ts.net`, `localhost`).
- The login token is kept in Android's secure storage (`expo-secure-store`).
- While the app is open it checks for changes every 5 seconds; when it's
  closed, notifications take over.
- The thread view needs the server to serve `/viewer/viewer.js` (the web
  app build does this). If the phone has never loaded it and is offline, posts
  show their titles only.

## Checked on an emulator

Release build on a Pixel 6a emulator profile (Android 15, x86_64 on KVM), November 2026:

- Logged in to an `http://` server on the local network, saw the inbox update live, opened a thread (images,
  markdown, lists render), answered a question; the agent's `cadenza wait` returned the answer.
- Cold start (`am start -W`, TotalTime): 0.61–0.75 s. A real Pixel 6a is slower than the emulator on this host;
  measure on the device before a release.
- Single-ABI release APK: 24 MB (both ABIs: 41 MB). The Play Store delivers per device.

Every release has `cadenza-android-arm64.apk` attached (debug-signed, for your own phone; native libraries are
unpacked at install, so it also runs on x86_64 emulators with arm64 translation):
`adb install -r cadenza-android-arm64.apk`. To build it yourself: `npx expo prebuild --platform android`, then
`cd android && ./gradlew assembleRelease -PreactNativeArchitectures=arm64-v8a`.

## Tablets and foldables

The app follows window size classes (`src/lib/layout.ts`): compact under 600 dp, medium 600–839, expanded 840+.

- From 700 dp wide it shows the inbox and the open thread side by side. That covers tablets and a book-style
  foldable's inner screen (Galaxy Z Fold, about 707–830 dp) in both orientations; a phone or the cover screen
  keeps one pane.
- Forms, settings, the reply panel and posts have maximum widths so nothing stretches across a big screen.
- Orientation is not locked. Rotation, fold/unfold, split-screen resizing and density changes are handled in
  place (`plugins/large-screens.js` sets `configChanges` and `resizeableActivity`), so the open thread stays open.

Checked on the emulator at phone, Fold inner (portrait and landscape), Fold cover and tablet (portrait and
landscape) sizes, including density changes, with the thread staying open and the process never restarting.

Also checked on the emulator's 7.6" fold-in foldable profile with the v0.1.2 release APK: device states CLOSED,
HALF_OPENED and OPENED (via `cmd device_state`) in portrait and landscape keep the process running and the open
thread open. That profile has a single display, so the switch to a separate cover screen was covered with size and
density changes instead.

In the half-opened "book" posture the two panes split exactly on the hinge: `modules/cadenza-fold` is a small
local Expo module that reports the fold from Jetpack WindowManager (`FoldingFeature`), and `src/lib/fold.ts`
places the split there (and leaves a physical hinge gap empty). That also gives two panes on an inner screen
narrower than the usual 700 dp. Lying flat, the width rules apply; held sideways (tabletop, horizontal hinge) the
fold is ignored. Checked on the emulator's 7.6" foldable: book posture split at 884 px on a 1768 px screen (the
hinge), back to one pane when flat, thread kept open throughout.

Not covered at all without hardware: real performance and a real Galaxy Z Fold's two displays.

## End-to-end test

`scripts/android-e2e.sh` runs the app for real on an emulator: a Cadenza server and the `cadenza` CLI (as the agent)
show work and ask a question, the Maestro flow `e2e/answer-question.yaml` logs in, opens the thread and taps an
answer, and the script checks the agent's `cadenza wait` received it. CI runs it on every change to the app
(`.github/workflows/android-e2e.yml`). Locally, with an emulator booted and Maestro installed:

```sh
scripts/android-e2e.sh web/apps/android/android/app/build/outputs/apk/release/app-release.apk
```
