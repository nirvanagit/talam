# Packages a prebuilt talam binary (see `make build`) into a distroless
# image. The Go build itself happens on the host via `go build`, not inside
# Docker — faster locally and avoids re-downloading modules in-container.
# Selected by --build-arg BINARY=talam-agent|talam-server|talam-operator
# (must match a file under bin/).
FROM gcr.io/distroless/static-debian12:nonroot
ARG BINARY
COPY bin/${BINARY} /talam
ENTRYPOINT ["/talam"]
