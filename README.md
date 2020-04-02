# http-server
This is a very basic HTTP server which is intended to debug HTTP clients. It writes requests to the standard output.
Initially I used it to test, if the settings of the fluentd splunk forwarders really do what I expected. For this purpose there is the `-hec` option which tries to simulate a HTTP Event Collector.

## Usage
* Accept requests on `0.0.0.0:8080` an write request headers and body to standard output:
```
http-server
```

* Run in HTTP Event Collector Mode
```
http-server -hec
```
This tries to read one JSON obejct per line and pretty-prints it to standard output.
If you don't show the headers you can pipe the output directly to `jq`:
```
http-server -header=false -hec | jq '.event.log'
```

### TLS
* Generate TLS certificate and key
```
oenssl req -x509 -newkey rsa:4096 -keyout tls.key -out tls.crt -days 365 -nodes -subj '/CN=simple-server' -extensions v3_req -config <( echo -e "[req]\ndistinguished_name=req\n[v3_req]\nsubjectAltName = @alt_names\n[alt_names]\nDNS.1 = localhost\nDNS.2 = simple-server.local\nIP.1 = 127.0.0.1" )
```
* Serve TLS
```
http-server -tls -addr :8443
```
By default it uses `tls.key` and `tls.cert` in the current directory. This can be overwritten with `-cert` and `-key`

## Docker
* Run
```
docker run -p 8080:8080 --rm dsbrng25b/http-server -hec
```

* Test
```
curl -d '{"foo": "bar", "bla": "abc"}' http://localhost:8080
```
