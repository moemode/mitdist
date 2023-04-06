package mr

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"sync"
	"time"
)

type WorkerId int
type TaskId int

type Empty struct{}
type Coordinator struct {
	files                []string
	nReduce              int
	done                 bool
	mapUnfinished        []int
	mapFinished          map[int]Empty
	mapTaskLock          sync.Mutex
	reduceUnfinished     []int
	reduceUnfinishedLock sync.Mutex
}

func (c *Coordinator) GetMapTask(_ *struct{}, r *MapTaskReply) error {
	ok, taskId := c.unfinishedMapTask()
	if ok {
		r.Filename = c.files[0]
		r.TaskId = taskId
		r.NReduce = c.nReduce
	}
	go c.handleTimeout(taskId)
	return nil
}

func (c *Coordinator) handleTimeout(taskId int) {
	<-time.After(10 * time.Second)
	c.mapTaskLock.Lock()
	defer c.mapTaskLock.Unlock()
	_, finished := c.mapFinished[taskId]
	if !finished {
		c.mapUnfinished = append(c.mapUnfinished, taskId)
	}
}

func (c *Coordinator) TaskCompleted(TaskId int, _ *struct{}) error {
	c.mapTaskLock.Lock()
	defer c.mapTaskLock.Unlock()
	c.mapFinished[TaskId] = Empty{}
	if len(c.mapFinished) == len(c.files) {
		c.done = true
	}
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

func (c *Coordinator) unfinishedMapTask() (bool, int) {
	c.mapTaskLock.Lock()
	defer c.mapTaskLock.Unlock()
	if len(c.mapUnfinished) > 0 {
		lastIndex := len(c.mapUnfinished) - 1
		r := c.mapUnfinished[lastIndex]
		c.mapUnfinished = c.mapUnfinished[:lastIndex]
		return true, r
	}
	return false, 0
}

// create a Coordinator.
// main/mrcoordinator.go calls this function.
// each file corresponds to one "split",
// and is the input to one Map task.
// nReduce is the number of reduce tasks to use.
func MakeCoordinator(files []string, nReduce int) *Coordinator {
	c := Coordinator{
		files:                files,
		nReduce:              nReduce,
		done:                 false,
		mapUnfinished:        makeRange(0, len(files)),
		mapFinished:          map[int]Empty{},
		mapTaskLock:          sync.Mutex{},
		reduceUnfinished:     makeRange(0, nReduce),
		reduceUnfinishedLock: sync.Mutex{},
	}
	c.server()
	return &c
}

func makeRange(min, max int) []int {
	a := make([]int, max-min)
	for i := range a {
		a[i] = min + i
	}
	return a
}
