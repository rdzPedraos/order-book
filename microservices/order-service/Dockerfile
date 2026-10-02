# Built from the repository root: docker build -f microservices/order-service/Dockerfile .
# Only go.mod, go.sum, shared/ and this service are copied, so a change in
# another service does not rebuild this image.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY shared/ shared/
COPY microservices/order-service/ microservices/order-service/
RUN CGO_ENABLED=0 go build -o /out/order-service ./microservices/order-service

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/order-service /order-service
ENTRYPOINT ["/order-service"]
