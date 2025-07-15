// Package memoryallocation refers to a technical discussion had on Reddit, where it was clarified that the syntax MyStruct{}
// by itself does not allocate memory. When used as *ptr = MyStruct{}, it compiles down to zero instructions in assembly.
package memoryallocation

type myStruct struct {
	A, B, C int
}

//go:noinline
func resetStruct(ptr *myStruct) {
	// Ensure the struct has data before resetting
	ptr.A = 42
	ptr.B = 100
	ptr.C = -1

	// The key line we're testing
	*ptr = myStruct{} // Does this create temporary memory?
}

// RunExample runs the code from the technical discussion.
func RunExample() {
	var x myStruct
	x.A = 1 // Initialize memory
	resetStruct(&x)
}
