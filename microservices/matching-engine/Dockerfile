# Built from the repository root: docker build -f microservices/matching-engine/Dockerfile .
# Only go.mod, go.sum, shared/ and this service are copied, so a change in
# another service does not rebuild this image.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY shared/ shared/
COPY microservices/matching-engine/ microservices/matching-engine/
RUN CGO_ENABLED=0 go build -o /out/matching-engine ./microservices/matching-engine

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/matching-engine /matching-engine
ENTRYPOINT ["/matching-engine"]
