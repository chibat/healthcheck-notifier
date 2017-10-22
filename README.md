# Healthcheck Notifier

## Build

```
$ go get -u github.com/golang/dep/cmd/dep
$ dep ensure
$ go build
```

## Run

```
$ ./healthcheck-notifier
```

open http://localhost:18888/ in web browser

## Configuration

```
```

## Debug

```
$ go run healthcheck-notifier.go
```

open http://localhost:18888/ in web browser

### Live reloading

Get gin
```
go get github.com/codegangsta/gin
```

Run
```
gin run healthcheck-notifier.go
```

open http://localhost:3000/ in web browser
