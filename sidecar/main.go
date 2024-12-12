package main

import (
	"smesh/pkg/manager"

	"github.com/gookit/slog"
)

func main() {

	slog.Info("starting the SMESH 🐝")
	c, err := manager.Setup()
	if err != nil {
		slog.Fatal(err)
	}
	err = manager.LoadEPF(c)
	if err != nil {
		slog.Fatal(err)
	}
	err = manager.Start(c)
	if err != nil {
		slog.Fatal(err)
	}
}
