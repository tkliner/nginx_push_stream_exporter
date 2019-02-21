# Nginx PushStream Exporter for Prometheus

This is a simple server implementation that scrapes [Push Stream Module](https://github.com/wandenberg/nginx-push-stream-module) of Nginx for Prometheus.

## Getting Started

To run it:

```bash
./nginx_push_stream_exporter [flags]
```

Help on flags:

```bash
./nginx_push_stream_exporter --help
```

## Usage

Scrape a remote host:

```bash
nginx_push_stream_exporter --nginx.scrape-uri="http://example.com:8080/channels-stats?id=ALL"
```

## License

Apache License 2.0