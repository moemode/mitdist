package kvraft

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

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

type Empty struct{}

type Op struct {
	// Your definitions here.
	// Field names must start with capital letters,
	// otherwise RPC will break.
	Type                    OpType
	Args                    []string
	ClientId, RequestNumber int64
}

type Result struct {
	requestNumber int64
	result        string
	err           Err
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
	return op.Type == op2.Type && testEq(op.Args, op2.Args) && op.ClientId == op2.ClientId && op.RequestNumber == op2.RequestNumber
}

type KVServer struct {
	mu      sync.Mutex
	me      int
	rf      *raft.Raft
	applyCh chan raft.ApplyMsg
	dead    int32 // set by Kill()

	maxraftstate int // snapshot if log grows this big

	// Your definitions here.
	handlerDoneCh         chan int
	waitingHandlerIndices map[int]Empty
	lastApplyMsg          raft.ApplyMsg
	lastApplyMsgChanged   *sync.Cond
	values                map[string]string
	lastRequest           map[int64]Result
}

func (kv *KVServer) Get(args *GetArgs, reply *GetReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	reply.Value, reply.Err = kv.Operation(Op{
		Type:          GetOp,
		Args:          []string{args.Key},
		ClientId:      args.ClientId,
		RequestNumber: args.RequestNumber,
	})
	if reply.Err != OK {
		log.Printf("[SERVER] GET reply: err: %v, value:%v", reply.Err, reply.Value)
	}
	log.Printf("[SERVER] GET reply: err: %v, value:%v", reply.Err, reply.Value)

}

func (kv *KVServer) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	k, v := args.Key, args.Value
	_, reply.Err = kv.Operation(Op{
		Type:          args.Op,
		Args:          []string{k, v},
		ClientId:      args.ClientId,
		RequestNumber: args.RequestNumber,
	})
	if reply.Err != OK {
		//log.Printf("[SERVER] PUT reply: %v", reply.Err)
		return
	}
}

func (kv *KVServer) Operation(op Op) (string, Err) {
	if kv.isDuplicate(op) {
		if r, mostRecent := kv.duplicateResult(op); mostRecent {
			return r.result, OK
		} else {
			return "", "Outdated"
		}
	}
	id, t, isLeader := kv.rf.Start(op)
	if !isLeader {
		return "", ErrWrongLeader
	}
	log.Printf("[SERVER] (ID=%v) Start %+v", id, op)
	_, alreadyWaiting := kv.waitingHandlerIndices[id]
	if alreadyWaiting {
		log.Fatalf("Some RPC is already waiting for %v", id)
	}
	kv.waitingHandlerIndices[id] = Empty{}
	defer func() {
		delete(kv.waitingHandlerIndices, id)
		if kv.lastApplyMsg.CommandIndex == id {
			kv.handlerDoneCh <- id
		}
	}()
	for t == kv.rf.Term() && kv.lastApplyMsg.CommandIndex < id {
		/*
			log.Printf("COmmand index %v, id: %v", kv.lastApplyMsg.CommandIndex, id)
			log.Printf("Back to sleep %+v", op)
			log.Printf("%v", kv.lastRequest)
		*/
		kv.lastApplyMsgChanged.Wait()
	}
	if t != kv.rf.Term() {
		return "", ErrWrongLeader
	}
	// handlers turn
	//log.Printf("Operation applied")
	m := kv.lastApplyMsg
	//err := Err("")
	appliedOp := m.Command.(Op)
	if !appliedOp.Equals(&op) {
		log.Printf("[SERVER] %+v does not match %+v", appliedOp, op)
		return "", ErrWrongLeader
	}
	//log.Printf("[SERVER] APPLIED (ID=%v) %+v was applied", id, op)
	r := kv.lastRequest[op.ClientId]
	return r.result, r.err
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

func (kv *KVServer) isDuplicate(op Op) bool {
	r, exists := kv.lastRequest[op.ClientId]
	return exists && r.requestNumber >= op.RequestNumber
}

func (kv *KVServer) duplicateResult(op Op) (Result, bool) {
	r, exists := kv.lastRequest[op.ClientId]
	if exists && r.requestNumber == op.RequestNumber {
		return r, true
	}
	return Result{}, false
}

func (kv *KVServer) apply() {
	for !kv.killed() {
		m := <-kv.applyCh
		if !m.CommandValid {
			log.Fatalf("Non Command Apply Msg Unimplemented")
		}
		kv.mu.Lock()
		kv.executeIfNew(m.Command.(Op))
		//log.Printf("Op ClientId: %v Request#: %v", op.ClientId, op.RequestNumber)
		if _, waiting := kv.waitingHandlerIndices[m.CommandIndex]; !waiting {
			kv.mu.Unlock()
			//log.Println("Continue")
			continue
		}
		kv.lastApplyMsg = m
		kv.lastApplyMsgChanged.Broadcast()
		log.Printf("Apply waiting for rpc exit,\n waitingHandlers: %+v\n lastApplyMsg: %+v", kv.waitingHandlerIndices, kv.lastApplyMsg)
		kv.mu.Unlock()
		for {
			idx := <-kv.handlerDoneCh
			if idx == m.CommandIndex {
				break
			}
		}
		log.Println("Apply detected rpc exit")
	}
}

func (kv *KVServer) executeIfNew(op Op) {
	if kv.isDuplicate(op) {
		return
	}
	r, e := kv.execute(op)
	kv.lastRequest[op.ClientId] = Result{
		requestNumber: op.RequestNumber,
		result:        r,
		err:           e,
	}
}

func (kv *KVServer) execute(op Op) (string, Err) {
	switch op.Type {
	case GetOp:
		v, exists := kv.values[op.Args[0]]
		if !exists {
			return "", ErrNoKey
		}
		return v, OK
	case PutOp:
		kv.values[op.Args[0]] = op.Args[1]
	case AppendOp:
		kv.values[op.Args[0]] += op.Args[1]
	}
	return "", OK

	//log.Printf("[EXECUTE] %v='%v'\n", op.Args[0], kv.values[op.Args[0]])
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
	kv.waitingHandlerIndices = make(map[int]Empty)
	kv.handlerDoneCh = make(chan int)
	kv.lastApplyMsgChanged = sync.NewCond(&kv.mu)
	kv.values = make(map[string]string)
	kv.lastRequest = make(map[int64]Result)
	go kv.apply()
	go func() {
		for {
			kv.lastApplyMsgChanged.Broadcast()
			time.Sleep(50 * time.Millisecond)
		}
	}()
	return kv
}
