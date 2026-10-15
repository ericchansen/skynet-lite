FROM golang:1.26.2-alpine AS build
WORKDIR /src
COPY go.mod main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /compat .

FROM scratch
COPY --from=build /compat /compat
USER 65532:65532
ENTRYPOINT ["/compat"]
