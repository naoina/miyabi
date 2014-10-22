# Miyabi [![Build Status](https://travis-ci.org/naoina/miyabi.png?branch=master)](https://travis-ci.org/naoina/miyabi)

Graceful shutdown and restart for Go's `net/http` handlers.

Miyabi is pronounced **me-ya-be**. It means Graceful in Japanese.

## Usage

It's very simple. Use `miyabi.ListenAndServe` instead of `http.ListenAndServe`.
You don't have to change other code because `miyabi.ListenAndServe` is compatible with `http.ListenAndServe`.

```go
package main

import (
    "io"
    "log"
    "net/http"
)

// hello world, the web server
func HelloServer(w http.ResponseWriter, req *http.Request) {
    io.WriteString(w, "hello, world!\n")
}

func main() {
    http.HandleFunc("/hello", HelloServer)
    log.Fatal(miyabi.ListenAndServe(":8080", nil))
}
```

See [Godoc](http://godoc.org/github.com/naoina/miyabi) for more information.

**NOTE**: Miyabi is using features of Go 1.3, so doesn't work in Go 1.2.x and older versions. Also when using on Windows, it works but graceful shutdown/restart are disabled explicitly.

## Graceful shutdown or restart

By default, send `SIGTERM` or `SIGINT` (Ctrl + c) signal to a process that is using Miyabi in order to graceful shutdown and send `SIGHUP` signal in order to graceful restart.
If you want to change the these signal, please set another signal to `miyabi.ShutdownSignal` and/or `miyabi.RestartSignal`.

In fact, `miyabi.ListenAndServe` and `miyabi.ListenAndServeTLS` will fork a process that is using Miyabi in order to achieve the graceful restart.
This means that you should write code as no side effects until the call of `miyabi.ListenAndServe` or `miyabi.ListenAndServeTLS`.

## License

Miyabi is licensed under the MIT.
