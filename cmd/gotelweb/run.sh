#!/bin/sh

go build

# check to make sure the code built properly
[ $? -ne 0 ] && exit 1

# throw the password in an env var
# export GOTEL_DB_PASSWORD=foo
./gotelweb -GOTEL_DB_HOST=127.0.0.1 -GOTEL_DB_USER=root -GOTEL_DB_PASSWORD=  -GOTEL_SYSLOG=false
