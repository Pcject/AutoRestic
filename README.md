# AutoRestic

[中文文档](README.zh-CN.md)

AutoRestic is a self-hosted web console for managing [restic](https://restic.net/) repositories. It combines a Go API, a Vue UI, SQLite metadata, scheduled backup jobs, snapshot browsing, restore actions, and execution logs in one Docker-friendly service.

AutoRestic is a high-privilege backup administration tool. It can read mounted source paths, restore files, unlock repositories, prune data, and delete snapshots. Do not expose it without authentication and network controls.

## Features

- Repository setup for local, rclone, and WebDAV restic backends.
- Encrypted storage for repository passwords and rclone/WebDAV credentials.
- Backup task wizard with path parsing, exclude rules, advanced restic flags, and schedules.
- Snapshot index and file browser designed for large repositories.
- Restore, delete, check, prune, unlock, and sync operations from the UI.
- Execution log viewer with stdout/stderr tabs and realtime WebSocket streaming.
- Docker image and Compose templates for self-hosted deployment.

## Screenshots

The screenshots below use sanitized demo data. Repository names, hostnames, paths, snapshot IDs, and timestamps are placeholders.

### Dashboard

![Dashboard](assets/screenshots/dashboard.svg)

### Backup Task Wizard

![Backup Task Wizard](assets/screenshots/backup-task-wizard.svg)

### Snapshots

![Snapshots](assets/screenshots/snapshots.svg)

### Snapshot File Browser

![Snapshot File Browser](assets/screenshots/snapshot-file-browser.svg)

### Execution Log

![Execution Log](assets/screenshots/execution-log.svg)

## Quick Start

Use the published image:

```bash
export AUTORESTIC_AUTH_TOKEN="$(openssl rand -base64 32)"
docker compose -f docker-compose.image.yml up -d
```

Open:

```text
http://127.0.0.1:8080
```

The default image Compose file binds the service to `127.0.0.1:8080`. Keep that default unless a reverse proxy in front of AutoRestic provides TLS and access control.

To build locally:

```bash
export AUTORESTIC_AUTH_TOKEN="$(openssl rand -base64 32)"
docker compose up --build -d
```

The UI stores the bearer token in browser local storage. Open Settings and save the same token before using API-backed pages.

## Docker

Published image:

```text
pcject/autorestic:latest
```

For production, prefer a pinned release tag instead of `latest` once release tags are available.

Example image deployment:

```yaml
services:
  autorestic:
    image: pcject/autorestic:latest
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:80"
    environment:
      AUTORESTIC_AUTH_TOKEN: ${AUTORESTIC_AUTH_TOKEN:?Set AUTORESTIC_AUTH_TOKEN before starting AutoRestic}
      AUTORESTIC_DB_PATH: /app/data/autorestic.db
      AUTORESTIC_ENC_KEY_PATH: /app/data/autorestic.key
      TZ: ${TZ:-Asia/Shanghai}
    volumes:
      - autorestic-data:/app/data
      # Mount source paths explicitly, preferably read-only.
      # - /host/photos:/backup/photos:ro
      # - /host/restore:/restore

volumes:
  autorestic-data:
```

Verify that a built image does not contain copied local databases or keys:

```bash
docker run --rm --entrypoint sh pcject/autorestic:latest -c \
  'find /app -name "autorestic.key" -o -name "*.db" -o -name "*.db-*"'
```

Expected result: no output.

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `AUTORESTIC_PORT` | `8080` | Go API port inside the container |
| `AUTORESTIC_DB_PATH` | `data/autorestic.db` | SQLite database path |
| `AUTORESTIC_ENC_KEY_PATH` | `data/autorestic.key` | Encryption key path for saved repository secrets |
| `AUTORESTIC_RESTIC_BIN` | `restic` | Restic executable path |
| `AUTORESTIC_AUTH_TOKEN` | empty | Bearer token for API access and WebSocket ticket issuance |
| `AUTORESTIC_CORS_ORIGINS` | empty | Comma-separated allowed browser origins when auth is enabled |

Compose deployments require `AUTORESTIC_AUTH_TOKEN` to avoid accidentally running an unauthenticated file-operation service.

## Security Notes

- Treat AutoRestic as an administrator-only tool.
- Always set `AUTORESTIC_AUTH_TOKEN` outside local development.
- Keep the default `127.0.0.1:8080` bind unless a trusted reverse proxy adds TLS and access control.
- Persist `/app/data`; losing `autorestic.key` means saved repository passwords cannot be decrypted.
- Back up both `/app/data/autorestic.db` and `/app/data/autorestic.key` before migrations or host moves.
- Mount backup sources read-only when possible, and use a dedicated writable restore target.
- Treat rclone config, WebDAV passwords, repository passwords, local databases, and key files as secrets.
- Do not commit `backend/data/`, `*.db`, `*.key`, `.env*`, local Compose overrides, or private planning files.

## Development

Backend:

```bash
cd backend
go test ./...
go run .
```

Frontend:

```bash
cd frontend
npm ci
npm run dev
```

Production build:

```bash
docker build -t pcject/autorestic:dev .
```

## Testing

Backend checks:

```bash
cd backend
go test ./...
go test -race ./...
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@latest ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

Frontend checks:

```bash
cd frontend
npm run typecheck
npm run build
npm audit
```

Smoke test after deployment:

1. Start the container and confirm health is `healthy`.
2. Open the UI, save the bearer token in Settings, and confirm the Logs page loads.
3. Create or import a test repository.
4. Run one small backup and confirm a snapshot appears.
5. Restore one harmless file into a dedicated restore directory.

## Publishing Checklist

- Publish from a clean initial commit, orphan public branch, or new repository.
- Do not push the current private development history if it contains removed internal files.
- Confirm the public tree has no tracked private files:

```bash
git ls-files docs .claude backend/data dist
```

Expected result: no output.

## Contributing

Issues and pull requests are welcome. Please keep changes focused, include tests for behavioral changes, and avoid committing local backup data, secrets, or generated build output.

## License

AutoRestic is released under the [MIT License](LICENSE).
