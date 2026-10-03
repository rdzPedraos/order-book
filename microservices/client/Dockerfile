# Built from the repository root: docker build -f microservices/client/Dockerfile .
# Only go.mod, go.sum and this service are copied: it imports nothing from
# shared/, so a change there or in another service does not rebuild this image.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY microservices/client/ microservices/client/
RUN CGO_ENABLED=0 go build -o /out/client ./microservices/client

# static/ sits at /static, the working directory of the binary.
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /
COPY --from=build /out/client /client
COPY --from=build /src/microservices/client/static /static
ENTRYPOINT ["/client"]
