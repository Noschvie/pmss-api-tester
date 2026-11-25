# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (c) 2026 Noschvie
# PMSS API Tester - https://github.com/Noschvie/pmss-api-tester

FROM golang:1.26.0-bookworm AS go
FROM mcr.microsoft.com/playwright:v1.56.1-noble
COPY --from=go /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}" \
    NODE_PATH=/opt/e2e/node_modules \
    E2E_VISUAL_CONTAINER=1 \
    TZ=UTC
WORKDIR /opt/e2e
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
WORKDIR /work
CMD ["node", "/opt/e2e/node_modules/@playwright/test/cli.js", "test", "--project=visual"]
