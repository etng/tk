# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS build

ARG TARGETOS
ARG TARGETARCH

WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/docker-service-bridge .

FROM docker:27-cli

ENV TOOLDKIT_ALLOW_CONTAINER_BIND=1
EXPOSE 17321

ENTRYPOINT ["docker-service-bridge"]
CMD ["-addr", "0.0.0.0:17321"]

COPY --from=build /out/docker-service-bridge /usr/local/bin/docker-service-bridge
