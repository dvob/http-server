# http-server
This started as a very basic HTTP server which I regularly use to debug HTTP-Clients, Loadbalancers and so forth.

For version 0.1.x I started to cleanup the code and make the configuration more versatile.
See `http-server -h` for available options.

## Usage
```
http-server [OPTIONS] [CONFIG]
```

* Accept requests on `0.0.0.0:80` and return
```
http-server
```

* Listen on port `8080`:
```
http-server -addr :8080
```

## Handler Configuration
By default the server just returns the status code `200` and sends `ok` in the response body. But you can configure in detail what action should be performed.

There are two things you can configure for each path:
* A number of ***middleware***
* One ***handler***

A request then runs through each middleware in the order in which the middlewares are specified and at the end a handler produces the response.
In the following example a request would first run through the `log` and `json` middleware and then the `static` handler would produce the response.
```
http-server log json static 
```

You can specify a paths as follows. In the following example requests to `/info` would run through the `log` middleware and then the `info` handler would return some information about the host and the request.
Every other request (`/`) would be sent through the  `log` middleware and then return the `static` handler would return a static response (`ok`).
```
http-server /info: log info /: log static
```

## TLS
If you enable TLS the `http-server` changes it's default port to `:443`.

### Let's Encrypt
If you use the `-tls-hosts` option the server will automatically perform the ACME with Let's Encrypt for the specified hosts:
```
http-server -tls-hosts www.myhost1.com,myhost1.com
```

### Certifictes
Generate TLS certificate and key:
```
oenssl req -x509 -newkey rsa:4096 -keyout tls.key -out tls.crt -days 365 -nodes -subj '/CN=simple-server' -extensions v3_req -config <( echo -e "[req]\ndistinguished_name=req\n[v3_req]\nsubjectAltName = @alt_names\n[alt_names]\nDNS.1 = localhost\nDNS.2 = simple-server.local\nIP.1 = 127.0.0.1" )
```
Alternatively you could use [pcert](https://github.com/dvob/pcert) to create a certificate in a much simpler way:
```
pcert create tls --subject "/CN=simple-server" --dns localhost --dns simple-server.local --ip 127.0.0.1
```

And then use the following options:
```
http-server -tls-cert tls.crt -tls-key tls.key
```

## Docker
* Run
```
docker run -p 8080:8080 --rm dvob/http-server
```
