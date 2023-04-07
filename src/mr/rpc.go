package mr

//
// RPC definitions.
//
// remember to capitalize all names.
//

import (
	"encoding/gob"
	"os"
	"strconv"
)

//
// example to show how to declare the arguments
// and reply for an RPC.
//

type ExampleArgs struct {
	X int
}

type ExampleReply struct {
	Y int
}

type TaskType int

const (
	MAP = iota
	REDUCE
	TERMINATE
)

type Reply = MapTaskReply

type TaskReply struct {
	Task interface{}
}

type TerminateTaskReply struct{}

type ReduceTaskReply struct {
	Partition int
	NMappers  int
}

type MapTaskReply struct {
	Filename        string
	NReduce, TaskId int
}

// Add your RPC definitions here.

func initRPCDecode() {
	gob.Register(MapTaskReply{})
	gob.Register(ReduceTaskReply{})
	gob.Register(TerminateTaskReply{})
}

// Cook up a unique-ish UNIX-domain socket name
// in /var/tmp, for the coordinator.
// Can't use the current directory since
// Athena AFS doesn't support UNIX-domain sockets.
func coordinatorSock() string {
	s := "/var/tmp/5840-mr-"
	s += strconv.Itoa(os.Getuid())
	return s
}
