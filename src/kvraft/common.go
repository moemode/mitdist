package kvraft

const (
	OK             = "OK"
	ErrNoKey       = "ErrNoKey"
	ErrWrongLeader = "ErrWrongLeader"
)

type Err string

type OpType string

const (
	GetOp    OpType = "Get"
	PutOp    OpType = "Put"
	AppendOp OpType = "Append"
)

// Put or Append
type PutAppendArgs struct {
	ClientId      int64
	RequestNumber int64
	Key           string
	Value         string
	Op            OpType // "Put" or "Append"
	// You'll have to add definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
}

type PutAppendReply struct {
	Err Err
}

type GetArgs struct {
	ClientId      int64
	RequestNumber int64
	Key           string
	// You'll have to add definitions here.
}

type GetReply struct {
	Err   Err
	Value string
}
