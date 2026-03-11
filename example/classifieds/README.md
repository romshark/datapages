# Demo: Classifieds

A demo application resembling an online classifieds marketplace.

The code you'd write is in
[app](https://github.com/romshark/datapages/tree/main/example/classifieds/app)
(the "source package").
The code that the generator produces is in
[datapagesgen](https://github.com/romshark/datapages/tree/main/example/classifieds/datapagesgen).

## Development Mode

```sh
make dev
```

You can then access:
- Preview: http://localhost:52000/
- Grafana Dashboards: http://localhost:3000/
- Prometheus UI: http://localhost:9091/

You can install [k6](https://k6.io/) and run `make load` in the background
to generate random traffic.
Increase the number of virtual users (`VU`) to apply more load to the server when needed.

## Production Mode

```sh
make stage
```
