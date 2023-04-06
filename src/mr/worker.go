package mr

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"log"
	"net/rpc"
	"os"
)

// Map functions return a slice of KeyValue.
type KeyValue struct {
	Key   string
	Value string
}

// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

// main/mrworker.go calls this function.
func Worker(mapf func(string, string) []KeyValue, reducef func(string, []string) string) {
	ok, maptask := CallGetMapTask()
	if ok {
		mapFile(mapf, maptask)
	}
}

func mapFile(mapf func(string, string) []KeyValue, mT *MapTaskReply) {
	filename := mT.Filename
	file, err := os.Open(filename)
	if err != nil {
		log.Fatalf("cannot open %v", filename)
	}
	content, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatalf("cannot read %v", filename)
	}
	file.Close()
	kva := mapf(filename, string(content))
	encodeKV(kva, mT.TaskId, mT.NReduce)
	CallTaskCompleted(mT.TaskId)
}

func encodeKV(kva []KeyValue, taskId int, nReduce int) {
	mapOutSplit := make([]*json.Encoder, nReduce)
	for i := 0; i < nReduce; i++ {
		file, err := os.Create(fmt.Sprintf("mr-%v-%v", taskId, i))
		if err != nil {
			log.Fatalf("%v", err)
		}
		mapOutSplit[i] = json.NewEncoder(file)
	}
	for _, kv := range kva {
		err := mapOutSplit[ihash(kv.Key)%nReduce].Encode(&kv)
		if err != nil {
			log.Fatalf("cannot encode %v: %v\n", kv, err)
		}
	}
}

func CallGetMapTask() (bool, *MapTaskReply) {
	var args struct{}
	reply := MapTaskReply{}
	ok := call("Coordinator.GetMapTask", &args, &reply)
	if ok {
		fmt.Printf("[WORKER] got map task: %v\n", reply.Filename)
		return true, &reply
	} else {
		fmt.Printf("[WORKER] GetMapTask failed")
		return false, nil
	}
}

func CallTaskCompleted(taskId int) bool {
	var reply struct{}
	ok := call("Coordinator.TaskCompleted", taskId, &reply)
	if ok {
		fmt.Printf("[WORKER] completed task")
		return true
	} else {
		fmt.Printf("[WORKER] GetMapTask failed")
		return false
	}
}

// example function to show how to make an RPC call to the coordinator.
//
// the RPC argument and reply types are defined in rpc.go.
func CallExample() {

	// declare an argument structure.
	args := ExampleArgs{}

	// fill in the argument(s).
	args.X = 99

	// declare a reply structure.
	reply := ExampleReply{}

	// send the RPC request, wait for the reply.
	// the "Coordinator.Example" tells the
	// receiving server that we'd like to call
	// the Example() method of struct Coordinator.
	ok := call("Coordinator.Example", &args, &reply)
	if ok {
		// reply.Y should be 100.
		fmt.Printf("reply.Y %v\n", reply.Y)
	} else {
		fmt.Printf("call failed!\n")
	}
}

// send an RPC request to the coordinator, wait for the response.
// usually returns true.
// returns false if something goes wrong.
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := coordinatorSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}
