FROM golang:1.22-alpine AS build
WORKDIR /app

COPY go.mod ./
COPY main.go ./

RUN CGO_ENABLED=0 go build -o /bin/fake-cbw-device ./main.go

FROM gcr.io/distroless/static-debian12:nonroot
ENV PORT=8080
EXPOSE 8080

COPY --from=build /bin/fake-cbw-device /fake-cbw-device

ENTRYPOINT ["/fake-cbw-device"]
