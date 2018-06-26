# Nanny
[![Build Status](https://travis-ci.org/lunemec/nanny.svg?branch=master)](https://travis-ci.org/lunemec/nanny) [![Go Report Card](https://goreportcard.com/badge/github.com/lunemec/nanny)](https://goreportcard.com/report/github.com/lunemec/nanny)

Nanny is a monitoring tool that monitors the **absence of action**.

Nanny runs API server, which expects to be called every N seconds, and if no such call is made, Nanny notifies you.

Nanny can notify you via these channels (for now):
* print text to stderr
* email
* sentry

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
curl http://localhost:8080/api/v1/signal --data '{ "name": "my awesome program", "notifier": "stderr", "next_signal": 5 }'
```
With this call, you tell nanny that if program named `my awesome program` does not call again within `next_signal` seconds (5s), it should notify you using `stderr` notifier.

After 5s pass, nanny prints to *stderr*:
```bash
2018-06-26T14:24:29+02:00: Nanny: I did not hear from "my awesome program" in 5s! (Meta: map[])
```

## Installation
Easiest way is to download .tar.gz from **releases** section, edit `nanny.toml` and run it.

Or you can clone this repository and compile it yourself:
```bash
git clone https://github.com/lunemec/nanny.git
cd nanny
make build
```

## Configuration
See nanny.toml for configuration example. The fields are self-explanatory (I think). Please create issue if anything does not make sense!

All enabled notifiers can be used via API, so enable only those you wish to allow.

Program names used in API call's must be unique, they are used as key to load running
timers.
```bash
curl http://localhost:8080/api/v1/signal --data '{ "name": "<- this must be unique", "notifier": "stderr", "next_signal": 5 }'
```

## Logging
By default, nanny logs only errors. To enable more verbose logging, use `LOGXI=*` environment variable.

## Adding custom data (tags) to notifications
You can add extra meta-data to the API calls, which will be passed to all the notifiers. Metadata must conform to type `map[string]string`.

```bash
curl http://localhost:8080/api/v1/signal --data '{ "name": "<- this must be unique", "notifier": "stderr", "next_signal": 5 "meta":{"custom": "metadata"} }'
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
> Why write such tool?

Sometimes you expect some job to run, say cron. But when someone messes up your crontab, or the machine is offline, you might not be notified.

Also often programs just log errors and fail silently, with nanny they fail loudly.

> How do I secure my nanny?

To use HTTPS, or authentication you should use reverse proxy like Apache or Nginx.