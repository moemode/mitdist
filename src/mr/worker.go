package mr

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/rpc"
	"os"
	"path/filepath"
	"sort"
)

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
		case ReduceTaskReply:
			reduce(reducef, task.Partition, task.NMappers)
		case TerminateTaskReply:
			fmt.Println("[WORKER] Exit on TerminateTaskReply")
			os.Exit(0)
		default:
			log.Fatalf("Worker received unknown task type")
		}
		CallTaskCompleted(task)
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

func tmpFile() (string, *os.File, error) {
	ofile, err := ioutil.TempFile(".", "rdtmp")
	if err != nil {
		return "", nil, err
	}
	tmppath, err := filepath.Abs(ofile.Name())
	return tmppath, ofile, err
}

func reduce(reducef func(string, []string) string, partition int, nMappers int) {
	intermediate := readPartition(partition, nMappers)
	sort.Sort(ByKey(intermediate))
	tmppath, ofile, err := tmpFile()
	if err != nil {
		log.Fatalf("Could not create temporary file %v", err)
	}
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
	oname := fmt.Sprintf("mr-out-%v", partition)
	err = os.Rename(tmppath, filepath.Join(filepath.Dir(tmppath), oname))
	if err != nil {
		log.Fatal(err)
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
	ok := call("Coordinator.FinishTask", &task, &reply)
	if ok {
		return true
	} else {
		return false
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
