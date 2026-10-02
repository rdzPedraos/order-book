# Built from the repository root: docker build -f microservices/market-service/Dockerfile .
# Only go.mod, go.sum, shared/ and this service are copied, so a change in
# another service does not rebuild this image.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY shared/ shared/
COPY microservices/market-service/ microservices/market-service/
RUN CGO_ENABLED=0 go build -o /out/market-service ./microservices/market-service

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/market-service /market-service
ENTRYPOINT ["/market-service"]
