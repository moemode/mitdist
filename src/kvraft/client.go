package kvraft

import (
	"crypto/rand"
	"log"
	"math/big"
	"sync"
	"sync/atomic"

	"6.5840/labrpc"
)

type Clerk struct {
	servers  []*labrpc.ClientEnd
	leaderId int32
	nServers int32
	id       int64
	reqN     int64
	reqNumMu sync.Mutex
	// You will have to modify this struct.
}

func nrand() int64 {
	max := big.NewInt(int64(1) << 62)
	bigx, _ := rand.Int(rand.Reader, max)
	x := bigx.Int64()
	return x
}

func MakeClerk(servers []*labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	ck.servers = servers
	ck.nServers = int32(len(servers))
	ck.id = nrand()
	ck.reqN = -1
	ck.tryNewLeader(0)
	log.Printf("Clerk %v", ck.id)
	// You'll have to add code here.
	return ck
}

// fetch the current value for a key.
// returns "" if the key does not exist.
// keeps trying forever in the face of all other errors.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.Get", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
func (ck *Clerk) Get(key string) string {
	// You will have to modify this function.
	rN := ck.reqNum()
	log.Printf("[CLERK %v] Get %v", ck.id, key)
	for {
		var reply GetReply
		lid := atomic.LoadInt32(&ck.leaderId)
		ok := ck.servers[lid].Call("KVServer.Get", &GetArgs{
			ClientId:      ck.id,
			RequestNumber: rN,
			Key:           key,
		}, &reply)
		if ok && (reply.Err == OK || reply.Err == ErrNoKey) {
			log.Printf("[CLERK %v] Get key=%v -> %v", ck.id, key, reply.Value)
			return reply.Value
		}
		ck.tryNewLeader(lid)
		/*
			if !ok || (reply.Err == ErrWrongLeader) {
				ck.tryNewLeader(lid)
			}*/
	}
}

func (ck *Clerk) tryNewLeader(oldLeader int32) {
	atomic.CompareAndSwapInt32(&ck.leaderId, oldLeader, int32(nrand()%int64(ck.nServers)))
}

func (ck *Clerk) reqNum() int64 {
	ck.reqNumMu.Lock()
	defer ck.reqNumMu.Unlock()
	ck.reqN++
	return ck.reqN
}

// shared by Put and Append.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.PutAppend", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
func (ck *Clerk) PutAppend(key string, value string, op string) {
	rN := ck.reqNum()
	for {
		var reply PutAppendReply
		lid := atomic.LoadInt32(&ck.leaderId)
		log.Printf("[CLERK %v] %v key:%v value:'%v'", ck.id, op, key, value)
		ok := ck.servers[lid].Call("KVServer.PutAppend", &PutAppendArgs{
			ClientId:      ck.id,
			RequestNumber: rN,
			Key:           key,
			Value:         value,
			Op:            OpType(op),
		}, &reply)
		if ok && reply.Err == "" {
			log.Printf("[CLERK %v] SUCCESS %v key:%v value:'%v'", ck.id, op, key, value)
			return
		}
		ck.tryNewLeader(lid)
	}
}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, "Put")
}
func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, "Append")
}
