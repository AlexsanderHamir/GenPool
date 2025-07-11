package main

type MyStruct struct {
	A, B, C int
}

//go:noinline
func resetStruct(ptr *MyStruct) {
	// Ensure the struct has data before resetting
	ptr.A = 42
	ptr.B = 100
	ptr.C = -1

	// The key line we're testing
	*ptr = MyStruct{} // Does this create temporary memory?
}

func main() {
	var x MyStruct
	x.A = 1 // Initialize memory
	resetStruct(&x)
}
