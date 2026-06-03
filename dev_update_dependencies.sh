#!/bin/bash

# Update "dev" Go dependencies.
go get -modfile=dev_go.mod -u
go mod tidy -modfile=dev_go.mod
