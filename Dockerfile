FROM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tagsvc ./cmd/tagsvc

# Статический бинарник, шаблон вшит через embed: кроме него в образе ничего не нужно.
FROM scratch
COPY --from=build /out/tagsvc /tagsvc
USER 65534:65534
EXPOSE 8080
ENTRYPOINT ["/tagsvc"]
