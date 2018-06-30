package console

import "syscall"
import "unsafe"

const STD_INPUT_HANDLE = uintptr(1) + ^uintptr(10)
const STD_OUTPUT_HANDLE = uintptr(1) + ^uintptr(11)
const STD_ERROR_HANDLE = uintptr(1) + ^uintptr(12)
const ENABLE_VIRTUAL_TERMINAL_PROCESSING uintptr = 0x0004

var g_Kernel32 *syscall.LazyDLL
var g_GetStdHandle *syscall.LazyProc
var g_GetConsoleMode *syscall.LazyProc
var g_SetConsoleMode *syscall.LazyProc

var g_Console uintptr
var g_CurrentMode uintptr

func Initialize() error {
    const STD_INPUT_HANDLE = uintptr(1) + ^uintptr(10)
    const STD_OUTPUT_HANDLE = uintptr(1) + ^uintptr(11)
    const STD_ERROR_HANDLE = uintptr(1) + ^uintptr(12)
    const ENABLE_VIRTUAL_TERMINAL_PROCESSING uintptr = 0x0004

    g_Kernel32 = syscall.NewLazyDLL("kernel32")
    g_GetStdHandle = g_Kernel32.NewProc("GetStdHandle")
    g_GetConsoleMode = g_Kernel32.NewProc("GetConsoleMode")
    g_SetConsoleMode = g_Kernel32.NewProc("SetConsoleMode")

    g_Console, _, _ := g_GetStdHandle.Call(STD_OUTPUT_HANDLE)

    rc, _, err := g_GetConsoleMode.Call(g_Console, uintptr(unsafe.Pointer(&g_CurrentMode)))
    if rc == 0 {
        return err
    }

    rc, _, err = g_SetConsoleMode.Call(g_Console, g_CurrentMode|ENABLE_VIRTUAL_TERMINAL_PROCESSING)
    if rc == 0 {
        return err
    }

    return nil
}

func Finalize() {
    if g_SetConsoleMode == nil {
        return
    }
    g_SetConsoleMode.Call(g_Console, g_CurrentMode)
}
