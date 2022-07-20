---
title: Supported admin APIs
parent: Administration
permalink: /administration/adminapi
---

# Supported admin APIs

Dendrite supports, at present, a very small number of endpoints that allow
admin users to perform administrative functions. Please note that there is no
API stability guarantee on these endpoints at present — they may change shape
without warning.

More endpoints will be added in the future.

## `/_dendrite/admin/evacuateRoom/{roomID}`

This endpoint will instruct Dendrite to part all local users from the given `roomID`
in the URL. It may take some time to complete. A JSON body will be returned containing
the user IDs of all affected users.

## `/_dendrite/admin/evacuateUser/{userID}`

This endpoint will instruct Dendrite to part the given local `userID` in the URL from
all rooms which they are currently joined. A JSON body will be returned containing
the room IDs of all affected rooms.

## `/_synapse/admin/v1/register`

Shared secret registration — please see the [user creation page](createusers) for
guidance on configuring and using this endpoint.
