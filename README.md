# Prometheus Ethermine Exporter

[![GitHub release](https://img.shields.io/github/v/release/HON95/prometheus-ethermine-exporter?label=Version)](https://github.com/HON95/prometheus-ethermine-exporter/releases)
[![CI](https://github.com/HON95/prometheus-ethermine-exporter/workflows/CI/badge.svg?branch=master)](https://github.com/HON95/prometheus-ethermine-exporter/actions?query=workflow%3ACI)
[![FOSSA status](https://app.fossa.com/api/projects/git%2Bgithub.com%2FHON95%2Fprometheus-ethermine-exporter.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2FHON95%2Fprometheus-ethermine-exporter?ref=badge_shield)
[![Docker pulls](https://img.shields.io/docker/pulls/hon95/prometheus-ethermine-exporter?label=Docker%20Hub)](https://hub.docker.com/r/hon95/prometheus-ethermine-exporter)

An exporter for the [Ethermine, Ethpool and Flypool API](https://ethermine.org/api).

## Usage

### Exporter (Docker)

Example `docker-compose.yml`:

```yaml
version: "3.7"

services:
  ethermine-exporter:
    image: hon95/hon95/prometheus-ethermine-exporter:1
    #command:
    #  - '--endpoint=:8080'
    #  - '--debug'
    user: 1000:1000
    environment:
      - TZ=Europe/Oslo
    ports:
      - "8080:8080/tcp"
```

### Prometheus

Example `prometheus.yml`:

```yaml
global:
    scrape_interval: 15s
    scrape_timeout: 10s

scrape_configs:
  - job_name: ethermine-pool
    # Limit due to API rate restriction
    scrape_interval: 5m
    metrics_path: /pool
    static_configs:
      - targets:
          - ethermine-exporter:8080

  - job_name: ethermine-miner
    # Limit due to API rate restriction
    scrape_interval: 5m
    metrics_path: /miner
    static_configs:
      - targets:
          - F6403152cAd46F2224046C9B9F523d690E41Bffd
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: ethermine-exporter:8080
```

## Configuration

### Docker Image Versions

Use `1` for stable v1.Y.Z releases and `latest` for bleeding/unstable releases.

## Metrics

See the [pool example output](examples/output-pool.txt) and the [miner example output](examples/output-miner.txt) (I'm too lazy to create a pretty table right now).

### Docker

See the dev/example Docker Compose file: [docker-compose.yml](dev/docker-compose.yml)

## Development

- Build (Go): `go build -o prometheus-ethermine-exporter`
- Lint: `golint ./..`
- Build and run along Traefik (Docker Compose): `docker-compose -f dev/docker-compose.yml up --force-recreate --build`

## License

GNU General Public License version 3 (GPLv3).
