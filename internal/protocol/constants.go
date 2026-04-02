package protocol

const (
	ProtocolVersion = 1

	// Message types (client -> server)
	TypeRegister     = "register"
	TypeAuth         = "auth"
	TypeAuthResp     = "auth_resp"
	TypeMessage      = "message"
	TypeGroupMessage = "group_message"
	TypeTyping       = "typing"
	TypeAck          = "ack"
	TypeReadReceipt  = "read_receipt"
	TypeReaction     = "reaction"
	TypeFileMeta     = "file_meta"
	TypeFileChunk    = "file_chunk"
	TypeGroupCreate  = "group_create"
	TypeGroupInvite  = "group_invite"
	TypeGroupLeave   = "group_leave"
	TypeEphemeral    = "ephemeral"

	// Message types (server -> client)
	TypeChallenge    = "challenge"
	TypeAuthOK       = "auth_ok"
	TypeAuthFail     = "auth_fail"
	TypePresence     = "presence"
	TypeError        = "error"
	TypeGroupCreated = "group_created"
	TypeGroupInfo    = "group_info"

	// Limits
	MaxMessageSize  = 64 * 1024 // 64 KB
	MaxFileSize     = 10 * 1024 * 1024 // 10 MB
	FileChunkSize   = 32 * 1024 // 32 KB
	MaxDisplayName  = 32
	MaxGroupName    = 64
	MaxGroupMembers = 50
	NonceSize       = 24
	KeySize         = 32
	InviteTokenSize = 32
)
