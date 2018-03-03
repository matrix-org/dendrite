#!/bin/bash

for db in account device mediaapi syncapi roomserver serverkey federationsender publicroomsapi naffka; do
    createdb -O dendrite dendrite_$db
done
