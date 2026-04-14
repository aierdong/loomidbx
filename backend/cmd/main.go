package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"unsafe"

	jsoniter "github.com/json-iterator/go"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

//export LDB_Version
func LDB_Version() *C.char {
	b, _ := json.Marshal(map[string]string{"version": "0.0.0-dev"})
	return C.CString(string(b))
}

//export LDB_FreeString
func LDB_FreeString(s *C.char) {
	if s == nil {
		return
	}
	C.free(unsafe.Pointer(s))
}

func main() {}
