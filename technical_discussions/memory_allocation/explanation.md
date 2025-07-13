### Resources

- https://groups.google.com/g/Golang-Nuts/c/D8BTigbetSY?utm_source=chatgpt.com&pli=1
- https://github.com/golang/go/issues/5373

### Question

Does `*ptr = MyStruct{}` allocate memory?

### Short answer

No, it doesn’t allocate any memory or create a temporary. It’s compiled down to simple instructions that just zero out the struct in place.

### Details

I tested this with a struct and a `resetStruct` function marked with `//go:noinline` to prevent inlining (so the assembly is clear). Here’s the key code snippet:

```go
type MyStruct struct {
    A, B, C int
}

//go:noinline
func resetStruct(ptr *MyStruct) {
    ptr.A = 42
    ptr.B = 100
    ptr.C = -1

    *ptr = MyStruct{} // Does NOT allocate, just zeroes out memory
}

```

Disassembling `resetStruct` shows just a couple of zero-store instructions (`STP (ZR, ZR), (R0)`) that overwrite the struct’s memory directly — no heap or stack allocation for a temporary struct is created.

This means `*ptr = MyStruct{}` is optimized as a zeroing operation on existing memory, **not** as an allocation of a new object.

### Full Assembly

```
main.resetStruct STEXT size=16 args=0x8 locals=0x0 funcid=0x0 align=0x0 leaf
	0x0000 00000 (/Users/alexsandergomes/Documents/GenPool/main.go:8)	TEXT	main.resetStruct(SB), LEAF|NOFRAME|ABIInternal, $0-8
	0x0000 00000 (/Users/alexsandergomes/Documents/GenPool/main.go:8)	FUNCDATA	$0, gclocals·2NSbawKySWs0upw55xaGlw==(SB)
	0x0000 00000 (/Users/alexsandergomes/Documents/GenPool/main.go:8)	FUNCDATA	$1, gclocals·ISb46fRPFoZ9pIfykFK/kQ==(SB)
	0x0000 00000 (/Users/alexsandergomes/Documents/GenPool/main.go:8)	FUNCDATA	$5, main.resetStruct.arginfo1(SB)
	0x0000 00000 (/Users/alexsandergomes/Documents/GenPool/main.go:8)	FUNCDATA	$6, main.resetStruct.argliveinfo(SB)
	0x0000 00000 (/Users/alexsandergomes/Documents/GenPool/main.go:8)	PCDATA	$3, $1
	0x0000 00000 (/Users/alexsandergomes/Documents/GenPool/main.go:10)	STP	(ZR, ZR), (R0)
	0x0004 00004 (/Users/alexsandergomes/Documents/GenPool/main.go:15)	MOVD	ZR, 16(R0)
	0x0008 00008 (/Users/alexsandergomes/Documents/GenPool/main.go:16)	RET	(R30)

```

### What if the struct contains pointers?

If the struct contains pointer or reference types (like `*T`, slices, maps, interfaces, or strings), the compiler **cannot** use this bulk zeroing (`memclr`) optimization because the GC needs to track pointer writes carefully (due to write barriers).

Instead, the compiler:

- Zeroes out each field individually, safely setting pointers to `nil`.
- Does this **in-place** on the existing struct memory.
- **Does not allocate any temporary memory**; it just updates the fields directly.

### Check it on your machine

```bash
go tool compile -S main.go > main.s
```
