-- name: InsertMultiRoomData :one
INSERT INTO syncapi_multiroom_data (
    user_id,
    type,
    data
) VALUES (
    $1,
    $2,
    $3
) ON CONFLICT (user_id, type) DO UPDATE SET id = nextval('syncapi_multiroom_id'), data = $3, ts = current_timestamp
RETURNING id;


-- name: InsertMultiRoomVisibility :exec
INSERT INTO syncapi_multiroom_visibility (
    user_id,
    type,
    room_id,
    expire_ts
) VALUES (
    $1,
    $2,
    $3,
    $4
) ON CONFLICT (user_id, type, room_id) DO UPDATE SET expire_ts = $4;

-- name: SelectMultiRoomVisibilityRooms :many
SELECT room_id FROM syncapi_multiroom_visibility
WHERE user_id = $1 
AND expire_ts > $2;


-- name: SelectMaxId :one
SELECT MAX(id) FROM syncapi_multiroom_data;

-- name: DeleteMultiRoomVisibility :exec
DELETE FROM syncapi_multiroom_visibility
WHERE user_id = $1
AND type = $2
AND room_id = $3;

-- name: DeleteMultiRoomVisibilityByExpireTS :execrows
DELETE FROM syncapi_multiroom_visibility
WHERE expire_ts <= $1;