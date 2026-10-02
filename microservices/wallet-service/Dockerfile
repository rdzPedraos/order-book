# Built from the repository root: docker build -f microservices/wallet-service/Dockerfile .
# Only go.mod, go.sum, shared/ and this service are copied, so a change in
# another service does not rebuild this image.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY shared/ shared/
COPY microservices/wallet-service/ microservices/wallet-service/
RUN CGO_ENABLED=0 go build -o /out/wallet-service ./microservices/wallet-service

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/wallet-service /wallet-service
ENTRYPOINT ["/wallet-service"]
