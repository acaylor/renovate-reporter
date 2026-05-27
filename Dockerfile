# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY *.go ./
COPY index.html ./

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/renovate-reporter .

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/renovate-reporter /renovate-reporter
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/renovate-reporter"]
CMD ["/logs"]
