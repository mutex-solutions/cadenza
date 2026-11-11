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

Every release has `cadenza-android-arm64.apk` attached (debug-signed, for your own phone):
`adb install -r cadenza-android-arm64.apk`. To build it yourself: `npx expo prebuild --platform android`, then
`cd android && ./gradlew assembleRelease -PreactNativeArchitectures=arm64-v8a`.
