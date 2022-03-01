#!/bin/sh

for db in userapi_accounts userapi_devices mediaapi syncapi roomserver keyserver federationapi appservice mscs; do
    createdb -U dendrite -O dendrite dendrite_$db
done
