FROM node:22-bookworm-slim AS web-build
WORKDIR /src
RUN npm install --global pnpm@10.33.0
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY apps/web/package.json ./apps/web/package.json
RUN --mount=type=cache,id=aerosight-pnpm,target=/pnpm/store pnpm install --frozen-lockfile --store-dir=/pnpm/store
COPY apps/web ./apps/web
COPY scripts/prepare-web.mjs scripts/web-csp.mjs scripts/prepare-server.mjs ./scripts/
COPY db/migrations ./db/migrations
ENV NEXT_TELEMETRY_DISABLED=1
RUN pnpm build:web && node scripts/prepare-web.mjs && node scripts/prepare-server.mjs

FROM golang:1.26.1-bookworm AS server-build
WORKDIR /src/apps/server
COPY apps/server/go.mod apps/server/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY apps/server ./
COPY --from=web-build /src/apps/server/internal/webassets/dist ./internal/webassets/dist
COPY --from=web-build /src/apps/server/internal/migrations/sql ./internal/migrations/sql
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/aerosight ./cmd/aerosight

FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 aerosight && useradd --uid 10001 --gid aerosight --no-create-home aerosight \
    && mkdir -p /app /var/lib/aerosight/objects && chown -R aerosight:aerosight /var/lib/aerosight
WORKDIR /app
COPY --from=server-build /out/aerosight /usr/local/bin/aerosight
ENV AEROSIGHT_ENV=production HTTP_LISTEN_ADDRESS=0.0.0.0:8080 OBJECT_STORAGE_LOCAL_ROOT=/var/lib/aerosight/objects
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/aerosight"]
CMD ["serve"]
