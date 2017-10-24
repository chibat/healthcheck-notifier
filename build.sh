#!/bin/bash

rm -rf build
mkdir build
GOOS=windows GOARCH=amd64 go build -o build/healthcheck-notifier.exe
GOOS=linux GOARCH=amd64 go build -o build/healthcheck-notifier-linux-amd64
GOOS=darwin GOARCH=amd64 go build -o build/healthcheck-notifier-darwin-amd64

