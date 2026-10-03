# Built from the repository root: docker build -f tools/loadgen/Dockerfile .
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY shared/ shared/
COPY tools/loadgen/ tools/loadgen/
RUN CGO_ENABLED=0 go build -o /out/loadgen ./tools/loadgen

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/loadgen /loadgen
ENTRYPOINT ["/loadgen"]
