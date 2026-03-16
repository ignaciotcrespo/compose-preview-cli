#!/bin/bash
set -e

if [ "$1" = "--cover" ]; then
    go test ./... -v -coverprofile=coverage.out
    go tool cover -func=coverage.out
    if [ "$2" = "--html" ]; then
        go tool cover -html=coverage.out
    else
        echo ""
        echo "Open HTML report: ./test.sh --cover --html"
    fi
else
    go test ./... -v
fi
