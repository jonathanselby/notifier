# Bafflewick example

```bash
go run ./cmd/notifier -config examples/bafflewick/config.json -request examples/bafflewick/request.json
cat /tmp/bafflewick-alerts.log
```

The default example uses only the local sink and therefore works without a network. External HTTP channels are optional; in exercise mode they are suppressed rather than accidentally sent.

## Per-recipient exercise

```bash
go run ./cmd/notifier -config examples/bafflewick/per-recipient-config.json -request examples/bafflewick/per-recipient-exercise.json
```

Expected: the local sink is `delivered`; the webhook, `priya.n` (email) and `tom.b` (SMS) are `suppressed` because the request is an exercise; `mallory` is `failed` with `address refused` because the address is outside the mail channel's pattern. The address check runs even in an exercise, and nothing reaches the network. Exit code 1, because one delivery failed. No address appears in the output.
