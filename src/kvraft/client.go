package kvraft

import (
	"crypto/rand"
	"log"
	"math/big"
	"sync/atomic"

	"6.5840/labrpc"
)

type Clerk struct {
	servers  []*labrpc.ClientEnd
	leaderId int32
	nServers int32
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
	ck.tryNewLeader(0)
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
	log.Printf("Get %v", key)
	for {
		var reply GetReply
		lid := atomic.LoadInt32(&ck.leaderId)
		ok := ck.servers[lid].Call("KVServer.Get", &GetArgs{
			Key: key,
		}, &reply)
		if ok && (reply.Err == OK || reply.Err == ErrNoKey) {
			return reply.Value
		}
		if !ok || (reply.Err == ErrWrongLeader) {
			ck.tryNewLeader(lid)
		}
	}
}

func (ck *Clerk) tryNewLeader(oldLeader int32) {
	atomic.CompareAndSwapInt32(&ck.leaderId, oldLeader, int32(nrand()%int64(ck.nServers)))
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
	log.Printf("%v %v %v", op, key, value)
	for {
		var reply PutAppendReply
		lid := atomic.LoadInt32(&ck.leaderId)
		ok := ck.servers[lid].Call("KVServer.PutAppend", &PutAppendArgs{key, value, op}, &reply)
		if ok {
			return
		}
		if !ok || (reply.Err == ErrWrongLeader) {
			ck.tryNewLeader(lid)
		}
	}
}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, "Put")
}
func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, "Append")
}
