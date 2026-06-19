FROM node:22-bookworm AS build

WORKDIR /src
COPY package.json package-lock.json ./
COPY apps/web/package.json apps/web/package.json
RUN npm ci
COPY . .
ENV NEXT_PUBLIC_PRISM_EDITION=oss
RUN npm --workspace apps/web run build

FROM node:22-bookworm-slim
WORKDIR /app
ENV NODE_ENV=production
ENV NEXT_PUBLIC_PRISM_EDITION=oss
COPY --from=build /src/package.json /src/package-lock.json ./
COPY --from=build /src/apps/web/package.json apps/web/package.json
COPY --from=build /src/node_modules node_modules
COPY --from=build /src/apps/web/.next apps/web/.next
COPY --from=build /src/apps/web/next.config.mjs apps/web/next.config.mjs
EXPOSE 3000
CMD ["npm", "--workspace", "apps/web", "run", "start", "--", "--hostname", "0.0.0.0", "--port", "3000"]
