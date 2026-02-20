#!/usr/bin/env bash
set -e
cd "$(dirname "$0")"
go build -o kq . && exec ./kq "$@"
