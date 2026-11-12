# Cadenza privacy policy

Last updated: 2026-11-13

Cadenza is self-hosted software. The Cadenza apps (the Android app and the web app) connect only to the Cadenza
server that **you** enter, which you or your organization run. The Cadenza project does not run a service, and
it receives none of your data.

## What the apps handle

- **Your server address, password and session token.** The password is sent only to your server when you sign in.
  The session token is stored on the device (Android: encrypted secure storage).
- **Work your agents show you and your answers to it** (text, images, videos, documents, pages). This is loaded
  from and sent to your server. The Android app keeps a copy in memory while it runs.
- **Photos.** The camera or photo library is used only when you choose to attach a photo to an answer. The photo
  is resized on the device and sent to your server. Nothing is read from your photos otherwise.
- **Notifications.** If your server has phone notifications turned on, the app registers a push token with your
  server. Notifications are delivered through Expo and Firebase Cloud Messaging with credentials your server's
  operator sets up; the notification text (the agent's short title) passes through those services.

## What the apps do not do

- No analytics, advertising, tracking or crash reporting.
- No location, microphone or contacts access.
- No data is sold or shared with anyone other than your own server and, for notifications only, the push
  services above.

## Your data

All data lives on your Cadenza server, under the control of whoever runs it. To delete your data, delete it on
the server, or ask the server's operator. Uninstalling the app removes everything it stored on the device.

## Contact

Questions: open an issue at <https://github.com/mutex-solutions/cadenza/issues>. Security problems: see
[SECURITY.md](SECURITY.md).
