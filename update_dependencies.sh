#!/bin/bash

# Update "stable" Go dependencies.
go get -u
go mod tidy
