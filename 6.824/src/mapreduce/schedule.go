package mapreduce

import (
	"fmt"
	"sync"
)

//
// schedule() starts and waits for all tasks in the given phase (mapPhase
// or reducePhase). the mapFiles argument holds the names of the files that
// are the inputs to the map phase, one per map task. nReduce is the
// number of reduce tasks. the registerChan argument yields a stream
// of registered workers; each item is the worker's RPC address,
// suitable for passing to call(). registerChan will yield all
// existing registered workers (if any) and new ones as they register.
//
func schedule(jobName string, mapFiles []string, nReduce int, phase jobPhase, registerChan chan string) {
	var ntasks int
	var n_other int // number of inputs (for reduce) or outputs (for map)
	switch phase {
	case mapPhase:
		ntasks = len(mapFiles)
		n_other = nReduce
	case reducePhase:
		ntasks = nReduce
		n_other = len(mapFiles)
	}

	fmt.Printf("Schedule: %v %v tasks (%d I/Os)\n", ntasks, phase, n_other)

	// All ntasks tasks have to be scheduled on workers. Once all tasks
	// have completed successfully, schedule() should return.
	//
	// Your code here (Part III, Part IV).
	//
	workerAddrs := make(chan string, 100)
	go func(workerAddrs chan string, registerChan chan string) {
		for {
			addr := <-registerChan
			fmt.Printf("\nadd worker :%s\n", addr)
			workerAddrs <- addr
		}
	}(workerAddrs, registerChan)
	var wg sync.WaitGroup
	switch phase {
	case mapPhase:
		for index, file := range mapFiles {
			wg.Add(1)
			go func(index int, file string) {
				defer wg.Done()
			mapFailed:
				avaWorkerAddr := <-workerAddrs
				taskArgs := DoTaskArgs{JobName: jobName, File: file, Phase: phase, TaskNumber: index, NumOtherPhase: n_other}
				fmt.Printf("\ncall map rpc index: %d file: %s\n", index, file)
				isSuccess := call(avaWorkerAddr, "Worker.DoTask", taskArgs, nil)
				if isSuccess {
					workerAddrs <- avaWorkerAddr
				} else {
					goto mapFailed
				}
			}(index, file)

		}
		wg.Wait()
	case reducePhase:
		for index := 0; index < ntasks; index++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
			reduceFailed:
				avaWorkerAddr := <-workerAddrs
				taskArgs := DoTaskArgs{JobName: jobName, Phase: phase, TaskNumber: index, NumOtherPhase: n_other}
				isSuccess := call(avaWorkerAddr, "Worker.DoTask", taskArgs, nil)
				if isSuccess {
					workerAddrs <- avaWorkerAddr
				} else {
					goto reduceFailed
				}
			}(index)
		}
		wg.Wait()
	}
	fmt.Printf("Schedule: %v done\n", phase)
}
