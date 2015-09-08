package pxlocal

import "testing"

func TestFreePort(t *testing.T) {
	fp := NewFreePort(42000, 42003)
	//	fp.ListenTCP()
	taddr, lis1, err := fp.ListenTCP()
	if err != nil {
		t.Fatal(err)
	}
	if taddr.Port != 42000 {
		t.Fatalf("expect taddr 42000, but got %v", taddr)
	}

	taddr, lis2, err := fp.ListenTCP()
	if err != nil {
		t.Fatal(err)
	}
	if taddr.Port != 42001 {
		t.Fatalf("expect taddr 42001, but got %v", taddr)
	}
	defer lis2.Close()

	lis1.Close()
	taddr, lis3, err := fp.ListenTCP()
	if err != nil {
		t.Fatal(err)
	}
	if taddr.Port != 42002 {
		t.Fatalf("expect taddr 42002, but got %v", taddr)
	}
	defer lis3.Close()

	taddr, lis4, err := fp.ListenTCP()
	if err != nil {
		t.Fatal(err)
	}
	if taddr.Port != 42000 {
		t.Fatalf("expect taddr 42000, but got %v", taddr)
	}
	defer lis4.Close()
}
