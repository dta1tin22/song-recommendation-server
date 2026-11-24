# song-recommendation-server

Server-side music recommendation with embedded vector search.

This file documents the full dev setup (frontend, backend, Docker) and the currently implemented pages. Paths below are relative to the repository root unless stated otherwise.

This is an early scaffold: the React frontend is usable for UI review, and the Go backend is only a stub. There is no API, no database, and no recommendation logic wired up yet.

## Repository layout

| Path | What it is |
| --- | --- |
| `frontend/` | Vite + React 19 + TypeScript app, Tailwind v4, shadcn/ui, React Router v8. pnpm-managed. |
| `backend/` | Go module (`go 1.27.3`) with the `cmd/api` entrypoint. |
| `docker-compose.yml` | Dev-only compose: frontend Vite server + Go backend. |
| `frontend/design/` | Static HTML layout references from the design pass. Styling source of truth is shadcn tokens, not these files. |

There is **no root `package.json` or workspace** — always run frontend commands from `frontend/` and Go commands from `backend/`.

## Requirements

| Tool | Version | Needed for |
| --- | --- | --- |
| Node.js | 26 | Frontend (local dev) |
| pnpm | 12 | Frontend (local dev; `frontend/pnpm-lock.yaml`) |
| Go | 1.27.3+ | Backend (local dev) |
| Docker Engine + Compose plugin / Docker Desktop | current | Optional; container-based dev |

Check what you have:

```bash
node --version
pnpm --version
go version
docker compose version   # optional
```

### Installing the toolchain

**Windows (PowerShell)**

- Node.js 26: install from <https://nodejs.org/> (or `winget install OpenJS.NodeJS`).
- Enable pnpm via Corepack, which ships with Node:

  ```powershell
  corepack enable
  corepack prepare pnpm@12 --activate
  ```

  Alternatively: `npm install -g pnpm@12`.
- Go: install from <https://go.dev/dl/> (or `winget install GoLang.Go`). Restart PowerShell afterward so `go` is on `PATH`.
- Docker: install Docker Desktop and make sure the Compose plugin is enabled.
- Optional: use WSL 2 and follow the Linux commands below instead; the repo commands themselves are identical.

**Linux**

- Node.js 26: use [nvm](https://github.com/nvm-sh/nvm) (`nvm install 26 && nvm use 26`), [fnm](https://github.com/Schniz/fnm), or a distro package if it provides a recent enough version.
- Enable pnpm:

  ```bash
  corepack enable
  corepack prepare pnpm@12 --activate
  ```

  Alternatively: `npm install -g pnpm@12`.
- Go: install from <https://go.dev/dl/> or your package manager, then verify `go version` reports 1.27.3+.
- Docker: install Docker Engine and the Compose plugin (`docker compose version` should work; the old `docker-compose` binary is not required).
- If you use WSL, keep the repo inside the Linux filesystem (not `/mnt/c/...`) for reliable HMR and file watching.

## Run locally (recommended)

Open two terminals. Commands below use POSIX-style paths; they work unchanged in PowerShell, Git Bash, and Linux shells.

### Terminal 1 — frontend

```bash
cd frontend
pnpm install
pnpm dev
```

Open <http://localhost:5173>. The dev server binds `0.0.0.0:5173`. Vite proxies `/api` requests to `http://localhost:8080` by default (see `frontend/vite.config.ts`).

Other frontend commands (run from `frontend/`):

```bash
pnpm build     # tsc -b && vite build — this is the only typecheck
pnpm lint      # eslint . (not type-aware)
pnpm preview   # serve the production build
```

### Terminal 2 — backend

```bash
cd backend
go run ./cmd/api
```

Other backend commands (run from `backend/`):

```bash
go build ./cmd/api   # produces the `api` binary
go vet ./...
```

Heads-up: `App.Run()` currently just returns `nil`, so the backend starts and immediately exits with code 0. That is expected — there is no HTTP server yet. Starting it is optional for frontend UI work. `go test ./...` reports "no test files"; no test runner is installed on the frontend either.

## Run with Docker (dev-only)

From the repository root:

```bash
docker compose up --build
```

- Frontend: <http://localhost:5173>
- Backend container: port 8080 is mapped, but it exits 0 immediately for the reason above.
- Compose injects `VITE_PROXY_TARGET=http://backend:8080` so Vite proxies `/api` to the backend service rather than to the frontend container itself.
- After changing `frontend/package.json` / `pnpm-lock.yaml`, rebuild with `docker compose up --build`. If the mounted `node_modules` volume is stale, reset it with `docker compose down -v` first.
- If HMR misses changes (common on Docker Desktop and VMs), enable polling:

  ```powershell
  # Windows PowerShell
  $env:CHOKIDAR_USEPOLLING = "true"; docker compose up --build
  ```

  ```bash
  # Linux / WSL / Git Bash
  CHOKIDAR_USEPOLLING=true docker compose up --build
  ```

Stop with `docker compose down`. Both Dockerfiles are dev-oriented; `frontend/Dockerfile` runs `pnpm dev` and has no production/nginx stage.

## Implemented pages / URLs

All pages are reachable from the frontend dev server at `http://localhost:5173`.

| Page | URL | Status and notes |
| --- | --- | --- |
| Home | `/` | Done. Hero copy, search form, seed-style suggestion pills, and Cmd/Ctrl+K focus shortcut. The search submit is currently a no-op; it does not navigate or call an API. |
| Log In | `/login` | Done as UI. Email, password with show/hide toggle, remember-me checkbox, and submit button. No auth API, no third-party/Social login yet, and submit is a no-op. "Forgot password?" is still a `#` placeholder. |
| Sign Up | `/signup` | Done as UI. Display name, email, password with show/hide toggle and minimum length, submit button. No auth API and submit is a no-op. Links to `/login` and back. |
| Search results | `/search?q=...` | Done as a static placeholder. Reads `q` from the URL query string; shows a seed-track card, query ribbon, and 12 dummy similar-track rows with similarity scores. No backend call, row selection, or "load more" behavior; artwork is a placeholder block. Example: `/search?q=Aphex%20Twin`. |

Shared shell (on every page):

- **Top bar** — logo links to `/`, "Search" links to `/search`. "Random Seed", "History", "API", and the profile button are placeholders.
- **Bottom bar** — static footer from the design reference.

Routes are registered in `frontend/src/app/router.tsx`, paths are centralized in `frontend/src/app/routes/paths.ts`, and each page lives under `frontend/src/features/`. Unknown paths redirect to `/`.

## Current limitations

- No backend HTTP endpoints; the Go binary exits immediately by design.
- No authentication, persistence, or third-party login.
- Search results are hard-coded; the home search form does not pass its input to `/search` yet.
- Design HTML files in `frontend/design/` are layout references only — don't copy their utility classes or tokens into the app; use the installed shadcn components and theme tokens.
