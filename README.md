# httpdx
HTTP server with TCP proxies

## Config File Example

./httpdx.yml

```
client:
  server_url: "ws://localhost:7000"
  services:
    - name: ssh
      local_addr: :25000
      
server:
  addr: ":7000"
  routes:
    http:
      /:
        addr: 127.0.0.1:80
        
      # proxify /my-dir as / to destination and pass '/my-dir' 
      # into request header 'path_header' (default value is 'X-Forwarded-Prefix')
      /my-dir:
        addr: 127.0.0.1:80
        dir: true
        path_header: X-Forwarded-Prefix

    tcp_sockets:
      ssh: localhost:22
```

## Server

Starts the server.

Runs `httpdx -h` to usage.

`httpdx` or `httpdx -config ./httpdx.yml`


## Client

Starts the cliente to dispose remote tcp_sockets into local addr.

Runs `httpdx client -h` to usage.

Pass services as args `SERVICE_NAME@LOCAL_ADDR`.

- `httpdx client`, or 
- `httpdx -config ./httpdx.yml client`, or
- `httpdx client ssh@localhost:26000 other@localhost:26001`, or
- `httpdx -config ./httpdx.yml client ssh@localhost:26000 other@localhost:26001` 
