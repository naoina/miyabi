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

## License

Miyabi licensed under the MIT.
