package mapreduce

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"
)

func doReduce(
	jobName string, // the name of the whole MapReduce job
	reduceTask int, // which reduce task this is
	outFile string, // write the output here
	nMap int, // the number of map tasks that were run ("M" in the paper)
	reduceF func(key string, values []string) string,
) {
	//
	// doReduce manages one reduce task: it should read the intermediate
	// files for the task, sort the intermediate key/value pairs by key,
	// call the user-defined reduce function (reduceF) for each key, and
	// write reduceF's output to disk.
	//
	// You'll need to read one intermediate file from each map task;
	// reduceName(jobName, m, reduceTask) yields the file
	// name from map task m.
	//
	// Your doMap() encoded the key/value pairs in the intermediate
	// files, so you will need to decode them. If you used JSON, you can
	// read and decode by creating a decoder and repeatedly calling
	// .Decode(&kv) on it until it returns an error.
	//
	// You may find the first example in the golang sort package
	// documentation useful.
	//
	// reduceF() is the application's reduce function. You should
	// call it once per distinct key, with a slice of all the values
	// for that key. reduceF() returns the reduced value for that key.
	//
	// You should write the reduce output as JSON encoded KeyValue
	// objects to the file named outFile. We require you to use JSON
	// because that is what the merger than combines the output
	// from all the reduce tasks expects. There is nothing special about
	// JSON -- it is just the marshalling format we chose to use. Your
	// output code will look something like this:
	//
	// enc := json.NewEncoder(file)
	// for key := ... {
	// 	enc.Encode(KeyValue{key, reduceF(...)})
	// }
	// file.Close()
	//
	// Your code here (Part I).
	//
	kvs := make(map[string][]string)
	keys := make([]string, 0)
	for i := 0; i < nMap; i++ {
		bytes, e := ioutil.ReadFile(reduceName(jobName, i, reduceTask))
		if e != nil {
			fmt.Printf("read error:%v", e)
		}
		input := string(bytes)
		input = strings.Trim(input, "\n")
		result := strings.Split(input, "\n")

		for _, s := range result {
			kv := &KeyValue{}
			err := json.Unmarshal([]byte(s), kv)
			if err != nil {
				fmt.Printf(" error :%v", err)
			}
			k := kv.Key
			v := kv.Value
			_, ok := kvs[k]
			var l []string
			if ok {
				l = kvs[k]
			} else {
				l = make([]string, 0)
			}
			l = append(l, v)
			kvs[k] = l
		}
		for k, _ := range kvs {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	//outName := mergeName(jobName, reduceTask)
	outputFile, outputError := os.OpenFile(outFile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
	if outputError != nil {
		fmt.Printf("An error occurred with file opening or creation\n")
		return
	}
	outputWriter := bufio.NewWriter(outputFile)
	enc := json.NewEncoder(outputWriter)
	for _, key := range keys {
		valueList := kvs[key]
		enc.Encode(KeyValue{key, reduceF(key, valueList)})
	}
	outputWriter.Flush()
	outputFile.Close()
}
