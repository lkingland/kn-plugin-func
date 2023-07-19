package main

import (
	"fmt"
	"os"

	ce "github.com/knative-sandbox/func-go/cloudevents"

	f "f"
)

func main() {
	if err := ce.Start(ce.DefaultHandler{Handler: f.Handle}); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
