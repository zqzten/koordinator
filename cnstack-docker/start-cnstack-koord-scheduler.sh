#!/usr/bin/env bash

mkdir -p /log/
chmod 755 /log/

args="$@"
if [ $# -le 1 ]; then
    args="--logtostderr --v=4"
fi

set -a
: ${ALI_LOGROTATE_COMPRESS:=true} # compress logs
: ${ALI_LOGROTATE_MAX_BACKUPS:=8000} # save 8000 log files, 8000 * 28MB = 218.75GB
: ${ALI_LOGROTATE_MAX_AGE:=7} # save 7 days
set +a

kubectl create -f /etc/koordinator-crds/
kubectl apply -f /etc/koordinator-rbac/

pidof koord-scheduler || {
/koord-scheduler $args 2>&1 
}
