# httpdx

[![verify](https://github.com/moisespsena-dockers/httpdx/actions/workflows/verify.yml/badge.svg)](https://github.com/moisespsena-dockers/httpdx/actions/workflows/verify.yml)

httpdx exposes the TCP services of a machine — a database, ssh, anything that
listens on a port — through a single HTTP(S) address, and gives them back as
local ports on another machine. On top of that, the server is a small reverse
proxy for HTTP routes and static files.

```
 your machine                                   the server's machine
┌────────────────────────┐                    ┌──────────────────────────────────┐
│ psql -p 25021          │                    │                                  │
│   │                    │   HTTPS (one       │  httpdx server                   │
│   ▼                    │   websocket per    │   /!?name=bd  ──► 127.0.0.1:5432 │
│ httpdx client  ────────┼──── session) ─────►│   /!?name=ssh ──► 127.0.0.1:22   │
│   :25021 → route "bd"  │                    │   /admin/     ──► 127.0.0.1:81   │
│   :25020 → route "ssh" │                    │   /static/    ──► ./static_files │
└────────────────────────┘                    └──────────────────────────────────┘
```

- **Only one port is open on the server** — the HTTP one, usually behind the
  same reverse proxy (nginx…) and TLS certificate as the rest of the site.
- **Each local connection is a websocket** to the server, which opens a TCP
  connection to the route's address and copies bytes both ways.
- **Routes can ask for a user and a password** (HTTP Basic, checked by the
  server); use `https://` in the client's `server_url` so they travel
  encrypted.
- **Idle sessions stay open**: both ends ping (keep alive), so a reverse proxy
  in front of the server does not close a `psql` or `ssh` left idle.

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [Commands](#commands)
- [Configuration](#configuration)
  - [Client](#client)
  - [Server: TCP routes](#server-tcp-routes)
  - [Server: HTTP routes](#server-http-routes)
- [Authentication](#authentication)
- [Keep alive](#keep-alive)
- [Behind nginx](#behind-nginx)
- [Docker](#docker)
- [Development](#development)

## Install

- **A release** (linux/amd64): download the archive of the latest
  [release](https://github.com/moisespsena-dockers/httpdx/releases) — it has
  the binary, this README, the license and the config template.
- **With Go**:

  ```sh
  go install github.com/moisespsena-dockers/httpdx@latest
  ```

- **From the source**:

  ```sh
  git clone https://github.com/moisespsena-dockers/httpdx
  cd httpdx && CGO_ENABLED=0 go build -o httpdx .
  ```

- **Docker**: see [Docker](#docker).

`httpdx info` prints the commit, the build time and the Go settings of the
binary.

## Quick start

Expose a PostgreSQL of the server as port 25021 of your machine.

**1. On the server**, `httpdx.yml`:

```yaml
server:
  addr: ":8080"
  tcp_sockets:
    routes:
      bd:
        addr: 127.0.0.1:5432
        auth:
          user: me
          password: a-long-secret
```

```sh
httpdx            # "server" is the default command; ./httpdx.yml the default config
```

**2. On your machine**, `httpdx.yml`:

```yaml
client:
  server_url: "https://example.com"   # the server, or the site in front of it
  routes:
    - name: bd
      local_addr: 127.0.0.1:25021
      auth:
        user: me
        password: a-long-secret
```

```sh
httpdx client
psql -h 127.0.0.1 -p 25021 -U postgres mydb
```

The client checks the server once when it starts (a websocket to it) and
stops if it cannot reach it; after that, each local connection opens its own
websocket.

## Commands

```
httpdx [-config FILE] [COMMAND] [OPTIONS] [ARGS]
```

`-config` comes **before** the command. Without it, the config is
`<binary name>.yml` in the working directory — `./httpdx.yml`.

| Command | What it does |
|---|---|
| `server` (default) | Runs the server (the `server:` section). `-addr ADDR` overrides `server.addr`. |
| `client` | Runs the client (the `client:` section). `-server-url URL` overrides `client.server_url`. |
| `create-config` | Writes a config file from the template, every option commented. |
| `info` | Prints the build information (`-format` takes a Go template). |

**Client routes as arguments**: `NAME:LOCAL_ADDR` arguments replace the
routes of the config (`LOCAL_ADDR` must have a port):

```sh
httpdx client ssh:127.0.0.1:26000 bd:127.0.0.1:26001
httpdx -config ./other.yml client -server-url https://example.com bd:127.0.0.1:26001
```

**create-config**:

```sh
httpdx create-config -server-addr :8080 -server-url https://example.com -out httpdx.yml
```

| Option | Default | |
|---|---|---|
| `-server-addr` | `:80` | the server's `addr` (a port alone, `8080`, is `:8080`) |
| `-server-url` | `http://127.0.0.1:PORT` | the client's `server_url`; `PORT` is replaced by the server's port |
| `-out` | `-` (stdout) | the file to write |

## Configuration

One YAML file holds both sections; each command reads its own. The template
(`httpdx create-config`, or [config_template.yml](config_template.yml)) has
every option, commented.

### Client

```yaml
client:
  server_url: "https://example.com"
  keep_alive: 25s
  auth:                # default for the routes without their own
    user: me
    password: a-long-secret
  routes:
    - name: ssh                 # a route of the server (tcp_sockets.routes)
      local_addr: :25020        # where this machine listens for it
    - name: bd
      local_addr: 127.0.0.1:25021
      auth:                     # this route's own
        user: other
        password: other-secret
    - name: old
      local_addr: :25022
      disabled: true
```

| Key | Default | |
|---|---|---|
| `server_url` | required | The server (`http://` or `https://`): the websockets go to `<server_url>/!`. |
| `keep_alive` | `25s` | How often an open connection pings the server ([Keep alive](#keep-alive)); `"0"`: never. |
| `auth` | none | Default `user` and `password` of the routes; `disabled: true` sends none. |
| `routes[].name` | required | The name of the route on the server. |
| `routes[].local_addr` | required | The local address to listen on (`:25020` is every interface; `127.0.0.1:25020` this machine only). |
| `routes[].auth` | `auth` | The route's own credentials. |
| `routes[].disabled` | `false` | Not served. |

### Server: TCP routes

```yaml
server:
  addr: ":8080"
  tcp_sockets:
    handshake_timeout: 5      # seconds
    dial_timeout: 5           # seconds
    write_timeout: 5          # seconds
    compression_enabled: false
    keep_alive: 25s
    auth:                     # default for the routes without their own
      user: me
      password: a-long-secret
    routes:
      ssh:
        addr: 127.0.0.1:22
      bd:
        addr: 127.0.0.1:5432
        auth:
          user: other
          password: other-secret
      public:
        addr: 127.0.0.1:7000
        auth:
          disabled: true      # no credentials asked
```

| Key | Default | |
|---|---|---|
| `addr` | | The HTTP address the server listens on. |
| `tcp_sockets.handshake_timeout` | `5` | Seconds for the websocket handshake. |
| `tcp_sockets.dial_timeout` | `5` | Seconds to connect to a route's address. |
| `tcp_sockets.write_timeout` | `5` | Seconds a write to a client may take (a client that stopped reading is let go). |
| `tcp_sockets.compression_enabled` | `false` | Websocket compression. |
| `tcp_sockets.keep_alive` | `25s` | How often the server pings an open connection ([Keep alive](#keep-alive)); `"0"`: never. |
| `tcp_sockets.auth` | none | Default credentials of the routes. |
| `tcp_sockets.routes.NAME.addr` | required | The TCP address the route connects to. |
| `tcp_sockets.routes.NAME.auth` | `tcp_sockets.auth` | The route's own credentials; `disabled: true` asks none. |
| `tcp_sockets.routes.NAME.disabled` | `false` | Not served. |

The websocket address is `<server>/!?name=NAME`. A client gets a text message
starting with `ERROR:` (and the connection closed) when the route is not
registered, the credentials are wrong or the route's address does not answer;
the client logs it.

### Server: HTTP routes

The server is also a reverse proxy, by path:

```yaml
server:
  addr: ":8080"
  not_found: ./404.html        # the page of an unknown path; none: httpdx's
  not_found_disabled: false    # true: no page for an unknown path
  http:
    routes:
      /:
        addr: 127.0.0.1:80           # every path not matched below
      /admin/:
        addr: 127.0.0.1:81           # /admin/x is proxied as /admin/x
      /app:
        addr: 127.0.0.1:3000
        path_strip: true             # /app/x is proxied as /x …
        path_header: X-Forwarded-Prefix   # … with "X-Forwarded-Prefix: /app"
      /static/:
        dir: ./static_files          # files of a directory
        path_override: "%[3]s/%[1]s" # /static/f.txt → ./static_files/static/f.txt
```

| Key | Default | |
|---|---|---|
| `addr` | | The HTTP address to proxy to. |
| `path_strip` | `false` | Proxy without the route's path, and tell it in `path_header`. |
| `path_header` | `X-Forwarded-Prefix` | The header of the stripped path. |
| `dir` | | Serve the files of this directory instead of proxying. |
| `path_override` | `%[1]s` | With `dir`: the file path, a Go format of 1) the requested path without the route's, 2) `dir`, 3) the route's path. |
| `disabled` | `false` | Not served. |

A request with the header `X-Httpdx-Handle-Fallback: false` gets no
not-found page (the `/` route's answer, or nothing).

## Authentication

- The client sends the route's credentials (its `auth`, else the client's
  `auth`) as HTTP Basic on the websocket request.
- The server checks them against the route's `auth`, else
  `tcp_sockets.auth`; a route whose `auth` is `disabled: true` (or with no
  `auth` at all) asks for none.
- Credentials travel in the clear over `http://`: use `https://` (a TLS
  reverse proxy in front of the server — [Behind nginx](#behind-nginx)).
- They are kept in the config file: protect it (`chmod 600`).

## Keep alive

A reverse proxy in front of the server (nginx: `proxy_read_timeout`, 60s by
default) closes a websocket idle longer than its timeout: an open session
without traffic — a `psql`, an `ssh` — dropped. So both ends ping an open
connection, and close it when the other does not answer for 3 intervals:

- `client.keep_alive` — default `25s`; `"0"`: never;
- `server.tcp_sockets.keep_alive` — default `25s`; `"0"`: never. It also
  keeps alive the clients of before (without their own keep alive), and lets
  go of a client gone without closing.

Either is enough to keep a session open; keep both on. Choose an interval
below the reverse proxy's timeout.

## Behind nginx

The websockets go to the path `/!`: nginx must pass the upgrade.

```nginx
location = /! {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_set_header Host $host;
    proxy_read_timeout 300s;   # the keep alive (25s) stays well below it
}
```

The HTTP routes of the server go through an ordinary `location` with
`proxy_pass`.

## Docker

The image `moisespsena/httpdx` ([Dockerfile](Dockerfile)) has the binary in
`/bin/httpdx`, its working directory and config in `/config`
(`/config/httpdx.yml`, a template created at build), and runs the server.

```sh
# the server, with its config
docker run -d -p 8080:80 -v ./config:/config moisespsena/httpdx

# the client: the routes' local ports published
docker run --rm -v ./config:/config -p 25020-25022:25020-25022 moisespsena/httpdx httpdx client
```

## Development

| | |
|---|---|
| `make test` | `go test -race` — the client and the server, in process. |
| `make test-compose` | The tests in containers ([test/compose](test/compose/compose.yml)): two servers (with and without keep alive), each behind a front that drops connections idle 3s (as a reverse proxy does), an echo server as the upstream, four clients (keep alive on the client only, on the server only, on both with authentication, and on neither: the control) and a check — the sessions idle 7s must survive, the control must drop, and the server's errors (a route not registered, an upstream down, a wrong password) must come as such. Needs Docker with the compose plugin; the binaries are built here. |
| `make lint` | `gofmt`, `go vet` and `staticcheck` (`STATICCHECK_VERSION`, run by `go run`). |
| `make verify` | `lint`, `test` and `test-compose`: what a commit must pass. |

**CI** (GitHub Actions):

- [verify](.github/workflows/verify.yml): `make verify` on every push to
  `main`, on pull requests and by hand, and a snapshot build of the release
  config.
- [release](.github/workflows/release.yml): on a tag `v*`, `make verify` and
  then [GoReleaser](.goreleaser.yaml) publishes the release (linux/amd64).

To release: `git tag v1.2.3 && git push origin v1.2.3`.

**Layout**:

| | |
|---|---|
| `main.go` | The commands. |
| `client/` | The client: the local listeners, a websocket per connection, the keep alive. |
| `server/` | The server: the websocket ↔ TCP proxy (`tcpproxy.go`), the HTTP routes and the not-found page (`runner.go`). |
| `internal/testtools/` | The programs of the compose tests: `echo`, `front`, `check`. |
| `test/compose/` | The compose tests: their compose file, image and configs. |

## License

[MIT](LICENSE).
