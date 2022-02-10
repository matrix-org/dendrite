#!/bin/sh

<<<<<<< HEAD
for db in userapi_accounts userapi_devices pushserver mediaapi syncapi roomserver keyserver federationapi appservice naffka; do
=======
for db in userapi_accounts userapi_devices mediaapi syncapi roomserver keyserver federationapi appservice mscs; do
>>>>>>> main
    createdb -U dendrite -O dendrite dendrite_$db
done
