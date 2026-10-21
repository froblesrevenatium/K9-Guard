package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework AppKit -framework Foundation -framework UserNotifications
#include <stdbool.h>
#include <stdlib.h>

bool dbwIsBundled(void);
void dbwSetupNotifications(void);
void dbwNotify(const char *title, const char *body, const char *query);
*/
import "C"

import (
	"log"
	"sync"
	"sync/atomic"
	"unsafe"
)

var (
	nativeNotifications atomic.Bool
	clickMu             sync.Mutex
	clickHandler        func(query string)
)

//export dbwNotificationClicked
func dbwNotificationClicked(query *C.char) {
	name := C.GoString(query)
	clickMu.Lock()
	handler := clickHandler
	clickMu.Unlock()
	if handler != nil {
		handler(name)
	}
}

//export dbwAuthorizationResult
func dbwAuthorizationResult(granted C.int) {
	nativeNotifications.Store(granted == 1)
	log.Printf("notificaciones: permiso concedido=%t", granted == 1)
}

//export dbwLog
func dbwLog(message *C.char) {
	log.Print(C.GoString(message))
}

func setupNativeNotifications(onClick func(query string)) {
	clickMu.Lock()
	clickHandler = onClick
	clickMu.Unlock()
	if bool(C.dbwIsBundled()) {
		C.dbwSetupNotifications()
	} else {
		log.Print("notificaciones: la app no corre como paquete .app, se usa osascript")
	}
}

func nativeNotify(title, body, query string) bool {
	if !nativeNotifications.Load() {
		return false
	}
	cTitle, cBody, cQuery := C.CString(title), C.CString(body), C.CString(query)
	defer C.free(unsafe.Pointer(cTitle))
	defer C.free(unsafe.Pointer(cBody))
	defer C.free(unsafe.Pointer(cQuery))
	C.dbwNotify(cTitle, cBody, cQuery)
	return true
}
