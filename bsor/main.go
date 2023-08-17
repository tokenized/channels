package main

import (
	"fmt"
	"os"
	"reflect"

	"github.com/tokenized/channels"
	"github.com/tokenized/pkg/bsor"
)

func main() {
	defs, err := bsor.BuildDefinitions(
		reflect.TypeOf(channels.ReplyTo{}),
		reflect.TypeOf(channels.Response{}),
		reflect.TypeOf(channels.Signature{}),
	)
	if err != nil {
		fmt.Printf("Failed to create definitions : %s\n", err)
		return
	}

	file, err := os.Create("channels.bsor")
	if err != nil {
		fmt.Printf("Failed to create file : %s", err)
		return
	}

	if _, err := file.Write([]byte(defs.String() + "\n")); err != nil {
		fmt.Printf("Failed to write file : %s", err)
		return
	}
}
