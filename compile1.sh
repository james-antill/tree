#! /bin/sh -e

race=
# race=-race

cd cmd/tree

if ! go build $race; then
    exit 1
fi

tree=tree

if [ "x$(go env GOOS)" = "xwindows" ]; then
tree=tree.exe
fi

mv $tree tree.bin.$(go env GOOS)-$(go env GOARCH)

