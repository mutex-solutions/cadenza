# Publishing the Android app to Google Play testing

Cadenza's Android app can go to Google Play's **internal testing** (or closed/open testing) track automatically.
Testers then install and update it from the Play Store like any app. The workflow
`.github/workflows/android-play.yml` builds a signed Android App Bundle (AAB) and uploads it with fastlane.

Until the steps below are done the workflow skips itself, and releases keep shipping the sideload APK
(`cadenza-android-arm64.apk`, signed with the public debug key, for testing only).

## What you do by hand (once)

### 1. Google Play developer account

1. Sign up at <https://play.google.com/console/signup> and pay the one-time **US$25** registration fee.
2. Complete identity verification. Personal accounts created after November 2023 must run a **closed test with at
   least 12 testers for 14 days** before they can publish to production; internal testing works right away.

### 2. Create the app

1. Play Console → **Create app**: name "Cadenza", app, free.
2. Fill in the required "App content" forms (privacy policy URL, data safety, ads: none, content rating,
   target audience). Internal testing needs most of these before the first release can roll out.
3. Decide the package name. The app uses **`dev.cadenza.app`** (`android.package` in `web/apps/android/app.json`).
   It is permanent on Play: if you change it, set `PLAY_PACKAGE_NAME` too.

### 3. Create the upload key (keep it safe)

```sh
keytool -genkeypair -v -keystore cadenza-upload.jks -alias upload \
  -keyalg RSA -keysize 2048 -validity 10000 -dname "CN=Cadenza"
```

Use one password: modern keystores (PKCS12) use the keystore password for the key as well. Store the file and
password in a password manager. This is only the **upload** key: with **Play App Signing** (the default for new
apps) Google keeps the real app signing key, and a lost upload key can be reset in Play Console.

### 4. First upload by hand

Google Play only accepts API uploads after the app has one bundle uploaded in Play Console.

```sh
cd web/apps/android
CI=1 npx expo prebuild --platform android
cd android
CADENZA_UPLOAD_STORE_FILE=/path/to/cadenza-upload.jks CADENZA_UPLOAD_STORE_PASSWORD=… \
CADENZA_UPLOAD_KEY_ALIAS=upload CADENZA_UPLOAD_KEY_PASSWORD=… \
CADENZA_VERSION_CODE=1000000 ./gradlew bundleRelease -PreactNativeArchitectures=arm64-v8a,armeabi-v7a,x86_64
```

Then Play Console → **Testing → Internal testing → Create new release**, accept **Play App Signing**, upload
`app/build/outputs/bundle/release/app-release.aab` and roll it out. (CI version codes start above 1000000, so they
always come after this one.)

### 5. Service account for the API

1. Google Cloud Console → pick or create a project → **APIs & Services → Enable APIs** → enable
   **Google Play Android Developer API**.
2. **IAM & Admin → Service accounts → Create**, for example `cadenza-play-upload`. No project roles needed.
   Open it → **Keys → Add key → JSON** and download the file.
3. Play Console → **Users and permissions → Invite new users** → enter the service account's email → under
   **App permissions** add Cadenza with **Release apps to testing tracks** (and "View app information").
   It can take a few minutes, sometimes up to a day, before the API accepts it.

### 6. Testers

Play Console → **Testing → Internal testing → Testers**: create an email list (up to 100 people) and copy the
**opt-in link**. Testers open the link once on their phone, accept, then install Cadenza from the Play Store.
For more testers use closed testing (`alpha` track) with a Google Group.

### 7. GitHub secrets

Repository → **Settings → Secrets and variables → Actions → New repository secret**:

| Secret | Value |
|---|---|
| `ANDROID_UPLOAD_KEYSTORE_BASE64` | `base64 -w0 cadenza-upload.jks` (macOS: `base64 -i cadenza-upload.jks`) |
| `ANDROID_UPLOAD_KEYSTORE_PASSWORD` | the keystore password |
| `ANDROID_UPLOAD_KEY_ALIAS` | `upload` |
| `ANDROID_UPLOAD_KEY_PASSWORD` | the same password |
| `PLAY_SERVICE_ACCOUNT_JSON` | the whole JSON key file's contents |

### 8. Notifications on the Play build (optional)

Push to the Android app goes through Expo with your own project: run `npx eas init` in `web/apps/android` (sets
`extra.eas.projectId` in `app.json`), upload your Firebase (FCM v1) credentials in the Expo dashboard, and set
`CADENZA_EXPO=1` on the server. See `web/apps/android/README.md`. Without this the app works; only phone
notifications are off (browser notifications from the web app still work).

## What is automated

- **On every `v*` tag** (and by hand from **Actions → Android to Google Play → Run workflow**, choosing the track):
  1. versionName from the tag (`v0.1.5` → `0.1.5`), versionCode = 1000000 + the workflow's run number, so it
     always grows;
  2. builds the AAB for arm64, armv7 and x86_64, signed with your upload key (the build fails if it would be
     signed with the debug key);
  3. uploads it with fastlane to the chosen track (`internal` by default) as a completed release.
- The keystore is written to the runner's temp folder only for the build and deleted afterwards. Secrets never
  appear in logs.
- If the app's first release is still in review, run the workflow by hand with **release status: draft**.

## Pieces

| File | Role |
|---|---|
| `.github/workflows/android-play.yml` | the workflow |
| `web/apps/android/plugins/release-signing.js` | signs release builds with the upload key from environment variables |
| `web/apps/android/app.config.js` | sets versionName/versionCode from `CADENZA_VERSION_NAME` / `CADENZA_VERSION_CODE` |
| `web/apps/android/fastlane/` and `Gemfile` | the upload (`fastlane android upload aab:…`), fastlane pinned to 2.241.2 |

Sideload APKs (from releases) and Play builds are signed with different keys, so a phone cannot switch from one
to the other without uninstalling first.
