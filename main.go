//go:generate go run github.com/dmarkham/enumer -json -text -output=types/json.go -type=Operation,Code types/

package main

import (
	_ "network"  // force initialization of network by empty import
	
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	
	"client"
	. "types"
)

var fileFlag string
var iFlag bool 

func init() {
	flag.StringVar(&fileFlag, "f", "", "Provide a valid file name or path to file to execute the program by reading input from file.")
	flag.BoolVar(&iFlag, "i", false, "Use this flag to execute the program in interactive mode.")
}

func create_files(input_path string, output_path string) (*os.File, *os.File) {
	input, err := os.Open(input_path)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return input, nil
	}

	output, err := os.Create(output_path)
	if err != nil {
		fmt.Println("Error creating file:", err)
	}
	
	return input, output
}

func process(input *os.File, output *os.File) {
	if input == nil || output == nil {
		return 
	}
	defer input.Close()
	defer output.Close()

	dec := json.NewDecoder(input)
	enc := json.NewEncoder(output)

	var err error
	for {
		var request Request

		if input == os.Stdin {
			fmt.Print(">> ")
		}

		err = dec.Decode(&request)
		if err == io.EOF {
			break
		} else if err != nil {
			fmt.Println("Error reading from file:", err)
			return
		}

		output.WriteString("Input: ")
		err = enc.Encode(request)
		if err != nil {
			fmt.Println("Error writing to file:", err)
			return
		}

		response := client.ProcessOp(&request)

		output.WriteString("Output: ")
		err = enc.Encode(response)
		if err != nil {
			fmt.Println("Error writing to file:", err)
			return
		}
	}
}

func main() {
	defer close(client.Requests)

	flag.Parse()

	if (iFlag && fileFlag != "") || (!iFlag && fileFlag == "") {
		fmt.Println("Error in running program. Please provide either only -i flag or -f <path/to/file_name flag.")
		return
	}

	input := os.Stdin
	output := os.Stdout
	if fileFlag != "" {
		input, output = create_files(fileFlag, fileFlag+"_output")
	}

	process(input, output)
}
