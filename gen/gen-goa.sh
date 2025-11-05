#!/usr/bin/env bash
set -eo pipefail

echo "start generating goagen code"
rm -rf ./api/gen
go env -w GOFLAGS=-buildvcs=false
go generate ./api/...
cp -rf ./api/gen/http/openapi3.json ./swagger/swagger.json