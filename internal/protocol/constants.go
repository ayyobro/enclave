package protocol

const (
	ProtocolVersion = 1

	// Message types (client -> server)
	TypeRegister = "register"
	TypeAuth     = "auth"
	TypeAuthResp = "auth_resp"
	TypeMessage  = "message"
	TypeTyping   = "typing"
	TypeAck      = "ack"

	// Message types (server -> client)
	TypeChallenge = "challenge"
	TypeAuthOK    = "auth_ok"
	TypeAuthFail  = "auth_fail"
	TypePresence  = "presence"
	TypeError     = "error"

	// Limits
	MaxMessageSize  = 64 * 1024 // 64 KB
	MaxDisplayName  = 32
	NonceSize       = 24
	KeySize         = 32
	InviteTokenSize = 32
)
