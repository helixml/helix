package main

// Version is set at build time via -ldflags:
//
//	go build -ldflags "-X main.Version=1.2.3" .
var Version = "dev"
