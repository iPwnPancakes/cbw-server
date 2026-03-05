FROM golang:1.22-alpine AS build
WORKDIR /app

COPY src/go.mod ./go.mod
COPY src/main.go ./main.go

RUN CGO_ENABLED=0 go build -o /bin/cbw-server ./main.go

FROM gcr.io/distroless/static-debian12:nonroot
ENV PORT=8080
EXPOSE 8080

COPY --from=build /bin/cbw-server /cbw-server

ENTRYPOINT ["/cbw-server"]
