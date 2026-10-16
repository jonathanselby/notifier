# Bafflewick example

```bash
go run ./cmd/notifier -config examples/bafflewick/config.json -request examples/bafflewick/request.json
cat /tmp/bafflewick-alerts.log
```

The default example uses only the local sink and therefore works without a network. External HTTP channels are optional; in exercise mode they are suppressed rather than accidentally sent.
