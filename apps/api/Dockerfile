FROM golang:1.26.3-bookworm AS build

WORKDIR /src/apps/api
COPY apps/api/go.mod apps/api/go.sum ./
RUN go mod download

COPY apps/api/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/srapi ./cmd/srapi

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/srapi /srapi
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/srapi"]
