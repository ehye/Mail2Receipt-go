package main

import (
	"context"
	"os"

	"mail2receipt/internal/app"
)

func main() {
	os.Exit(app.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
