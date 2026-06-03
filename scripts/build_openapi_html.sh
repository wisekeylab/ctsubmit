#!/bin/bash
cd "$(dirname "$(readlink -f "$0")")"
npm list @redocly/cli || npm i @redocly/cli@latest
npx @redocly/cli@latest build-docs ../docs/openapi.yaml -o ../docs/openapi.html
