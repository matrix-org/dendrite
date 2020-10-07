#!/bin/bash

for db in account device mediaapi syncapi roomserver signingkeyserver keyserver federationsender appservice e2ekey naffka; do
    createdb -U dendrite -O dendrite dendrite_$db
done
