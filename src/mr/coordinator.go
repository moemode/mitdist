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
	nMap                 int
	nReduce              int
	done                 bool
	mapUnfinished        []int
	mapFinished          map[int]Empty
	mapTaskLock          sync.Mutex
	reduceUnfinished     []int
	reduceFinished       map[int]Empty
	reduceUnfinishedLock sync.Mutex
}

func (c *Coordinator) GetTask(_ *struct{}, r *TaskReply) error {
	task := c.findTask()
	r.Task = task
	switch task := task.(type) {
	case MapTaskReply:
		go c.resetMap(task.TaskId)
	case ReduceTaskReply:
		go c.resetReduce(task.Partition)
	default:
	}
	return nil
}

func (c *Coordinator) TaskCompleted(task interface{}, _ *struct{}) error {
	switch task := task.(type) {
	case MapTaskReply:
		c.completeMap(task.TaskId)
	case ReduceTaskReply:
		c.completeReduce(task.Partition)
	}
	return nil
}

func (c *Coordinator) resetReduce(partition int) {
	c.resetTask(partition, &c.reduceUnfinishedLock, &c.reduceUnfinished, &c.reduceFinished)
}

func (c *Coordinator) resetMap(taskId int) {
	c.resetTask(taskId, &c.mapTaskLock, &c.mapUnfinished, &c.mapFinished)
}

func (c *Coordinator) resetTask(taskId int, l *sync.Mutex, tasks *[]int, finished *map[int]Empty) {
	<-time.After(10 * time.Second)
	l.Lock()
	defer l.Unlock()
	_, fin := (*finished)[taskId]
	if !fin {
		*tasks = append(*tasks, taskId)
	}
}

func (c *Coordinator) completeMap(taskId int) {
	c.complete(taskId, &c.mapTaskLock, &c.mapFinished)
}

func (c *Coordinator) completeReduce(partition int) {
	c.complete(partition, &c.reduceUnfinishedLock, &c.reduceFinished)
}

func (c *Coordinator) complete(taskId int, l *sync.Mutex, finished *map[int]Empty) {
	l.Lock()
	defer l.Unlock()
	(*finished)[taskId] = Empty{}
	fmt.Printf("[COORDINTAOR] %v %v %v %v\n", len(c.mapFinished), len(c.files), len(c.reduceFinished), c.nReduce)
	c.done = len(c.mapFinished) == len(c.files) && len(c.reduceFinished) == c.nReduce
}

// start a thread that listens for RPCs from worker.go
func (c *Coordinator) server() {
	initRPCDecode()
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

func (c *Coordinator) findTask() interface{} {
	if len(c.mapFinished) < c.nMap {
		ok, taskId := c.unfinishedMapTask()
		if ok {
			return MapTaskReply{Filename: c.files[taskId], TaskId: taskId, NReduce: c.nReduce}
		}
	} else if len(c.reduceFinished) < c.nReduce {
		ok, partition := c.unfinishedReduceTask()
		if ok {
			return ReduceTaskReply{Partition: partition, NMappers: len(c.files)}
		}
	}
	return TerminateTaskReply{}
}

func (c *Coordinator) unfinishedTask(l *sync.Mutex, tasks *[]int) (bool, int) {
	l.Lock()
	defer l.Unlock()
	if len(*tasks) > 0 {
		lastIndex := len(*tasks) - 1
		r := (*tasks)[lastIndex]
		*tasks = (*tasks)[:lastIndex]
		return true, r
	}
	return false, 0
}

func (c *Coordinator) unfinishedMapTask() (bool, int) {
	return c.unfinishedTask(&c.mapTaskLock, &c.mapUnfinished)
}

func (c *Coordinator) unfinishedReduceTask() (bool, int) {
	return c.unfinishedTask(&c.reduceUnfinishedLock, &c.reduceUnfinished)
}

// create a Coordinator.
// main/mrcoordinator.go calls this function.
// each file corresponds to one "split",
// and is the input to one Map task.
// nReduce is the number of reduce tasks to use.
func MakeCoordinator(files []string, nReduce int) *Coordinator {
	c := Coordinator{
		files:                files,
		nMap:                 len(files),
		nReduce:              nReduce,
		done:                 false,
		mapUnfinished:        makeRange(0, len(files)),
		mapFinished:          map[int]Empty{},
		mapTaskLock:          sync.Mutex{},
		reduceUnfinished:     makeRange(0, nReduce),
		reduceFinished:       map[int]Empty{},
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
