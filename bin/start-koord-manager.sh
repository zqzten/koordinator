#!/usr/bin/env bash

args="$@"

# TODO add log rotate

kubectl apply -f /etc/koordinator-crds/
koord-manager $args
