//go:build !windows

package main

import("os";"testing";"syscall")
func duplicateTestFD(t *testing.T,f *os.File)int64{t.Helper();copy,e:=syscall.Dup(int(f.Fd()));if e!=nil{t.Fatal(e)};return int64(copy)}
