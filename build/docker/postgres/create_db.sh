#!/bin/sh

for db in userapi_accounts mediaapi syncapi roomserver keyserver federationapi appservice mscs; do
    createdb -U dendrite -O dendrite dendrite_$db
done
