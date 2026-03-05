# fake-cbw-device

Simple fake ControlByWeb-style HTTP device, built in Go and containerized with Docker.

## What it does

- Exposes `GET /state.xml` and `GET /state.json`
- Supports read/write via query params on either state route
  - Example: `/state.xml?relay1=1&register1=123&vin=12.3`
- Returns device-like XML/JSON payloads with string values
- Keeps state in memory only (no persistence)
- Includes `GET /config` to view and adjust which datapoints are currently exposed

Default datapoints:

- `digitalInput1`, `digitalInput2`, `digitalInput3`, `digitalInput4`
- `relay1`, `relay2`, `relay3`, `relay4`
- `vin`, `register1`, `lat`, `long`
- `utcTime`, `timezoneOffset`, `serialNumber`, `minRecRefresh`

## Run with Docker

Build:

```bash
docker build -t fake-cbw-device .
```

Run:

```bash
docker run --rm -p 8080:8080 fake-cbw-device
```

## Try it

```bash
curl "http://localhost:8080/state.xml"
curl "http://localhost:8080/state.json"
curl "http://localhost:8080/state.xml?register1=123"
curl "http://localhost:8080/state.json?relay1=1&vin=12.3"
```

Config endpoint examples:

```bash
curl "http://localhost:8080/config"
curl "http://localhost:8080/config?remove=relay4"
curl "http://localhost:8080/config?add=relay4"
curl "http://localhost:8080/config?set=vin,utcTime,serialNumber"
curl "http://localhost:8080/config?reset=1"
```

## Local run (without Docker)

```bash
go run ./main.go
```
