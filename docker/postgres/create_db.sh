#!/bin/bash

<<<<<<< HEAD
for db in account device mediaapi syncapi roomserver serverkey federationsender publicroomsapi appservice naffka; do
    createdb -U dendrite -O dendrite dendrite_$db
=======
for db in account device mediaapi syncapi roomserver serverkey federationsender publicroomsapi naffka encryptapi; do
    createdb -O dendrite dendrite_$db
>>>>>>> 8b4b3c6fc46900e9bfe5e234eda309200662b34a
done
