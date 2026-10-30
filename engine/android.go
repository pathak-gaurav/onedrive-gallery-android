//go:build android

package main

/*
#include <stdlib.h>
*/
import "C"
import "unsafe"

//export GalleryCall
func GalleryCall(op, body *C.char) *C.char { return C.CString(Call(C.GoString(op), C.GoString(body))) }

//export GalleryInitialize
func GalleryInitialize(dir, cert *C.char) *C.char {
	if e := Initialize(C.GoString(dir), C.GoString(cert)); e != nil {
		return C.CString(e.Error())
	}
	return C.CString("")
}

//export GalleryFree
func GalleryFree(p *C.char) { C.free(unsafe.Pointer(p)) }
