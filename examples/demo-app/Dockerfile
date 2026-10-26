FROM golang:1.27 AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/demo-app ./cmd/demo-app

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/demo-app /demo-app
EXPOSE 8080
ENTRYPOINT ["/demo-app"]
