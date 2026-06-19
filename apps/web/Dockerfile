# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM node:22-bookworm AS build

WORKDIR /src
COPY package.json package-lock.json ./
COPY apps/web/package.json apps/web/package.json
RUN --mount=type=cache,id=prism-oss-web-npm,target=/root/.npm,sharing=locked npm ci --no-audit --no-fund
COPY . .
ENV NEXT_PUBLIC_PRISM_EDITION=oss
ENV NEXT_TELEMETRY_DISABLED=1
RUN npm --workspace apps/web run build

FROM --platform=$TARGETPLATFORM node:22-bookworm AS runtime-deps
WORKDIR /app
ENV NODE_ENV=production
COPY package.json package-lock.json ./
COPY apps/web/package.json apps/web/package.json
RUN --mount=type=cache,id=prism-oss-web-npm,target=/root/.npm,sharing=locked npm ci --omit=dev --no-audit --no-fund

FROM --platform=$TARGETPLATFORM node:22-bookworm-slim
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_PUBLIC_PRISM_EDITION=oss
ENV NEXT_TELEMETRY_DISABLED=1
COPY --from=runtime-deps /app/package.json /app/package-lock.json ./
COPY --from=runtime-deps /app/apps/web/package.json apps/web/package.json
COPY --from=runtime-deps /app/node_modules node_modules
COPY --from=build /src/apps/web/.next apps/web/.next
COPY --from=build /src/apps/web/next.config.mjs apps/web/next.config.mjs
EXPOSE 3000
CMD ["npm", "--workspace", "apps/web", "run", "start", "--", "--hostname", "0.0.0.0", "--port", "3000"]
