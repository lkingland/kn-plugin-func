package main

import (
	f "f"

	"github.com/lkingland/func-runtimes/go/http"
)

func main() {
	http.Start(http.DefaultInstance{Handler: f.Handle})
}
