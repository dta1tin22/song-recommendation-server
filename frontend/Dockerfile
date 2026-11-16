# syntax=docker/dockerfile:1

# Development image: runs the Vite dev server with HMR.
# Source is bind-mounted in docker-compose.yml; the COPY below is a fallback
# so the image also runs standalone.
FROM node:26-alpine

# Keep in sync with the host pnpm major (host: pnpm 12, lockfileVersion 9.0).
RUN npm install -g pnpm@12

WORKDIR /app

# Manifests first so dependency layers cache independently of source edits.
COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY . .

EXPOSE 5173

CMD ["pnpm", "dev"]
