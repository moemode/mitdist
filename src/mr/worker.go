package mr

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io/ioutil"
	"log"
	"net/rpc"
	"os"
	"sort"
)

// Map functions return a slice of KeyValue.
type KeyValue struct {
	Key   string
	Value string
}

// for sorting by key.
type ByKey []KeyValue

// for sorting by key.
func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

// main/mrworker.go calls this function.
func Worker(mapf func(string, string) []KeyValue, reducef func(string, []string) string) {
	initRPCDecode()
	for {
		ok, r := CallGetTask()
		if !ok {
			log.Fatalf("CallGetTask failed, coordinator done or unreachable, TERMINATE worker")
		}
		task := r.Task
		switch task := task.(type) {
		case MapTaskReply:
			mapFile(mapf, task)
			CallTaskCompleted(task)
		case ReduceTaskReply:
			reduce(reducef, task.Partition, task.NMappers)
			CallTaskCompleted(task)
		case TerminateTaskReply:
			os.Exit(0)
		default:
			log.Fatalf("Worker received unknown task type")
		}
	}
}

func readPartition(partition, nMappers int) []KeyValue {
	kva := []KeyValue{}
	for i := 0; i < nMappers; i++ {
		filename := fmt.Sprintf("mr-%v-%v", i, partition)
		file, err := os.Open(filename)
		if err != nil {
			log.Fatalf("cannot open %v", filename)
		}
		dec := json.NewDecoder(file)
		for {
			var kv KeyValue
			if err := dec.Decode(&kv); err != nil {
				break
			}
			kva = append(kva, kv)
		}
	}
	return kva
}

func reduce(reducef func(string, []string) string, partition int, nMappers int) {
	intermediate := readPartition(partition, nMappers)
	sort.Sort(ByKey(intermediate))
	oname := fmt.Sprintf("mr-out-%v", partition)
	ofile, _ := os.Create(oname)
	// call Reduce on each distinct key in intermediate[],
	// and print the result to ofile
	i := 0
	for i < len(intermediate) {
		j := i + 1
		for j < len(intermediate) && intermediate[j].Key == intermediate[i].Key {
			j++
		}
		values := []string{}
		for k := i; k < j; k++ {
			values = append(values, intermediate[k].Value)
		}
		output := reducef(intermediate[i].Key, values)
		// this is the correct format for each line of Reduce output.
		fmt.Fprintf(ofile, "%v %v\n", intermediate[i].Key, output)
		i = j
	}
	ofile.Close()
}

func mapFile(mapf func(string, string) []KeyValue, mT MapTaskReply) {
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
	fmt.Printf("[WORKER] Completed %v\n", mT)
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

func CallGetTask() (bool, *TaskReply) {
	var args struct{}
	reply := TaskReply{}
	ok := call("Coordinator.GetTask", &args, &reply)
	if ok {
		fmt.Printf("[WORKER] got task: %+v\n", reply.Task)
		return true, &reply
	} else {
		fmt.Printf("[WORKER] CallGetTask failed")
		return false, nil
	}
}

func CallTaskCompleted(task interface{}) bool {
	var reply struct{}
	ok := call("Coordinator.TaskCompleted", &task, &reply)
	if ok {
		return true
	} else {
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
