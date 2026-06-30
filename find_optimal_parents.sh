#!/bin/bash

CWD=`pwd`
SCRIPT_DIR=`cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd`
cd $SCRIPT_DIR/cmd/optimalparents

go run main.go
mv optimal_parents.csv ../../pki/data/

cd $CWD
