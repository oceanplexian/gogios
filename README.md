# Gogios

A single-binary Go replacement for Nagios 4.1.1 that reads the same configuration format and provides full compatibility with the Nagios ecosystem.

## Features

- **Drop-in Nagios replacement** — reads standard `nagios.cfg` and all object configuration files
- **Livestatus API** — built-in LQL query server (TCP and Unix socket) compatible with Thruk
- **Single binary** — no CGIs, no Apache, no NEB modules
- **Check engine** — bounded worker pool, interleaved scheduling, SOFT/HARD state machine
- **Notifications** — escalations, dependencies, acknowledgements, contact filtering
- **Downtime** — fixed, flexible, and triggered scheduled downtimes
- **External commands** — command pipe compatible with Nagios external command interface
- **State persistence** — retention.dat and status.dat compatibility

## Building

```bash
go build -o gogios ./cmd/gogios
```

## Usage

```bash
# Verify configuration
./gogios -v /path/to/nagios.cfg

# Run in foreground
./gogios /path/to/nagios.cfg

# Run as daemon
./gogios -d /path/to/nagios.cfg
```

### Configuration

Gogios reads standard Nagios 4.x configuration. Key additions:

```ini
# Livestatus over TCP (for Thruk)
livestatus_tcp=0.0.0.0:6557

# Livestatus over Unix socket
query_socket=/var/lib/nagios/rw/live
```

### Flags

| Flag | Description |
|------|-------------|
| `-v` | Verify configuration (use twice for verbose) |
| `-d` | Run as daemon |
| `-s` | Test scheduling |
| `-V` | Print version |

## Architecture

```
cmd/gogios/          Main entry point
internal/
  api/               StateProvider, CommandSink interfaces
    livestatus/       LQL query server (tables, filters, stats, output)
  checker/           Check execution engine, host/service state machines
  config/            Nagios config parser (main config, objects, templates)
  dependency/        Host and service dependency evaluation
  downtime/          Scheduled downtime and comment management
  extcmd/            External command FIFO processor
  freshness/         Passive check freshness monitoring
  logging/           Log file management with rotation
  macros/            Nagios macro expansion ($HOSTNAME$, etc.)
  notify/            Notification engine with escalations
  objects/           Object types (Host, Service, Contact, etc.) and store
  perfdata/          Performance data processing
  scheduler/         Event queue and check scheduling
  status/            status.dat writer and retention.dat persistence
```

## Livestatus Compatibility

The built-in Livestatus server implements the LQL protocol as expected by Thruk 3.26:

- Tables: hosts, services, hostgroups, servicegroups, contacts, contactgroups, commands, timeperiods, comments, downtimes, status, columns, log
- Filters with `And:`, `Or:`, `Negate:` combining
- Stats with `StatsAnd:`, `StatsOr:` combining and aggregation functions (sum, avg, min, max, std)
- Output formats: json, wrapped_json, csv
- fixed16 response headers
- KeepAlive connections
- External commands via `COMMAND` requests

## License

Private.
