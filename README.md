# cbw-server

Simple fake ControlByWeb-style HTTP device, built in Go and containerized with Docker.

## What it does

- Exposes `GET /state.xml` and `GET /state.json`
- Supports read/write via query params on either state route
  - Example: `/state.xml?relay1=1&register1=123&vin=12.3`
- Returns device-like XML/JSON payloads with string values
- Keeps state in memory only (no persistence)
- Includes `GET /config` to view and adjust which datapoints are currently exposed
- Lets you force response protocol via flags: `--http0.9`, `--http1.0`, `--http1.1`
- Uses a default `serialNumber` MAC address of `DE:AD:BE:EF:00:01` (override with `--mac`)

Default datapoints:

- `digitalInput1`, `digitalInput2`, `digitalInput3`, `digitalInput4`
- `relay1`, `relay2`, `relay3`, `relay4`
- `vin`, `register1`
- `utcTime`, `timezoneOffset`, `serialNumber`, `minRecRefresh`

## Run with Docker

Build:

```bash
docker build -t cbw-server .
```

Run:

```bash
docker run --rm -p 8080:8080 cbw-server
```

Run with a forced response protocol (example: HTTP/1.0):

```bash
docker run --rm -p 8080:8080 cbw-server --http1.0
```

Run with a custom MAC address exposed as `serialNumber`:

```bash
docker run --rm -p 8080:8080 cbw-server --mac DE:AD:BE:EF:CA:FE
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

## Response protocol mode

Use exactly one of these flags to control the protocol used in responses:

- `--http1.1` (default if no protocol flag is set)
- `--http1.0`
- `--http0.9`

Use this flag to set the `serialNumber` MAC value:

- `--mac DE:AD:BE:EF:00:01` (default)

Examples:

```bash
# HTTP/1.1 responses (default)
go run ./src --http1.1

# HTTP/1.0 responses
go run ./src --http1.0

# HTTP/0.9 style responses (body only)
go run ./src --http0.9
curl --http0.9 "http://localhost:8080/state.xml"

# Custom MAC address exposed as serialNumber
go run ./src --mac DE:AD:BE:EF:CA:FE
```

## Local run (without Docker)

```bash
cd src && go run .
```
