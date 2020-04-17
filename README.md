# stream-tracker

stream-tracker authenticates and tracks active streams. It is designed for
use with [nginx-rtmp](https://github.com/arut/nginx-rtmp-module).

## Arguments

* `--bind`: The `address:port` to listen on (`0.0.0.0:8080` by default)
* `--config`: The path to the config file described below (mandatory)
* `--redis-url`: The URL of the redis server `stream-tracker` will use to store state and publish updates
* `--read-password`: The password protecting the API endpoints, if specified (defaults to no password) 

## Configuration

`stream-tracker`'s configuration file is a yaml file with a single key, `mapping`, which
should have a mapping from some sort of stream identifier to the stream
key for that stream. For example:

```yaml
mapping:
  "1": "abcd-1234"
  "2": "sdgh-4632"
```

No constraints are placed on either the key or value except that they must
both be strings.

## Endpoints

### `/notify`

* `/notify/publish`: intended for nginx-rtmp's `on_publish`
* `/notify/publish_done`: intended for nginx-rtmp's `on_publish_done`

### `/api`

* `/api/streams`: returns a map from stream IDs to stream keys, as well as
whether that stream is currently broadcasting
* `/api/stream_updates`: [Server sent events](https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events)
endpoint that is sent a message every time a stream starts or stops broadcasting.
