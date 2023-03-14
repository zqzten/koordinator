#!/usr/bin/env bash
 
set -euo pipefail
 
DEV="${VARIABLE:-dsa}"
DEV_VM="${VARIABLE:-dsa_vm}"
DIR="/usr/bin"
 
function cmd() {
 
    echo "$@"
 
    "${@}"
}
 
for dev in $(accel-config list | jq '.[].dev' | grep "$DEV" | sed 's/\"//g'); do
    cmd accel-config disable-device "$dev"
 
done
 
if [ -z "$(lscpu | grep "Hypervisor vendor")" ]
then
  echo "BM"
  if [ -z "$(cat /proc/cmdline | grep "intel_iommu=on,sm_on")" ]
  then
      sed -i "s/shared/dedicated/g" $DIR/conf/$DEV.conf
      echo "wq mode: dedicated"
  else
      sed -i "s/dedicated/shared/g" $DIR/conf/$DEV.conf
      echo "wq mode: shared"
  fi
 
  for dev in $(accel-config list --idle | jq '.[].dev' | grep "$DEV" | sed 's/\"//g'); do
 
      i=`echo $dev | sed 's/^.\{3\}//'`
      echo "i=$i"
 
      config="$DEV.conf"
 
      [ -f "$DIR/conf/$DEV.conf" ] && config="$DIR/conf/$DEV.conf"
 
      sed "s/X/${i}/g" < "$config" > $DIR/conf/$dev.conf
      cmd accel-config load-config -f -e -c "$DIR/conf/$dev.conf"
  done
else
  echo "VM"
  for dev in $(accel-config list --idle | jq '.[].dev' | grep "$DEV" | sed 's/\"//g'); do
 
    i=`echo $dev | sed 's/^.\{3\}//'`
    echo "i=$i"
 
    config="$DEV_VM.conf"
 
    [ -f "$DIR/conf/$DEV_VM.conf" ] && config="$DIR/conf/$DEV_VM.conf"
 
    sed "s/X/${i}/g" < "$config" > $DIR/conf/$dev.conf
    cmd accel-config load-config -f -e -c "$DIR/conf/$dev.conf"
  done
fi