#!/bin/sh

for db in account device mediaapi syncapi roomserver signingkeyserver keyserver federationsender appservice naffka; do
    createdb -U dendrite -O dendrite dendrite_$db
done
