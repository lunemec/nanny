
# Nanny
[![Build Status](https://travis-ci.org/lunemec/nanny.svg?branch=master)](https://travis-ci.org/lunemec/nanny) [![Go Report Card](https://goreportcard.com/badge/github.com/lunemec/nanny)](https://goreportcard.com/report/github.com/lunemec/nanny) [![Maintainability](https://api.codeclimate.com/v1/badges/224b9390145c2e5a8046/maintainability)](https://codeclimate.com/github/lunemec/nanny/maintainability) [![codecov](https://codecov.io/gh/lunemec/nanny/branch/master/graph/badge.svg)](https://codecov.io/gh/lunemec/nanny)

Nanny is a monitoring tool that monitors the **absence of activity**.

Nanny runs an API server, which expects to be called every N seconds, and if no such call is made, Nanny notifies you.

Nanny can notify you via these channels (for now):
* print text to stderr
* email
* sentry
* sms (twilio)
* slack (webhook)

## Example
Run API server:
```bash
$ LOGXI=* ./nanny
14:21:07.969059 INF ~ Using config file
   path: nanny.toml
14:21:07.977322 INF ~ Nanny listening addr: localhost:8080
```
Call it via curl:
```bash
curl http://localhost:8080/api/v1/signal --data '{ "name": "my awesome program", "notifier": "stderr", "next_signal": "5s" }'
```
With this call, you tell nanny that if program named `my awesome program` does not call again within `next_signal` (5s), it should notify you using `stderr` notifier. Additionally, nanny appends the IP or `X-Forwarded-For` HTTP header to the program name. You can disable this behaviour by sending a `X-Dont-Modify-Name` along with the request.

After 5s pass, nanny prints to *stderr*:
```bash
2018-06-26T14:24:29+02:00: Nanny: I haven't heard from "my awesome program@127.0.0.1" in the last 5s! (Meta: map[])
```

## Installation
The easiest way is to download .tar.gz from **releases** section, edit `nanny.toml` and run it.

Or you can clone this repository and compile it yourself:
```bash
git clone https://github.com/lunemec/nanny.git
cd nanny
make build
```

Note that Nanny requires Go >= 1.8 to run.

An alternative way of using Nanny is to run it inside a Docker container. You must build the Nanny Docker image first by using the command `make docker`. After that a Nanny Docker instance can be started like this:
```bash
docker run -d -v ${PWD}/nanny.toml:/nanny.toml -p 8080:8080 -e NANNY_ADDR=0.0.0.0:8080 lunemec/nanny:latest
```
**Note:** Use the `docker run` parameter `-e NANNY_ADDR=0.0.0.0:8080` or set the `addr` configuration inside the `nanny.toml` file to `addr="0.0.0.0:8080"`. If you leave the default setting (`addr="localhost:8080"`) it won't work since `localhost` inside a Docker container is just the container itself. Access from the outside (via the port publishing) would not be possible!

It's also possible to run Nanny using the provided Docker Compose file (see [docker-compose.yml]](docker-compose.yml)):
```yml
docker-compose up -d
```
**Note:** The provided `docker-compose.yml` file assumes that there is a `nanny.toml` file in the same directory as your `docker-compose.yml` file is placed. Feel free to update the volume definition according to your setup inside the Docker Compose file be editing the line `${PWD}/nanny.toml:/nanny.toml`.

## Configuration
See nanny.toml for a configuration example. The fields are self-explanatory (I think). Please create an issue if anything does not make sense!

All enabled notifiers can be used via API, so enable only those you wish to allow.

### ENV variables
ENV variables can be used to override the config file settings. They should be prefixed with `NANNY_` and followed by same name as in `nanny.toml`.

Example:
```
NANNY_NAME="custom name" NANNY_ADDR="localhost:9090" LOGXI=* ./nanny
```

## API
### Nanny version
  Print nanny version.

* **URL**

  /api/version

* **Method:**

  `GET`

* **Success Response:**

  * **Code:** 200
    **Content:** `Nanny vX.Y`

### Signal
  Signal Nanny to register notification with given parameters.

* **URL**

  /api/v1/signal

* **Method:**

  `POST`

* **Headers:**

  `X-Dont-Modify-Name: true` If specified, Nanny won't modify the `name` specified in the JSON payload. Useful when your signals come from programs with dynamic IP addresses.

* **Data Params**
  ```js
  {
    "name": "name of monitored program",
    "notifier": "stderr", # You can use only enabled notifiers, see config.
    "next_signal": "55s", # When to expect next call (or notify).
    "meta": {             # Meta can contain any string:string values,
      "extra": "data"     # they are passed to the notifiers and will eventually
    }                     # be passed to the user.
  }
  ```

* **Success Response:**

  * **Code:** 200
    **Content:** `{"status_code":200, "status":"OK"}`

* **Error Response:**
  * **Code:** 400 Bad Request
    **Content:** `{"status_code":400,"error":"unable to find notifier: "}`

  OR

  * **Code:** 500 Internal Server Error
    **Content:** `Message describing error, may be JSON or may be text.`

### Current signals
  Return current signals as JSON.

* **URL**

  /api/v1/signals

* **Method:**

  `GET`

* **Success Response:**

  * **Code:** 200
    **Content:**
    ```
    {
      "nanny_name": "Nanny",
      "signals": [
        {
          "name": "my awesome program",
          "notifier": "stderr",
          "next_signal":"2018-08-21T10:00:15+02:00",
          "meta": {
            "current-step": "loading"
          }
        },
        {
          "name": "my awesome program without meta",
          "notifier": "email",
          "next_signal":"2018-08-21T09:45:00+02:00"
        }
      ]
    }
    ```

## Monitoring nanny
You can use one Nanny to monitor another Nanny or create a monitored Nanny-pair.

Run 1st nanny, on port 8080 that will use nanny at port 9090 as its monitor:
```bash
NANNY_ADDR="localhost:8080" LOGXI=* ./nanny --nanny "http://localhost:9090/api/v1/signal" --nanny-notifier "stderr"
```

Run 2nd nanny, on port 9090 that will use 1st nanny on port 8080:
```bash
NANNY_ADDR="localhost:9090" LOGXI=* ./nanny --nanny "http://localhost:8080/api/v1/signal" --nanny-notifier "stderr"
```

You may get some warnings until both Nannies are listening, but they will recover. If you stop one of them, the other will notify you.

Be sure to change nanny SQLite DB location! They would share the same DB and it could cause strange behavior.
This can be done in the config file or by setting `NANNY_STORAGE_DSN` ENV variable.

## Logging
By default, nanny logs only errors. To enable more verbose logging, use `LOGXI=*` environment variable.

## Adding custom data (tags) to notifications
You can add extra meta-data to the API calls, which will be passed to all the notifiers. Metadata must conform to type `map[string]string`.

```bash
curl http://localhost:8080/api/v1/signal --data '{ "name": "my program", "notifier": "stderr", "next_signal": "5"s "meta":{"custom": "metadata"} }'
```

These metadata will be displayed in the messages for stderr and email, and in tags for sentry.

## Contributing
Contributions welcome! Just be sure you run tests and lints.

```bash
$ make
  Build
make build                            Build production binary.
  Dev
make run                              Run Nanny in dev mode, all logging and race detector ON.
make test                             Run tests.
make vet                              Run go vet.
make lint                             Run gometalinter (you have to install it).
```

## FAQ
> Why write such a tool?

Sometimes you expect some job to run, say cron. But when someone messes up your crontab, or the machine is offline, you might not be notified.

Also often programs just log errors and fail silently, with nanny they fail loudly.

> How do I secure my nanny?

To use HTTPS, or authentication you should use a reverse proxy like Apache or Nginx.

[![](https://codescene.io/projects/3429/status.svg) Get more details at **codescene.io**.](https://codescene.io/projects/3429/jobs/latest-successful/results)
