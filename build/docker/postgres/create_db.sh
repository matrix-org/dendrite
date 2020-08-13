#!/bin/bash

for db in account device mediaapi syncapi roomserver serverkey keyserver federationsender currentstate appservice e2ekey naffka; do
    createdb -U dendrite -O dendrite dendrite_$db
done
