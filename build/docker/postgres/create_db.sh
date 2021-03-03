#!/bin/sh

for db in userapi_accounts userapi_devices mediaapi syncapi roomserver signingkeyserver keyserver federationsender appservice naffka; do
    createdb -U dendrite -O dendrite dendrite_$db
done
