FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bank ./cmd/bank

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/bank /app/bank
COPY migrations /app/migrations
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/bank"]
