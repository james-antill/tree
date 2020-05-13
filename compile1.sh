#! /bin/sh -e

cd cmd/tree

if ! go build; then
    exit 1
fi

mv tree tree.bin.${GOOS}-${GOARCH}

