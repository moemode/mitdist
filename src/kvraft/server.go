package kvraft

import (
	"log"
	"sync"
	"sync/atomic"

	"6.5840/labgob"
	"6.5840/labrpc"
	"6.5840/raft"
)

const Debug = false

func DPrintf(format string, a ...interface{}) (n int, err error) {
	if Debug {
		log.Printf(format, a...)
	}
	return
}

type Op struct {
	// Your definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	Type string
	Args []string
}

func testEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (op *Op) Equals(op2 *Op) bool {
	return op.Type == op2.Type && testEq(op.Args, op2.Args)
}

type KVServer struct {
	mu      sync.Mutex
	me      int
	rf      *raft.Raft
	applyCh chan raft.ApplyMsg
	dead    int32 // set by Kill()

	maxraftstate int // snapshot if log grows this big

	// Your definitions here.
	handlerDoneCh       chan bool
	waitingHandlerCh    map[int]chan raft.ApplyMsg
	lastApplyMsg        raft.ApplyMsg
	lastApplyMsgChanged *sync.Cond
	values              map[string]string
}

func (kv *KVServer) Get(args *GetArgs, reply *GetReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	reply.Err = kv.Operation(Op{
		Type: "Get",
		Args: []string{args.Key},
	})
	if reply.Err != "" {
		return
	}
	reply.Err = ErrNoKey
	v, ok := kv.values[args.Key]
	if ok {
		reply.Value = v
		reply.Err = OK
	}
}

func (kv *KVServer) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	reply.Err = kv.Operation(Op{
		Type: args.Op,
		Args: []string{args.Key, args.Value},
	})
}

func (kv *KVServer) Operation(submittedOp Op) Err {
	id, _, isLeader := kv.rf.Start(submittedOp)
	if !isLeader {
		return ErrWrongLeader
	}
	for kv.lastApplyMsg.CommandIndex != id {
		kv.lastApplyMsgChanged.Wait()
	}
	// handlers turn
	m := kv.lastApplyMsg
	appliedOp := m.Command.(Op)
	if appliedOp.Equals(&submittedOp) {
		return ErrWrongLeader
	}
	kv.handlerDoneCh <- true
	delete(kv.waitingHandlerCh, id)
	return ""
}

// the tester calls Kill() when a KVServer instance won't
// be needed again. for your convenience, we supply
// code to set rf.dead (without needing a lock),
// and a killed() method to test rf.dead in
// long-running loops. you can also add your own
// code to Kill(). you're not required to do anything
// about this, but it may be convenient (for example)
// to suppress debug output from a Kill()ed instance.
func (kv *KVServer) Kill() {
	atomic.StoreInt32(&kv.dead, 1)
	kv.rf.Kill()
	// Your code here, if desired.
}

func (kv *KVServer) killed() bool {
	z := atomic.LoadInt32(&kv.dead)
	return z == 1
}

func (kv *KVServer) apply() {
	for !kv.killed() {
		m := <-kv.applyCh
		if !m.CommandValid {
			log.Fatalf("Non Command Apply Msg Unimplemented")
		}
		kv.mu.Lock()
		//apply to kv map
		ch, waiting := kv.waitingHandlerCh[m.CommandIndex]
		kv.mu.Unlock()
		if waiting {
			ch <- m
			<-kv.handlerDoneCh
		}
	}
}

// servers[] contains the ports of the set of
// servers that will cooperate via Raft to
// form the fault-tolerant key/value service.
// me is the index of the current server in servers[].
// the k/v server should store snapshots through the underlying Raft
// implementation, which should call persister.SaveStateAndSnapshot() to
// atomically save the Raft state along with the snapshot.
// the k/v server should snapshot when Raft's saved state exceeds maxraftstate bytes,
// in order to allow Raft to garbage-collect its log. if maxraftstate is -1,
// you don't need to snapshot.
// StartKVServer() must return quickly, so it should start goroutines
// for any long-running work.
func StartKVServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister, maxraftstate int) *KVServer {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	labgob.Register(Op{})

	kv := new(KVServer)
	kv.me = me
	kv.maxraftstate = maxraftstate

	// You may need initialization code here.

	kv.applyCh = make(chan raft.ApplyMsg)
	kv.rf = raft.Make(servers, me, persister, kv.applyCh)

	// You may need initialization code here.
	kv.waitingHandlerCh = make(map[int]chan raft.ApplyMsg)
	kv.handlerDoneCh = make(chan bool)
	kv.lastApplyMsgChanged = sync.NewCond(&kv.mu)
	kv.values = make(map[string]string)
	return kv
}
