package kvraft

const (
	OK             = "OK"
	ErrNoKey       = "ErrNoKey"
	ErrWrongLeader = "ErrWrongLeader"
	ErrOldRequest  = "ErrOldRequest"
)

type Err string

type OpType string

const (
	GetOp    OpType = "Get"
	PutOp    OpType = "Put"
	AppendOp OpType = "Append"
)

type operable interface {
	toOperation() Op
}

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

func (args *PutAppendArgs) toOperation() Op {
	return Op{
		Type:          args.Op,
		Args:          []string{args.Key, args.Value},
		ClientId:      args.ClientId,
		RequestNumber: args.RequestNumber,
	}
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

func (args *GetArgs) toOperation() Op {
	return Op{
		Type:          GetOp,
		Args:          []string{args.Key},
		ClientId:      args.ClientId,
		RequestNumber: args.RequestNumber,
	}
}

type GetReply struct {
	Err   Err
	Value string
}
