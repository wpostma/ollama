#!/bin/sh

rm -f ollama-app ollama

# 1. Build React SPA
echo build react
cd app/ui/app
npm install
npm run build 
cd ../../..

echo ' '
echo ' '
echo ' '
echo ' ------------------------------------------------------------------- '
echo ' '
# 2. Build Go app (embeds SPA dist/ into binary)
echo build go ollama-app
/usr/local/go/bin/go version
CGO_ENABLED=1  /usr/local/go/bin/go build -trimpath -ldflags "-s -w" -o ollama-app ./app/cmd/app
echo build go ollama console
/usr/local/go/bin/go build -trimpath -ldflags "-s -w" -o ollama .
echo completed #?


