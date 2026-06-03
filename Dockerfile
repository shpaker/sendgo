# sendgo — single-binary, single-container build.
#
# Stage 1: собираем фронт через webpack (как раньше).
# Stage 2: копируем dist/+locales/ в server/static/, собираем Go-бинарь с //go:embed.
# Stage 3: distroless runtime, один бинарь, ENTRYPOINT — он же.

# ---------- stage 1: frontend ----------
FROM node:16-alpine AS frontend
WORKDIR /app
COPY package*.json ./
RUN PUPPETEER_SKIP_CHROMIUM_DOWNLOAD=true npm ci
COPY app/ ./app/
COPY common/ ./common/
COPY public/ ./public/
COPY assets/ ./assets/
COPY assets_src/ ./assets_src/
COPY build/ ./build/
COPY webpack.config.js postcss.config.js tailwind.config.js browserslist ./
COPY .babelrc* ./
RUN npm run build

# ---------- stage 2: Go build ----------
FROM golang:1.25-alpine AS backend
WORKDIR /src
COPY server/ ./server/

# Эмбедим артефакты сборки фронта в исходники Go перед `go build`.
COPY --from=frontend /app/dist           /src/server/static/dist
COPY --from=frontend /app/public/locales /src/server/static/locales

WORKDIR /src/server
RUN go mod download
ARG COMMIT=unknown
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X github.com/sendgo/sendgo/server/internal/config.Version=${VERSION} -X github.com/sendgo/sendgo/server/internal/config.Commit=${COMMIT}" \
    -o /out/sendgo ./cmd/sendgo

# ---------- stage 3: runtime ----------
FROM gcr.io/distroless/static:nonroot
COPY --from=backend /out/sendgo /sendgo
USER nonroot:nonroot
EXPOSE 1443
ENTRYPOINT ["/sendgo"]
