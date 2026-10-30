//go:build windows

package main

import("os";"testing";"golang.org/x/sys/windows")
func duplicateTestFD(t *testing.T,f *os.File)int64{t.Helper();p:=windows.CurrentProcess();var copy windows.Handle;if e:=windows.DuplicateHandle(p,windows.Handle(f.Fd()),p,&copy,0,false,windows.DUPLICATE_SAME_ACCESS);e!=nil{t.Fatal(e)};return int64(copy)}
