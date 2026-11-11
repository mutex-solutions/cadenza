# Self-hosting Cadenza

Cadenza is one server program plus S3-compatible storage. It runs comfortably on a 1-CPU machine.

## Trial (one minute)

```sh
docker run -p 8080:8080 -v cadenza:/data ghcr.io/mutex-solutions/cadenza
```

or download `cadenza-server` from the releases and run it. Trial mode keeps everything in `CADENZA_DATA_DIR`:
a SQLite database and uploaded files on local disk. Open the server in a browser and set the owner password.

## Real use

Use `deploy/docker-compose.yml` as a starting point: the server, Postgres and SeaweedFS for storage.

1. Pick two HTTPS hostnames, one for the app (`cadenza.example.com`) and one for storage
   (`files.cadenza.example.com`), and point a TLS reverse proxy (Caddy, nginx, Traefik) at ports 8080 and 8333.
2. Change every value marked `CHANGE` in the compose file and `seaweedfs-s3.json`.
3. Create the bucket and allow browser uploads (CORS), shown below.
4. `docker compose up -d`, open the app, set the owner password.
5. On each machine: install `cadenza`, run `cadenza login https://cadenza.example.com` and `cadenza setup`.

### Storage

Any S3-compatible store works if your machines and phone can reach it over HTTPS: SeaweedFS, Garage, AWS S3,
Cloudflare R2, Ceph. Files go directly between the store and the CLI or apps through signed links. Uploads carry a
signed SHA-256 checksum so the store rejects bytes that don't match.

| Store | Notes |
|---|---|
| SeaweedFS, Garage, AWS S3, Cloudflare R2 | Checksums supported. |
| Backblaze B2 | Rejects checksum headers: set `CADENZA_S3_CHECKSUM=none`. The server still checks each file's size. |
| MinIO | The community edition is archived (2026); prefer another store. |

The bucket needs CORS so the web app can upload photos and load files. For example with the AWS CLI:

```sh
aws --endpoint-url https://files.cadenza.example.com s3api create-bucket --bucket cadenza
aws --endpoint-url https://files.cadenza.example.com s3api put-bucket-cors --bucket cadenza --cors-configuration '{
  "CORSRules": [{"AllowedOrigins": ["https://cadenza.example.com"], "AllowedMethods": ["GET", "PUT", "HEAD"],
                 "AllowedHeaders": ["content-type", "x-amz-checksum-sha256"], "ExposeHeaders": ["ETag"], "MaxAgeSeconds": 3600}]}'
```

## Configuration

All settings are environment variables.

| Variable | Default | Meaning |
|---|---|---|
| `CADENZA_PUBLIC_URL` | `http://localhost:8080` | The address phones and agents use. Links and cookies depend on it. |
| `CADENZA_ADDR` | `:8080` | Listen address. |
| `CADENZA_DATA_DIR` | `./data` | Local files: the SQLite database and trial-mode uploads. |
| `CADENZA_DATABASE` | `sqlite:$CADENZA_DATA_DIR/cadenza.db` | `sqlite:<path>` or `postgres://user:pass@host/db`. |
| `CADENZA_S3_BUCKET` | (empty: trial mode) | Bucket for files. Setting it turns on S3. |
| `CADENZA_S3_ENDPOINT` | (AWS) | The store's address as the server reaches it. |
| `CADENZA_S3_PUBLIC_ENDPOINT` | `CADENZA_S3_ENDPOINT` | The store's address as phones and agents reach it (used in signed links). |
| `CADENZA_S3_REGION` | `us-east-1` | Region. |
| `CADENZA_S3_ACCESS_KEY`, `CADENZA_S3_SECRET_KEY` | | Credentials. |
| `CADENZA_S3_CHECKSUM` | `sha256` | `none` for stores that reject checksum headers. |
| `CADENZA_VAPID_SUBJECT` | `mailto:admin@localhost` | Your contact address for browser push services. |
| `CADENZA_EXPO` | | `1` to send Android notifications through Expo. |
| `CADENZA_EXPO_ACCESS_TOKEN` | | Your own Expo access token (turns on Expo; needed if your Expo project uses enhanced push security). |
| `CADENZA_FCM_SERVICE_ACCOUNT_FILE` | | Path to your own Firebase service account JSON, to send to Android directly through FCM. |
| `CADENZA_MIN_CLI_VERSION` | release default | Older `cadenza` commands are told to update. |

## Notifications

- **Browsers and installed web apps** (Android, desktop, iPhone with iOS 16.4+ after "Add to Home Screen") use
  Web Push. It needs no outside account: the server makes its keys on first start and keeps them in the database.
- **The Android app** uses Expo push, with your own Expo project and Firebase credentials: build the app with EAS
  under your Expo account (see `web/apps/android/README.md`), then set `CADENZA_EXPO=1` (and
  `CADENZA_EXPO_ACCESS_TOKEN` if your project uses enhanced security). Or send straight through FCM with
  `CADENZA_FCM_SERVICE_ACCOUNT_FILE`.

Every send is logged; the app's settings page shows the log and has a test button.

## Operating

- `GET /healthz` answers `ok` when the database is reachable.
- `cadenza-server owner reset-password` sets a new owner password (reads it from stdin; stop the server first
  when using SQLite).
- Back up the database. Files in storage are named by their SHA-256 and never change, so storage backups are
  simple copies.
- The change log keeps 30 days; apps that were offline longer reload everything.
