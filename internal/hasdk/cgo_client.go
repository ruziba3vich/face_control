//go:build hasdk

// cgoClient is the real HASdk-backed implementation. Build with `-tags hasdk`
// once the libHASdk shared object and face model files are present.
//
// HASdk is a C/C++ library. Lifecycle the C side enforces (see HASdk_API_En.pdf):
//   1. HA_Init() once per process
//   2. HA_InitFaceModel(modelDir) once per process (mandatory since v0.9.7)
//   3. HA_Connect(ip, port, user, password) per camera → HA_Cam* handle
//   4. HA_AddJpgFaces / HA_DeleteFaceDataByPersonID using the handle
//   5. HA_DeInit() at shutdown
//
// Handles are cached per device so we don't reconnect on every API call.
//
// TODO: wire actual cgo bindings. Headers, lib path, and model dir are
// expected via CGO_CFLAGS / CGO_LDFLAGS and config.HASdkModelDir.

package hasdk

/*
// #cgo CFLAGS: -I${SRCDIR}/../../third_party/hasdk/include
// #cgo LDFLAGS: -L${SRCDIR}/../../third_party/hasdk/lib -lHASdk
// #include <stdlib.h>
// #include "HASdk.h"
*/
import "C"

import (
	"context"
	"errors"
	"sync"
)

type cgoClient struct {
	mu      sync.Mutex
	handles map[string]uintptr // deviceID -> *C.HA_Cam (as uintptr)
	modelOK bool
}

func NewCgoClient(modelDir string) (*cgoClient, error) {
	// TODO: C.HA_Init()
	// TODO: if C.HA_InitFaceModel(C.CString(modelDir)) != 0 { return nil, errors.New(...) }
	_ = modelDir
	return &cgoClient{handles: map[string]uintptr{}}, errors.New("cgoClient not implemented; build without -tags hasdk to use NoopClient")
}

func (c *cgoClient) Register(_ context.Context, _ RegisterRequest) error {
	// TODO: get-or-create handle via HA_Connect, then HA_AddJpgFaces
	return errors.New("not implemented")
}

func (c *cgoClient) Delete(_ context.Context, _ Device, _ string) error {
	// TODO: HA_DeleteFaceDataByPersonID
	return errors.New("not implemented")
}

func (c *cgoClient) Close() error {
	// TODO: HA_DisConnect each handle, then HA_DeInit
	return nil
}
