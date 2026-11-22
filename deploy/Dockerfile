FROM node:22-bookworm-slim AS web
WORKDIR /src
COPY platform/web/package*.json ./
RUN npm ci
COPY platform/web/index.html platform/web/vite.config.js ./
COPY platform/web/src ./src
RUN npm run build

FROM golang:1.26-bookworm AS control
WORKDIR /src
COPY platform/server/go.mod platform/server/go.sum ./
RUN go mod download
COPY platform/server/cmd ./cmd
COPY platform/server/internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -o /autotest-server ./cmd/server

FROM python:3.13-slim-bookworm
ENV PLAYWRIGHT_BROWSERS_PATH=/opt/browsers PYTHONPATH=/opt/autotest \
    PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 PYTHON_DOTENV_DISABLED=1 \
    LANGSMITH_TRACING=false NEO4J_ENABLED=false
COPY deploy/worker-requirements.txt /tmp/worker-requirements.txt
RUN pip install --no-cache-dir --only-binary=:all: -r /tmp/worker-requirements.txt \
    && python -m playwright install --with-deps --only-shell chromium \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /state/cache \
    && chown -R 10001:10001 /state
COPY --from=control /autotest-server /usr/local/bin/autotest-server
COPY --from=web /src/dist /opt/autotest/web
COPY agents/*.py /opt/autotest/agents/
COPY auto_ai/*.py /opt/autotest/auto_ai/
RUN test -f /opt/autotest/agents/worker_credentials.py
USER 10001:10001
WORKDIR /state
EXPOSE 8090
CMD ["autotest-server", "-addr", "0.0.0.0:8090", "-db", "/state/platform.db", "-web", "/opt/autotest/web"]
