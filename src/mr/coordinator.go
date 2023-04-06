package mr

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
)

type WorkerId int
type TaskId int

/*
fetchTask()
*/

type Coordinator struct {
	// Your definitions here.
	/*
		assignment                      map[WorkerId][]TaskId
		nMap, nReduce                   int
		nMapCompleted, nReduceCompleted int
		nWorkers, nextWorkerId          int
	*/
	files   []string
	nReduce int
	done    bool
}

// Your code here -- RPC handlers for the worker to call.

// an example RPC handler.
//
// the RPC argument and reply types are defined in rpc.go.
func (c *Coordinator) Example(args *ExampleArgs, reply *ExampleReply) error {
	reply.Y = args.X + 1
	return nil
}

func (c *Coordinator) GetMapTask(_ *struct{}, r *MapTaskReply) error {
	if len(c.files) > 0 {
		r.Filename = c.files[0]
		r.TaskId = 0
		r.NReduce = c.nReduce
	}
	return nil
}

func (c *Coordinator) TaskCompleted(TaskId int, _ *struct{}) error {
	c.done = true
	return nil
}

// start a thread that listens for RPCs from worker.go
func (c *Coordinator) server() {
	rpc.Register(c)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := coordinatorSock()
	os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

// main/mrcoordinator.go calls Done() periodically to find out
// if the entire job has finished.
func (c *Coordinator) Done() bool {
	if c.done {
		fmt.Println("Coordinator is done.")
	}
	return c.done
}

// create a Coordinator.
// main/mrcoordinator.go calls this function.
// each file corresponds to one "split",
// and is the input to one Map task.
// nReduce is the number of reduce tasks to use.
func MakeCoordinator(files []string, nReduce int) *Coordinator {
	c := Coordinator{files, nReduce, false}
	// Your code here.
	c.server()
	return &c
}
