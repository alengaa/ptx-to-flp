package main

// version is set at build time:
//
//	go build -ldflags "-X main.version=0.1.0-alpha.1"
var version = "dev"

func appTitle() string { return "PTX to FLP " + version }
