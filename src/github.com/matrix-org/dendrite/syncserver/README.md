# Sync Server

This server is responsible for servicing `/sync` requests. It gets its data from the room server output log.

## Internals

When the server gets a `/sync` request, it needs to:
 - Work out *which* rooms to return to the client.
 - For each room, work out *which* events to return to the client.

The logic for working out which rooms is based on [Synapse](https://github.com/matrix-org/synapse/blob/v0.19.3/synapse/handlers/sync.py#L821):
  1) Get the CURRENT joined room list for this user.
  2) Get membership list changes for this user between the provided stream position and now.
  3) For each room which has membership list changes:
     - Check if the room is 'newly joined' (insufficient to just check for a join event because we allow dupe joins).
        If it is, then we need to send the full room state down (and 'limited' is always true).
     - Check if user is still CURRENTLY invited to the room. If so, add room to 'invited' block.
     - Check if the user is CURRENTLY left/banned. If so, add room to 'archived' block.
  4) Add joined rooms (joined room list)

For each room, the /sync response returns the most recent timeline events and the state of the room at the start of the timeline.
The logic for working out *which* events is not based entirely on Synapse code, as it is known broken with respect to working out
room state. In order to know which events to return, the server needs to calculate room state at various points in the history of
the room. For example, imagine a room with the following 15 events (letters are state events (updated via '), numbers are timeline events):

```
index     0  1  2  3  4  5  6  7  8   9  10  11  12  13    14     15   (1-based indexing as StreamPosition(0) represents no event)
timeline    [A, B, C, D, 1, 2, 3, D', 4, D'', 5, B', D''', D'''', 6]
```

The current state of this room is: `[A, B', C, D'''']`.

If this room was requested with `?since=9&limit=5` then 5 timeline events would be returned, the most recent ones:
```
    11 12  13    14     15
   [5, B', D''', D'''', 6]
```

The state of the room at the START of the timeline can be represented in 2 ways:
 - The 'full_state' from index 0 : `[A, B, C, D'']` (aka the state between 0-11 exclusive)
 - A partial state from index 9  : `[D'']`          (aka the state between 9-11 exclusive)

Servers advance state events (e.g from `D'` to `D''`) based on the state conflict resolution algorithm.
You might think that you could advance the current state by just updating the entry for the `(event type, state_key)` tuple
for each state event, but this state can diverge from the state calculated using the state conflict resolution algorithm.
For example, if there are two "simultaneous" updates to the same state key, that is two updates at the same depth in the
event graph, then the final result of the state conflict resolution algorithm might not match the order the events appear
in the timeline.

The correct advancement for state events is represented by the `AddsStateEventIDs` and `RemovesStateEventIDs` that
are in `OutputRoomEvents` from the room server.

This version of the sync server uses very simple indexing to calculate room state at various points.
This is inefficient when a very old `since` value is provided, or the `full_state` is requested, as the state delta becomes
very large. This is mitigated slightly with indexes, but better data structures could be used in the future.
