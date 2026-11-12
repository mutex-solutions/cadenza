# Sending Android test builds with Firebase App Distribution

Testers get Cadenza from Google's **App Tester** app instead of downloading the APK by hand, and are told when a
new build is out. The workflow `.github/workflows/android-firebase.yml` builds the APK, signs it with your own
upload key (not the public debug key used for the APK attached to GitHub releases), and sends it to a tester group
on every `v*` tag. Until the steps below are done it skips itself.

Builds from App Distribution and the APK from GitHub releases are signed with different keys: a phone that has one
installed must uninstall it before installing the other.

## What you do by hand (once)

1. **Firebase project.** At <https://console.firebase.google.com/> → **Create a project** (Google Analytics is not
   needed). The free Spark plan includes App Distribution.
2. **Android app.** In the project → **Add app → Android**, package name **`dev.cadenza.app`**. Skip the config
   file and SDK steps; App Distribution does not need them. Then **Project settings → General → Your apps** shows the
   **App ID** (`1:…:android:…`).
3. **Testers.** **Release & Monitor → App Distribution → Get started**, then **Testers & Groups → Add group**,
   name `testers` (the workflow sends to the group alias `testers`; set the repository variable
   `FIREBASE_TESTER_GROUPS` to use others, comma-separated), and add the testers' emails.
4. **Service account.** At <https://console.cloud.google.com/iam-admin/serviceaccounts> (pick the Firebase project)
   → **Create service account**, for example `cadenza-distribution`, role **Firebase App Distribution Admin**.
   Open it → **Keys → Add key → Create new key → JSON**.
5. **Upload key.** The same `ANDROID_UPLOAD_*` secrets as Google Play (see
   [android-play-setup.md](android-play-setup.md), step 3 and step 7). Create the key once with `keytool`.

## GitHub secrets

| Secret | Value |
|---|---|
| `FIREBASE_APP_ID` | the App ID from step 2 |
| `FIREBASE_SERVICE_ACCOUNT_JSON` | the whole JSON key file from step 4 |
| `ANDROID_UPLOAD_KEYSTORE_BASE64`, `ANDROID_UPLOAD_KEYSTORE_PASSWORD`, `ANDROID_UPLOAD_KEY_ALIAS`, `ANDROID_UPLOAD_KEY_PASSWORD` | the upload key |

## For testers

The first time, testers get an invitation email: open it on the phone, accept, and sign in to **App Tester**
(it installs from the invitation if needed). Each new build then shows up in App Tester with its release notes.
Android asks once to allow installs from App Tester.

## What is automated

- On every `v*` tag (and by hand from **Actions → Android to Firebase App Distribution → Run workflow**, with
  optional release notes): versionName from the tag, versionCode = 1000000 + the run number; an arm64 APK signed
  with your upload key (the build fails if it would use the debug key); sent to the tester group with
  firebase-tools.
- Pull requests that change the workflow build and sign the APK but do not send it.
- The key files are written to the runner's temp folder only for the build and deleted afterwards.
