package authorization

type Permission int64

const (
	// since iota starts with 0, the first value
	// defined here will be the default
	PermissionUndefined Permission = iota
	PermissionRead
	PermissionWrite
	PermissionPing
	PermissionInvite
	PermissionRedact
	PermissionBan
	PermissionModifyChannelProfile
	PermissionModifyChannelPermissions
	PermissionPinMessages
	PermissionAddRemoveChannels
	PermissionModifySpacePermissions
	PermissionModifyChannelDefaults
	PermissionModifySpaceProfile
	PermissionOwner
)

func (p Permission) String() string {
	switch p {
	case PermissionUndefined:
		return "Undefined"
	case PermissionRead:
		return "Read"
	case PermissionWrite:
		return "Write"
	case PermissionPing:
		return "Ping"
	case PermissionInvite:
		return "Invite"
	case PermissionRedact:
		return "Redact"
	case PermissionBan:
		return "Ban"
	case PermissionModifyChannelProfile:
		return "ModifyChannelProfile"
	case PermissionModifyChannelPermissions:
		return "ModifyChannelPermissions"
	case PermissionPinMessages:
		return "PinMessages"
	case PermissionAddRemoveChannels:
		return "AddRemoveChannels"
	case PermissionModifySpacePermissions:
		return "ModifySpacePermissions"
	case PermissionModifyChannelDefaults:
		return "ModifyChannelDefaults"
	case PermissionModifySpaceProfile:
		return "ModifySpaceProfile"
	case PermissionOwner:
		return "Owner"
	}
	return "Unknown"
}
