# httpdx
HTTP server with TCP proxies

## Config File Example

./httpdx.yml

```
client:
  server_url: "ws://localhost:7000"
  routes:
    - name: ssh
      local_addr: :25000
      
server:
  addr: ":7000"
  
  # not found HTML file to handles not found error.
  # If not set, uses default not found handler message.
  not_found: "my_not_found.html"
  
  # if is true, disables not found handles
  not_found_disabled: false
  
  tcp_sockets:
    # timeouts is in seconds (default is 5s).
    handshake_timeout: 5
    dial_timeout: 5
    write_timeout: 5
    
    compression_enabled: false
    
    routes:
      ssh: 
        addr: localhost:22
        disabled: false
    
  
  http:
    routes:
      /:
        addr: 127.0.0.1:80
        disabled: false
        
      # proxify /my-dir as / to destination and pass '/my-dir' 
      # into request header 'path_header' (default is 'X-Forwarded-Prefix')
      /my-dir:
        addr: 127.0.0.1:80
        dir: true
        path_header: X-Forwarded-Prefix
        disabled: false
```

## Server

Starts the server.

Runs `httpdx -h` to usage.

`httpdx` or `httpdx -config ./httpdx.yml`

if requests contains header `X-Httpdx-Handle-Fallback: false`, disables Not Found handlers.

## Client

Starts the cliente to dispose remote tcp_sockets into local addr.

Runs `httpdx client -h` to usage.

Pass services as args `SERVICE_NAME@LOCAL_ADDR`.

- `httpdx client`, or 
- `httpdx -config ./httpdx.yml client`, or
- `httpdx client ssh@localhost:26000 other@localhost:26001`, or
- `httpdx -config ./httpdx.yml client ssh@localhost:26000 other@localhost:26001` 
