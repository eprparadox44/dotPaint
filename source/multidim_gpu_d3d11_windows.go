//go:build windows

package main

import (
	"fmt"
	"math"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// A206 adds a deliberately narrow Direct3D 11 backend for the two expensive
// high-quality paths: Schwarzschild ray tracing and opaque 4-D face rastering
// (especially the Klein bottle).  The existing CPU renderer remains the exact
// compatibility fallback for unsupported/remote/old graphics configurations.

const (
	mdD3DDriverTypeHardware = 1
	mdD3D11SDKVersion       = 7
	mdD3D11CreateBGRA       = 0x20

	mdD3DFeatureLevel110 = 0xB000
	mdD3DFeatureLevel101 = 0xA100
	mdD3DFeatureLevel100 = 0xA000

	mdDXGIFormatR32G32B32A32Float = 2
	mdDXGIFormatR32G32B32Float    = 6
	mdDXGIFormatR8G8B8A8UNorm     = 28
	mdDXGIFormatD32Float          = 40

	mdD3D11UsageDefault = 0
	mdD3D11UsageDynamic = 2
	mdD3D11UsageStaging = 3

	mdD3D11BindVertexBuffer    = 0x1
	mdD3D11BindShaderResource  = 0x8
	mdD3D11BindConstantBuffer  = 0x4
	mdD3D11BindDepthStencil    = 0x40
	mdD3D11BindRenderTarget    = 0x20
	mdD3D11BindUnorderedAccess = 0x80

	mdD3D11ResourceMiscBufferStructured = 0x40
	mdDXGIFormatUnknown                 = 0
	mdD3D11SRVDimensionBuffer           = 1
	mdD3D11UAVDimensionBuffer           = 1

	mdD3D11CPUAccessWrite = 0x10000
	mdD3D11CPUAccessRead  = 0x20000

	mdD3D11MapRead         = 1
	mdD3D11MapWriteDiscard = 4

	mdD3D11ClearDepth = 0x1

	mdD3D11PrimitiveTriangleList = 4
	mdD3D11InputPerVertexData    = 0
	mdD3D11FillSolid             = 3
	mdD3D11CullNone              = 1

	mdD3D11FilterMinMagMipLinear = 0x15
	mdD3D11TextureAddressClamp   = 3
	mdD3D11ComparisonAlways      = 8

	mdD3D11BlendOne         = 2
	mdD3D11BlendSrcAlpha    = 5
	mdD3D11BlendInvSrcAlpha = 6
	mdD3D11BlendOpAdd       = 1
	mdD3D11ColorWriteAll    = 0x0F
)

type mdD3D11SampleDesc struct {
	Count   uint32
	Quality uint32
}

type mdD3D11Texture2DDesc struct {
	Width          uint32
	Height         uint32
	MipLevels      uint32
	ArraySize      uint32
	Format         uint32
	SampleDesc     mdD3D11SampleDesc
	Usage          uint32
	BindFlags      uint32
	CPUAccessFlags uint32
	MiscFlags      uint32
}

type mdD3D11BufferDesc struct {
	ByteWidth           uint32
	Usage               uint32
	BindFlags           uint32
	CPUAccessFlags      uint32
	MiscFlags           uint32
	StructureByteStride uint32
}

type mdD3D11MappedSubresource struct {
	Data       uintptr
	RowPitch   uint32
	DepthPitch uint32
}

type mdD3D11SubresourceData struct {
	SysMem           uintptr
	SysMemPitch      uint32
	SysMemSlicePitch uint32
}

type mdD3D11ShaderResourceViewDesc struct {
	Format        uint32
	ViewDimension uint32
	FirstElement  uint32
	NumElements   uint32
	Extra         [2]uint32 // complete the 16-byte SRV union used by larger view types
}

type mdD3D11UnorderedAccessViewDesc struct {
	Format        uint32
	ViewDimension uint32
	FirstElement  uint32
	NumElements   uint32
	Flags         uint32
}

type mdD3D11InputElementDesc struct {
	SemanticName         uintptr
	SemanticIndex        uint32
	Format               uint32
	InputSlot            uint32
	AlignedByteOffset    uint32
	InputSlotClass       uint32
	InstanceDataStepRate uint32
}

type mdD3D11RasterizerDesc struct {
	FillMode              uint32
	CullMode              uint32
	FrontCounterClockwise int32
	DepthBias             int32
	DepthBiasClamp        float32
	SlopeScaledDepthBias  float32
	DepthClipEnable       int32
	ScissorEnable         int32
	MultisampleEnable     int32
	AntialiasedLineEnable int32
}

type mdD3D11SamplerDesc struct {
	Filter         uint32
	AddressU       uint32
	AddressV       uint32
	AddressW       uint32
	MipLODBias     float32
	MaxAnisotropy  uint32
	ComparisonFunc uint32
	BorderColor    [4]float32
	MinLOD         float32
	MaxLOD         float32
}

type mdD3D11RenderTargetBlendDesc struct {
	BlendEnable           int32
	SrcBlend              uint32
	DestBlend             uint32
	BlendOp               uint32
	SrcBlendAlpha         uint32
	DestBlendAlpha        uint32
	BlendOpAlpha          uint32
	RenderTargetWriteMask uint32
}

type mdD3D11BlendDesc struct {
	AlphaToCoverageEnable  int32
	IndependentBlendEnable int32
	RenderTarget           [8]mdD3D11RenderTargetBlendDesc
}

type mdD3D11Viewport struct {
	TopLeftX float32
	TopLeftY float32
	Width    float32
	Height   float32
	MinDepth float32
	MaxDepth float32
}

type mdD3D11BHConstants struct {
	Width, Height, Quality, Mode   uint32
	TanX, TanY, Pad0, Pad1         float32
	DiskX, DiskY, DiskZ, Pad2      float32
	Spin, Horizon, DiskInner, Pad3 float32
}

type mdD3D11CelestialConstants struct {
	Width, Height, Skin, MaskPass        uint32
	Radius, CenterX, CenterY, PolarScale float32
	InvRow0                              [4]float32
	InvRow1                              [4]float32
	InvRow2                              [4]float32
}

type mdD3D11FaceVertex struct {
	X, Y, Z    float32
	R, G, B, A float32
}

const mdD3D11OcclusionMaxFaceVerts = 8

type mdD3D11OcclusionFace struct {
	V     [mdD3D11OcclusionMaxFaceVerts][4]float32 // x, z, y, unused
	Count uint32
	Pad   [3]uint32
}

type mdD3D11OcclusionQuery struct {
	X, Z, Y, DepthEpsilon float32
}

type mdD3D11OcclusionPair struct {
	QueryIndex uint32
	FaceIndex  uint32
	Pad        [2]uint32
}

type mdD3D11FieldSample struct {
	X, Y, Z, Soft                  float32
	Q, GateOffset, GateX, GateY    float32
	GateZ, GateEnabled, Pad0, Pad1 float32
}

type mdD3D11FieldConstants struct {
	Steps, SampleCount, TotalCount, Pad0 uint32
	MinCoord, StepSize, SoftFactor, Pad1 float32
}

type mdD3D11FieldVectorSegment struct {
	MidX, MidY, MidZ, Soft float32
	DLX, DLY, DLZ, Current float32
}

type mdD3D11FieldVectorWire struct {
	CenterX, CenterY, CenterZ, Pad0 float32
	AxisX, AxisY, AxisZ, Current    float32
}

type mdD3D11FieldVectorConstants struct {
	Steps, ChargeCount, SegmentCount, WireCount uint32
	TotalCount, Mode, Pad0, Pad1                uint32
	MinCoord, StepSize, SoftFactor, Pad2        float32
}

type mdD3D11FieldVectorValue struct {
	X, Y, Z, Magnitude float32
}

type mdD3D11Backend struct {
	mu sync.Mutex

	device       uintptr
	context      uintptr
	featureLevel uint32
	deviceReady  bool
	deviceErr    string

	blackHoleReady         bool
	blackHoleErr           string
	blackHoleCS            uintptr
	blackHoleCB            uintptr
	blackHoleTexture       uintptr
	blackHoleUAV           uintptr
	blackHoleStaging       uintptr
	blackHoleW, blackHoleH int

	celestialAttempted                 bool
	celestialReady                     bool
	celestialErr                       string
	celestialCS                        uintptr
	celestialCB                        uintptr
	celestialSampler                   uintptr
	celestialTextures                  [4]uintptr
	celestialSRVs                      [4]uintptr
	celestialAssetsReady               bool
	celestialAssetsErr                 string
	celestialOutputTexture             uintptr
	celestialOutputUAV                 uintptr
	celestialOutputStaging             uintptr
	celestialOutputW, celestialOutputH int

	faceReady       bool
	faceErr         string
	faceVS          uintptr
	facePS          uintptr
	faceLayout      uintptr
	faceRasterState uintptr
	faceTexture     uintptr
	faceRTV         uintptr
	faceDepth       uintptr
	faceDepthClear  uintptr
	faceDSV         uintptr
	faceStaging     uintptr
	faceVB          uintptr
	faceVBCap       int
	faceW, faceH    int

	occlusionReady         bool
	occlusionErr           string
	occlusionCS            uintptr
	occlusionFacesBuffer   uintptr
	occlusionFacesSRV      uintptr
	occlusionFacesCap      int
	occlusionQueriesBuffer uintptr
	occlusionQueriesSRV    uintptr
	occlusionQueriesCap    int
	occlusionPairsBuffer   uintptr
	occlusionPairsSRV      uintptr
	occlusionPairsCap      int
	occlusionCountsBuffer  uintptr
	occlusionCountsUAV     uintptr
	occlusionCountsStaging uintptr
	occlusionCountsCap     int

	fieldReady         bool
	fieldErr           string
	fieldCS            uintptr
	fieldCB            uintptr
	fieldSamplesBuffer uintptr
	fieldSamplesSRV    uintptr
	fieldSamplesCap    int
	fieldValuesBuffer  uintptr
	fieldValuesUAV     uintptr
	fieldValuesStaging uintptr
	fieldValuesCap     int
	fieldSurfaceReady  bool
	fieldSurfaceErr    string
	fieldBlendState    uintptr

	fieldVectorReady          bool
	fieldVectorErr            string
	fieldVectorCS             uintptr
	fieldVectorCB             uintptr
	fieldVectorSegmentsBuffer uintptr
	fieldVectorSegmentsSRV    uintptr
	fieldVectorSegmentsCap    int
	fieldVectorWiresBuffer    uintptr
	fieldVectorWiresSRV       uintptr
	fieldVectorWiresCap       int
	fieldVectorValuesBuffer   uintptr
	fieldVectorValuesUAV      uintptr
	fieldVectorValuesStaging  uintptr
	fieldVectorValuesCap      int
}

var (
	mdD3D11Once             sync.Once
	mdD3D11PrewarmOnce      sync.Once
	mdD3D11FieldPrewarmOnce sync.Once
	mdD3D11Global           *mdD3D11Backend
	mdD3D11DLL              = syscall.NewLazyDLL("d3d11.dll")
	mdD3D11Create           = mdD3D11DLL.NewProc("D3D11CreateDevice")
	mdD3DCompiler           *syscall.LazyDLL
	mdD3DCompileFn          *syscall.LazyProc
	mdD3D11FallbackLog      sync.Map
)

func mdHRESULTFailed(v uintptr) bool { return int32(v) < 0 }

func mdCOMMethod(obj uintptr, index int) uintptr {
	if obj == 0 {
		return 0
	}
	vt := *(*uintptr)(unsafe.Pointer(obj))
	return *(*uintptr)(unsafe.Pointer(vt + uintptr(index)*unsafe.Sizeof(uintptr(0))))
}

func mdCOMCall(obj uintptr, index int, args ...uintptr) uintptr {
	fn := mdCOMMethod(obj, index)
	if fn == 0 {
		return ^uintptr(0)
	}
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, obj)
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

func mdCOMRelease(obj *uintptr) {
	if obj == nil || *obj == 0 {
		return
	}
	mdCOMCall(*obj, 2)
	*obj = 0
}

func mdD3D11LoadCompiler() error {
	if mdD3DCompileFn != nil {
		return nil
	}
	for _, name := range []string{"d3dcompiler_47.dll", "d3dcompiler_43.dll"} {
		dll := syscall.NewLazyDLL(name)
		if err := dll.Load(); err != nil {
			continue
		}
		proc := dll.NewProc("D3DCompile")
		if err := proc.Find(); err == nil {
			mdD3DCompiler = dll
			mdD3DCompileFn = proc
			return nil
		}
	}
	return fmt.Errorf("D3DCompile을 제공하는 d3dcompiler DLL을 찾지 못했습니다")
}

func mdD3DBlobBytes(blob uintptr) []byte {
	if blob == 0 {
		return nil
	}
	ptr := mdCOMCall(blob, 3)
	sz := mdCOMCall(blob, 4)
	if ptr == 0 || sz == 0 || sz > 1<<30 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(ptr)), int(sz))
}

func mdD3DCompileShader(source, entry, target string) ([]byte, error) {
	if err := mdD3D11LoadCompiler(); err != nil {
		return nil, err
	}
	src := []byte(source)
	entryZ := append([]byte(entry), 0)
	targetZ := append([]byte(target), 0)
	var code, errs uintptr
	r, _, _ := mdD3DCompileFn.Call(
		uintptr(unsafe.Pointer(&src[0])), uintptr(len(src)), 0, 0, 0,
		uintptr(unsafe.Pointer(&entryZ[0])), uintptr(unsafe.Pointer(&targetZ[0])),
		0, 0, uintptr(unsafe.Pointer(&code)), uintptr(unsafe.Pointer(&errs)),
	)
	defer mdCOMRelease(&code)
	defer mdCOMRelease(&errs)
	if mdHRESULTFailed(r) {
		msg := "알 수 없는 HLSL 컴파일 오류"
		if b := mdD3DBlobBytes(errs); len(b) > 0 {
			msg = string(b)
		}
		return nil, fmt.Errorf("%s/%s: %s", entry, target, msg)
	}
	b := mdD3DBlobBytes(code)
	if len(b) == 0 {
		return nil, fmt.Errorf("%s/%s 셰이더 바이트 코드가 비어 있습니다", entry, target)
	}
	out := append([]byte(nil), b...)
	return out, nil
}

func mdD3D11Get() *mdD3D11Backend {
	mdD3D11Once.Do(func() {
		b := &mdD3D11Backend{}
		b.init()
		mdD3D11Global = b
	})
	return mdD3D11Global
}

func mdD3D11Available() bool {
	b := mdD3D11Get()
	return b != nil && b.deviceReady
}

func mdD3D11BlackHoleAvailable() bool {
	b := mdD3D11Get()
	return b != nil && b.deviceReady && b.blackHoleReady
}

func mdD3D11FaceAvailable() bool {
	b := mdD3D11Get()
	return b != nil && b.deviceReady && b.faceReady
}

func mdD3D11OcclusionAvailable() bool {
	b := mdD3D11Get()
	return b != nil && b.deviceReady && b.occlusionReady
}

func mdD3D11FieldAvailable() bool {
	b := mdD3D11Get()
	return b != nil && b.deviceReady && b.fieldReady
}

func mdD3D11FieldVectorAvailable() bool {
	b := mdD3D11Get()
	return b != nil && b.deviceReady && b.fieldVectorReady
}

func mdD3D11FieldSurfaceAvailable() bool {
	b := mdD3D11Get()
	return b != nil && b.deviceReady && b.faceReady && b.fieldSurfaceReady
}

func mdD3D11CelestialAvailable() bool {
	b := mdD3D11Get()
	if b == nil || !b.deviceReady {
		return false
	}
	b.mu.Lock()
	err := b.ensureCelestialPipeline()
	ready := b.celestialReady && err == nil
	b.mu.Unlock()
	return ready
}

func mdD3D11PipelineReason(pipeline string) string {
	b := mdD3D11Get()
	if b == nil {
		return "D3D11 backend missing"
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.deviceReady {
		if b.deviceErr != "" {
			return b.deviceErr
		}
		return "D3D11 device unavailable"
	}
	switch pipeline {
	case "blackhole":
		if b.blackHoleReady {
			return ""
		}
		return b.blackHoleErr
	case "faces":
		if b.faceReady {
			return ""
		}
		return b.faceErr
	case "occlusion":
		if b.occlusionReady {
			return ""
		}
		return b.occlusionErr
	case "field":
		if b.fieldReady {
			return ""
		}
		return b.fieldErr
	case "field-vector":
		if b.fieldVectorReady {
			return ""
		}
		return b.fieldVectorErr
	case "field-surface":
		if b.fieldSurfaceReady && b.faceReady {
			return ""
		}
		if b.fieldSurfaceErr != "" {
			return b.fieldSurfaceErr
		}
		return b.faceErr
	case "celestial":
		if b.celestialReady {
			return ""
		}
		if b.celestialAssetsErr != "" {
			return b.celestialAssetsErr
		}
		return b.celestialErr
	}
	return "unknown pipeline"
}

func mdD3D11LogFallbackOnce(pipeline, reason string) {
	if reason == "" {
		reason = "pipeline unavailable"
	}
	key := pipeline + "\x00" + reason
	if _, loaded := mdD3D11FallbackLog.LoadOrStore(key, struct{}{}); !loaded {
		logf("MULTIDIM_GPU fallback pipeline=%s reason=%q", pipeline, reason)
	}
}

func mdD3D11Prewarm() {
	mdD3D11PrewarmOnce.Do(func() {
		go func() {
			started := time.Now()
			b := mdD3D11Get()
			if b == nil || !b.deviceReady {
				if b != nil {
					mdD3D11LogFallbackOnce("device", b.deviceErr)
				}
				return
			}
			b.mu.Lock()
			err := b.ensureCelestialAssets()
			blackHoleReady := b.blackHoleReady
			faceReady := b.faceReady
			occlusionReady := b.occlusionReady
			fieldReady := b.fieldReady
			fieldVectorReady := b.fieldVectorReady
			celestialReady := b.celestialReady && b.celestialAssetsReady
			b.mu.Unlock()
			if err != nil {
				logf("MULTIDIM_GPU prewarm celestial failed elapsed=%s err=%v", time.Since(started), err)
			}
			logf("MULTIDIM_GPU prewarm complete total=%s blackhole=%t faces=%t occlusion=%t field=%t field_vector=%t celestial=%t", time.Since(started), blackHoleReady, faceReady, occlusionReady, fieldReady, fieldVectorReady, celestialReady)
		}()
	})
}

func mdD3D11PrewarmField() {
	mdD3D11FieldPrewarmOnce.Do(func() {
		go func() {
			started := time.Now()
			ready := mdD3D11FieldAvailable()
			vectorReady := mdD3D11FieldVectorAvailable()
			surfaceReady := mdD3D11FieldSurfaceAvailable()
			reason := ""
			if !ready {
				reason = mdD3D11PipelineReason("field")
			} else if !vectorReady {
				reason = mdD3D11PipelineReason("field-vector")
			} else if !surfaceReady {
				reason = mdD3D11PipelineReason("field-surface")
			}
			logf("FIELD_GPU prewarm potential=%t vector=%t membrane=%t elapsed=%s reason=%q", ready, vectorReady, surfaceReady, time.Since(started), reason)
		}()
	})
}

func mdD3D11StatusText() string {
	b := mdD3D11Get()
	if b == nil || !b.deviceReady {
		return "CPU 호환 렌더러"
	}
	b.mu.Lock()
	bh, faces, occlusion, field, fieldVector, fieldSurface, celestial := b.blackHoleReady, b.faceReady, b.occlusionReady, b.fieldReady, b.fieldVectorReady, b.fieldSurfaceReady, b.celestialReady
	level := b.featureLevel
	b.mu.Unlock()
	backend := func(ok bool) string {
		if ok {
			return "GPU"
		}
		return "CPU"
	}
	return fmt.Sprintf("Direct3D 11 (%x) · 블랙홀 %s · 면 %s · 가림 %s · 전위격자 %s · 장벡터 %s · 등전위막 %s · 천체 %s", level, backend(bh), backend(faces), backend(occlusion), backend(field), backend(fieldVector), backend(fieldSurface), backend(celestial))
}

func (b *mdD3D11Backend) init() {
	started := time.Now()
	logf("MULTIDIM_GPU device begin api=Direct3D11 driver=hardware")
	levels := []uint32{mdD3DFeatureLevel110}
	var device, context uintptr
	var level uint32
	r, _, _ := mdD3D11Create.Call(
		0, mdD3DDriverTypeHardware, 0, mdD3D11CreateBGRA,
		uintptr(unsafe.Pointer(&levels[0])), uintptr(len(levels)), mdD3D11SDKVersion,
		uintptr(unsafe.Pointer(&device)), uintptr(unsafe.Pointer(&level)), uintptr(unsafe.Pointer(&context)),
	)
	if mdHRESULTFailed(r) || device == 0 || context == 0 {
		b.deviceErr = fmt.Sprintf("D3D11CreateDevice failed HRESULT=%#x", uint32(r))
		logf("MULTIDIM_GPU device failed elapsed=%s err=%q", time.Since(started), b.deviceErr)
		return
	}
	b.device, b.context, b.featureLevel, b.deviceReady = device, context, level, true
	logf("MULTIDIM_GPU device ready api=Direct3D11 featureLevel=%#x init=%s", b.featureLevel, time.Since(started))
	b.initBlackHolePipeline()
	b.initFacePipeline()
	b.initOcclusionPipeline()
	b.initFieldPipeline()
	b.initFieldVectorPipeline()
	logf("MULTIDIM_GPU backend blackhole=%s faces=%s occlusion=%s field=%s field_vector=%s celestial=lazy", mdD3D11BackendName(b.blackHoleReady), mdD3D11BackendName(b.faceReady), mdD3D11BackendName(b.occlusionReady), mdD3D11BackendName(b.fieldReady), mdD3D11BackendName(b.fieldVectorReady))
}

func mdD3D11BackendName(ready bool) string {
	if ready {
		return "D3D11"
	}
	return "CPU"
}

func (b *mdD3D11Backend) initBlackHolePipeline() {
	started := time.Now()
	code, err := mdD3DCompileShader(mdD3D11BlackHoleShader, "CSMain", "cs_5_0")
	if err != nil {
		b.blackHoleErr = err.Error()
		logf("MULTIDIM_GPU shader name=blackhole target=cs_5_0 ok=false elapsed=%s err=%q", time.Since(started), b.blackHoleErr)
		return
	}
	compiled := time.Now()
	if hr := mdCOMCall(b.device, 18, uintptr(unsafe.Pointer(&code[0])), uintptr(len(code)), 0, uintptr(unsafe.Pointer(&b.blackHoleCS))); mdHRESULTFailed(hr) || b.blackHoleCS == 0 {
		b.blackHoleErr = fmt.Sprintf("CreateComputeShader(blackhole) failed HRESULT=%#x", uint32(hr))
		logf("MULTIDIM_GPU shader name=blackhole target=cs_5_0 ok=false compile=%s create=%s err=%q", compiled.Sub(started), time.Since(compiled), b.blackHoleErr)
		return
	}
	cbDesc := mdD3D11BufferDesc{
		ByteWidth: uint32(unsafe.Sizeof(mdD3D11BHConstants{})), Usage: mdD3D11UsageDynamic,
		BindFlags: mdD3D11BindConstantBuffer, CPUAccessFlags: mdD3D11CPUAccessWrite,
	}
	if hr := mdCOMCall(b.device, 3, uintptr(unsafe.Pointer(&cbDesc)), 0, uintptr(unsafe.Pointer(&b.blackHoleCB))); mdHRESULTFailed(hr) || b.blackHoleCB == 0 {
		b.blackHoleErr = fmt.Sprintf("blackhole constant buffer failed HRESULT=%#x", uint32(hr))
		mdCOMRelease(&b.blackHoleCS)
		logf("MULTIDIM_GPU shader name=blackhole target=cs_5_0 ok=false compile=%s create=%s err=%q", compiled.Sub(started), time.Since(compiled), b.blackHoleErr)
		return
	}
	b.blackHoleReady = true
	logf("MULTIDIM_GPU shader name=blackhole target=cs_5_0 ok=true bytes=%d compile=%s create=%s total=%s", len(code), compiled.Sub(started), time.Since(compiled), time.Since(started))
}

func (b *mdD3D11Backend) initFacePipeline() {
	started := time.Now()
	vsStarted := time.Now()
	vsCode, err := mdD3DCompileShader(mdD3D11FaceShader, "VSMain", "vs_5_0")
	if err != nil {
		b.faceErr = err.Error()
		logf("MULTIDIM_GPU shader name=faces-vs target=vs_5_0 ok=false elapsed=%s err=%q", time.Since(vsStarted), b.faceErr)
		return
	}
	vsElapsed := time.Since(vsStarted)
	psStarted := time.Now()
	psCode, err := mdD3DCompileShader(mdD3D11FaceShader, "PSMain", "ps_5_0")
	if err != nil {
		b.faceErr = err.Error()
		logf("MULTIDIM_GPU shader name=faces-ps target=ps_5_0 ok=false elapsed=%s err=%q", time.Since(psStarted), b.faceErr)
		return
	}
	psElapsed := time.Since(psStarted)
	if hr := mdCOMCall(b.device, 12, uintptr(unsafe.Pointer(&vsCode[0])), uintptr(len(vsCode)), 0, uintptr(unsafe.Pointer(&b.faceVS))); mdHRESULTFailed(hr) || b.faceVS == 0 {
		b.faceErr = fmt.Sprintf("CreateVertexShader failed HRESULT=%#x", uint32(hr))
		logf("MULTIDIM_GPU shader name=faces-vs target=vs_5_0 ok=false err=%q", b.faceErr)
		return
	}
	if hr := mdCOMCall(b.device, 15, uintptr(unsafe.Pointer(&psCode[0])), uintptr(len(psCode)), 0, uintptr(unsafe.Pointer(&b.facePS))); mdHRESULTFailed(hr) || b.facePS == 0 {
		b.faceErr = fmt.Sprintf("CreatePixelShader failed HRESULT=%#x", uint32(hr))
		mdCOMRelease(&b.faceVS)
		logf("MULTIDIM_GPU shader name=faces-ps target=ps_5_0 ok=false err=%q", b.faceErr)
		return
	}
	posName := append([]byte("POSITION"), 0)
	colName := append([]byte("COLOR"), 0)
	elements := []mdD3D11InputElementDesc{
		{SemanticName: uintptr(unsafe.Pointer(&posName[0])), Format: mdDXGIFormatR32G32B32Float, AlignedByteOffset: 0, InputSlotClass: mdD3D11InputPerVertexData},
		{SemanticName: uintptr(unsafe.Pointer(&colName[0])), Format: mdDXGIFormatR32G32B32A32Float, AlignedByteOffset: 12, InputSlotClass: mdD3D11InputPerVertexData},
	}
	if hr := mdCOMCall(b.device, 11, uintptr(unsafe.Pointer(&elements[0])), uintptr(len(elements)), uintptr(unsafe.Pointer(&vsCode[0])), uintptr(len(vsCode)), uintptr(unsafe.Pointer(&b.faceLayout))); mdHRESULTFailed(hr) || b.faceLayout == 0 {
		b.faceErr = fmt.Sprintf("CreateInputLayout failed HRESULT=%#x", uint32(hr))
		b.releaseFacePipeline()
		logf("MULTIDIM_GPU shader name=opaque-faces ok=false err=%q", b.faceErr)
		return
	}
	rsDesc := mdD3D11RasterizerDesc{FillMode: mdD3D11FillSolid, CullMode: mdD3D11CullNone, DepthClipEnable: 1}
	if hr := mdCOMCall(b.device, 22, uintptr(unsafe.Pointer(&rsDesc)), uintptr(unsafe.Pointer(&b.faceRasterState))); mdHRESULTFailed(hr) || b.faceRasterState == 0 {
		b.faceErr = fmt.Sprintf("CreateRasterizerState failed HRESULT=%#x", uint32(hr))
		b.releaseFacePipeline()
		logf("MULTIDIM_GPU shader name=opaque-faces ok=false err=%q", b.faceErr)
		return
	}
	b.faceReady = true
	logf("MULTIDIM_GPU shader name=opaque-faces ok=true vs_bytes=%d ps_bytes=%d vs_compile=%s ps_compile=%s total=%s", len(vsCode), len(psCode), vsElapsed, psElapsed, time.Since(started))
}

func (b *mdD3D11Backend) initOcclusionPipeline() {
	started := time.Now()
	code, err := mdD3DCompileShader(mdD3D11OcclusionShader, "CSMain", "cs_5_0")
	if err != nil {
		b.occlusionErr = err.Error()
		logf("MULTIDIM_GPU shader name=occlusion target=cs_5_0 ok=false elapsed=%s err=%q", time.Since(started), b.occlusionErr)
		return
	}
	compiled := time.Now()
	if hr := mdCOMCall(b.device, 18, uintptr(unsafe.Pointer(&code[0])), uintptr(len(code)), 0, uintptr(unsafe.Pointer(&b.occlusionCS))); mdHRESULTFailed(hr) || b.occlusionCS == 0 {
		b.occlusionErr = fmt.Sprintf("CreateComputeShader(occlusion) failed HRESULT=%#x", uint32(hr))
		logf("MULTIDIM_GPU shader name=occlusion target=cs_5_0 ok=false compile=%s create=%s err=%q", compiled.Sub(started), time.Since(compiled), b.occlusionErr)
		return
	}
	b.occlusionReady = true
	logf("MULTIDIM_GPU shader name=occlusion target=cs_5_0 ok=true bytes=%d compile=%s create=%s total=%s", len(code), compiled.Sub(started), time.Since(compiled), time.Since(started))
}

func (b *mdD3D11Backend) initFieldPipeline() {
	started := time.Now()
	code, err := mdD3DCompileShader(mdD3D11FieldPotentialShader, "CSMain", "cs_5_0")
	if err != nil {
		b.fieldErr = err.Error()
		logf("MULTIDIM_GPU shader name=field-potential target=cs_5_0 ok=false elapsed=%s err=%q", time.Since(started), b.fieldErr)
		return
	}
	compiled := time.Now()
	if hr := mdCOMCall(b.device, 18, uintptr(unsafe.Pointer(&code[0])), uintptr(len(code)), 0, uintptr(unsafe.Pointer(&b.fieldCS))); mdHRESULTFailed(hr) || b.fieldCS == 0 {
		b.fieldErr = fmt.Sprintf("CreateComputeShader(field-potential) failed HRESULT=%#x", uint32(hr))
		logf("MULTIDIM_GPU shader name=field-potential target=cs_5_0 ok=false compile=%s create=%s err=%q", compiled.Sub(started), time.Since(compiled), b.fieldErr)
		return
	}
	cbDesc := mdD3D11BufferDesc{
		ByteWidth: uint32(unsafe.Sizeof(mdD3D11FieldConstants{})), Usage: mdD3D11UsageDynamic,
		BindFlags: mdD3D11BindConstantBuffer, CPUAccessFlags: mdD3D11CPUAccessWrite,
	}
	if hr := mdCOMCall(b.device, 3, uintptr(unsafe.Pointer(&cbDesc)), 0, uintptr(unsafe.Pointer(&b.fieldCB))); mdHRESULTFailed(hr) || b.fieldCB == 0 {
		b.fieldErr = fmt.Sprintf("field-potential constant buffer failed HRESULT=%#x", uint32(hr))
		mdCOMRelease(&b.fieldCS)
		logf("MULTIDIM_GPU shader name=field-potential target=cs_5_0 ok=false err=%q", b.fieldErr)
		return
	}
	b.fieldReady = true
	if b.faceReady {
		blendDesc := mdD3D11BlendDesc{}
		blendDesc.RenderTarget[0] = mdD3D11RenderTargetBlendDesc{
			BlendEnable: 1,
			SrcBlend:    mdD3D11BlendSrcAlpha, DestBlend: mdD3D11BlendInvSrcAlpha, BlendOp: mdD3D11BlendOpAdd,
			SrcBlendAlpha: mdD3D11BlendOne, DestBlendAlpha: mdD3D11BlendInvSrcAlpha, BlendOpAlpha: mdD3D11BlendOpAdd,
			RenderTargetWriteMask: mdD3D11ColorWriteAll,
		}
		if hr := mdCOMCall(b.device, 20, uintptr(unsafe.Pointer(&blendDesc)), uintptr(unsafe.Pointer(&b.fieldBlendState))); mdHRESULTFailed(hr) || b.fieldBlendState == 0 {
			b.fieldSurfaceErr = fmt.Sprintf("CreateBlendState(field-membrane) failed HRESULT=%#x", uint32(hr))
		} else {
			b.fieldSurfaceReady = true
		}
	} else {
		b.fieldSurfaceErr = b.faceErr
	}
	logf("MULTIDIM_GPU shader name=field-potential target=cs_5_0 ok=true bytes=%d compile=%s create=%s total=%s membrane=%s membrane_error=%q", len(code), compiled.Sub(started), time.Since(compiled), time.Since(started), mdD3D11BackendName(b.fieldSurfaceReady), b.fieldSurfaceErr)
}

func (b *mdD3D11Backend) initFieldVectorPipeline() {
	started := time.Now()
	code, err := mdD3DCompileShader(mdD3D11FieldVectorShader, "CSMain", "cs_5_0")
	if err != nil {
		b.fieldVectorErr = err.Error()
		logf("MULTIDIM_GPU shader name=field-vector target=cs_5_0 ok=false elapsed=%s err=%q", time.Since(started), b.fieldVectorErr)
		return
	}
	compiled := time.Now()
	if hr := mdCOMCall(b.device, 18, uintptr(unsafe.Pointer(&code[0])), uintptr(len(code)), 0, uintptr(unsafe.Pointer(&b.fieldVectorCS))); mdHRESULTFailed(hr) || b.fieldVectorCS == 0 {
		b.fieldVectorErr = fmt.Sprintf("CreateComputeShader(field-vector) failed HRESULT=%#x", uint32(hr))
		logf("MULTIDIM_GPU shader name=field-vector target=cs_5_0 ok=false compile=%s create=%s err=%q", compiled.Sub(started), time.Since(compiled), b.fieldVectorErr)
		return
	}
	cbDesc := mdD3D11BufferDesc{
		ByteWidth: uint32(unsafe.Sizeof(mdD3D11FieldVectorConstants{})), Usage: mdD3D11UsageDynamic,
		BindFlags: mdD3D11BindConstantBuffer, CPUAccessFlags: mdD3D11CPUAccessWrite,
	}
	if hr := mdCOMCall(b.device, 3, uintptr(unsafe.Pointer(&cbDesc)), 0, uintptr(unsafe.Pointer(&b.fieldVectorCB))); mdHRESULTFailed(hr) || b.fieldVectorCB == 0 {
		b.fieldVectorErr = fmt.Sprintf("field-vector constant buffer failed HRESULT=%#x", uint32(hr))
		mdCOMRelease(&b.fieldVectorCS)
		logf("MULTIDIM_GPU shader name=field-vector target=cs_5_0 ok=false err=%q", b.fieldVectorErr)
		return
	}
	b.fieldVectorReady = true
	logf("MULTIDIM_GPU shader name=field-vector target=cs_5_0 ok=true bytes=%d compile=%s create=%s total=%s", len(code), compiled.Sub(started), time.Since(compiled), time.Since(started))
}

func (b *mdD3D11Backend) releaseFieldVectorResources() {
	mdCOMRelease(&b.fieldVectorValuesStaging)
	mdCOMRelease(&b.fieldVectorValuesUAV)
	mdCOMRelease(&b.fieldVectorValuesBuffer)
	mdCOMRelease(&b.fieldVectorWiresSRV)
	mdCOMRelease(&b.fieldVectorWiresBuffer)
	mdCOMRelease(&b.fieldVectorSegmentsSRV)
	mdCOMRelease(&b.fieldVectorSegmentsBuffer)
	b.fieldVectorSegmentsCap = 0
	b.fieldVectorWiresCap = 0
	b.fieldVectorValuesCap = 0
}

func (b *mdD3D11Backend) releaseFieldVectorPipeline() {
	b.releaseFieldVectorResources()
	mdCOMRelease(&b.fieldVectorCB)
	mdCOMRelease(&b.fieldVectorCS)
	b.fieldVectorReady = false
}

func (b *mdD3D11Backend) releaseFieldResources() {
	mdCOMRelease(&b.fieldValuesStaging)
	mdCOMRelease(&b.fieldValuesUAV)
	mdCOMRelease(&b.fieldValuesBuffer)
	mdCOMRelease(&b.fieldSamplesSRV)
	mdCOMRelease(&b.fieldSamplesBuffer)
	b.fieldSamplesCap = 0
	b.fieldValuesCap = 0
}

func (b *mdD3D11Backend) releaseFieldPipeline() {
	b.releaseFieldVectorPipeline()
	b.releaseFieldResources()
	mdCOMRelease(&b.fieldBlendState)
	mdCOMRelease(&b.fieldCB)
	mdCOMRelease(&b.fieldCS)
	b.fieldSurfaceReady = false
	b.fieldReady = false
}

func (b *mdD3D11Backend) releaseOcclusionResources() {
	mdCOMRelease(&b.occlusionCountsStaging)
	mdCOMRelease(&b.occlusionCountsUAV)
	mdCOMRelease(&b.occlusionCountsBuffer)
	mdCOMRelease(&b.occlusionPairsSRV)
	mdCOMRelease(&b.occlusionPairsBuffer)
	mdCOMRelease(&b.occlusionQueriesSRV)
	mdCOMRelease(&b.occlusionQueriesBuffer)
	mdCOMRelease(&b.occlusionFacesSRV)
	mdCOMRelease(&b.occlusionFacesBuffer)
	b.occlusionFacesCap = 0
	b.occlusionQueriesCap = 0
	b.occlusionPairsCap = 0
	b.occlusionCountsCap = 0
}

func (b *mdD3D11Backend) releaseOcclusionPipeline() {
	b.releaseOcclusionResources()
	mdCOMRelease(&b.occlusionCS)
	b.occlusionReady = false
}

func mdD3D11NextCapacity(needed int) int {
	capElements := 64
	for capElements < needed {
		capElements *= 2
	}
	return capElements
}

func (b *mdD3D11Backend) ensureStructuredInput(buffer, srv *uintptr, capacity *int, needed, stride int, label string) error {
	if needed <= 0 || stride <= 0 {
		return fmt.Errorf("%s structured input size invalid", label)
	}
	if *buffer != 0 && *srv != 0 && *capacity >= needed {
		return nil
	}
	mdCOMRelease(srv)
	mdCOMRelease(buffer)
	capElements := mdD3D11NextCapacity(needed)
	desc := mdD3D11BufferDesc{
		ByteWidth: uint32(capElements * stride), Usage: mdD3D11UsageDynamic,
		BindFlags: mdD3D11BindShaderResource, CPUAccessFlags: mdD3D11CPUAccessWrite,
		MiscFlags: mdD3D11ResourceMiscBufferStructured, StructureByteStride: uint32(stride),
	}
	if hr := mdCOMCall(b.device, 3, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(buffer))); mdHRESULTFailed(hr) || *buffer == 0 {
		return fmt.Errorf("CreateBuffer(%s) HRESULT=%#x", label, uint32(hr))
	}
	view := mdD3D11ShaderResourceViewDesc{Format: mdDXGIFormatUnknown, ViewDimension: mdD3D11SRVDimensionBuffer, NumElements: uint32(capElements)}
	if hr := mdCOMCall(b.device, 7, *buffer, uintptr(unsafe.Pointer(&view)), uintptr(unsafe.Pointer(srv))); mdHRESULTFailed(hr) || *srv == 0 {
		mdCOMRelease(buffer)
		return fmt.Errorf("CreateShaderResourceView(%s) HRESULT=%#x", label, uint32(hr))
	}
	*capacity = capElements
	logf("MULTIDIM_GPU resize pipeline=occlusion resource=%s elements=%d stride=%d", label, capElements, stride)
	return nil
}

func (b *mdD3D11Backend) ensureOcclusionCounts(needed int) error {
	if needed <= 0 {
		return fmt.Errorf("occlusion output size invalid")
	}
	if b.occlusionCountsBuffer != 0 && b.occlusionCountsUAV != 0 && b.occlusionCountsStaging != 0 && b.occlusionCountsCap >= needed {
		return nil
	}
	mdCOMRelease(&b.occlusionCountsStaging)
	mdCOMRelease(&b.occlusionCountsUAV)
	mdCOMRelease(&b.occlusionCountsBuffer)
	capElements := mdD3D11NextCapacity(needed)
	stride := int(unsafe.Sizeof(uint32(0)))
	desc := mdD3D11BufferDesc{
		ByteWidth: uint32(capElements * stride), Usage: mdD3D11UsageDefault,
		BindFlags: mdD3D11BindUnorderedAccess,
		MiscFlags: mdD3D11ResourceMiscBufferStructured, StructureByteStride: uint32(stride),
	}
	if hr := mdCOMCall(b.device, 3, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&b.occlusionCountsBuffer))); mdHRESULTFailed(hr) || b.occlusionCountsBuffer == 0 {
		return fmt.Errorf("CreateBuffer(occlusion-counts) HRESULT=%#x", uint32(hr))
	}
	view := mdD3D11UnorderedAccessViewDesc{Format: mdDXGIFormatUnknown, ViewDimension: mdD3D11UAVDimensionBuffer, NumElements: uint32(capElements)}
	if hr := mdCOMCall(b.device, 8, b.occlusionCountsBuffer, uintptr(unsafe.Pointer(&view)), uintptr(unsafe.Pointer(&b.occlusionCountsUAV))); mdHRESULTFailed(hr) || b.occlusionCountsUAV == 0 {
		mdCOMRelease(&b.occlusionCountsBuffer)
		return fmt.Errorf("CreateUnorderedAccessView(occlusion-counts) HRESULT=%#x", uint32(hr))
	}
	stagingDesc := desc
	stagingDesc.Usage = mdD3D11UsageStaging
	stagingDesc.BindFlags = 0
	stagingDesc.CPUAccessFlags = mdD3D11CPUAccessRead
	stagingDesc.MiscFlags = 0
	stagingDesc.StructureByteStride = 0
	if hr := mdCOMCall(b.device, 3, uintptr(unsafe.Pointer(&stagingDesc)), 0, uintptr(unsafe.Pointer(&b.occlusionCountsStaging))); mdHRESULTFailed(hr) || b.occlusionCountsStaging == 0 {
		mdCOMRelease(&b.occlusionCountsUAV)
		mdCOMRelease(&b.occlusionCountsBuffer)
		return fmt.Errorf("CreateBuffer(occlusion-staging) HRESULT=%#x", uint32(hr))
	}
	b.occlusionCountsCap = capElements
	logf("MULTIDIM_GPU resize pipeline=occlusion resource=counts elements=%d stride=%d", capElements, stride)
	return nil
}

func mdD3D11MapStructured[T any](b *mdD3D11Backend, buffer uintptr, values []T) error {
	if b == nil || buffer == 0 || len(values) == 0 {
		return fmt.Errorf("structured upload is empty")
	}
	var mapped mdD3D11MappedSubresource
	if hr := mdCOMCall(b.context, 14, buffer, 0, mdD3D11MapWriteDiscard, 0, uintptr(unsafe.Pointer(&mapped))); mdHRESULTFailed(hr) || mapped.Data == 0 {
		return fmt.Errorf("structured Map HRESULT=%#x", uint32(hr))
	}
	byteCount := len(values) * int(unsafe.Sizeof(values[0]))
	dst := unsafe.Slice((*byte)(unsafe.Pointer(mapped.Data)), byteCount)
	src := unsafe.Slice((*byte)(unsafe.Pointer(&values[0])), byteCount)
	copy(dst, src)
	mdCOMCall(b.context, 15, buffer, 0)
	runtime.KeepAlive(values)
	return nil
}

func mdD3D11RenderOcclusionCounts(label string, faces []mdD3D11OcclusionFace, queries []mdD3D11OcclusionQuery, pairs []mdD3D11OcclusionPair) ([]int, bool) {
	if len(faces) == 0 || len(queries) == 0 || len(pairs) == 0 {
		return nil, false
	}
	started := time.Now()
	b := mdD3D11Get()
	if b == nil || !b.deviceReady || !b.occlusionReady {
		mdD3D11LogFallbackOnce("occlusion", mdD3D11PipelineReason("occlusion"))
		return nil, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	faceStride := int(unsafe.Sizeof(mdD3D11OcclusionFace{}))
	queryStride := int(unsafe.Sizeof(mdD3D11OcclusionQuery{}))
	pairStride := int(unsafe.Sizeof(mdD3D11OcclusionPair{}))
	if err := b.ensureStructuredInput(&b.occlusionFacesBuffer, &b.occlusionFacesSRV, &b.occlusionFacesCap, len(faces), faceStride, "faces"); err != nil {
		mdD3D11LogFallbackOnce("occlusion-faces", err.Error())
		return nil, false
	}
	if err := b.ensureStructuredInput(&b.occlusionQueriesBuffer, &b.occlusionQueriesSRV, &b.occlusionQueriesCap, len(queries), queryStride, "queries"); err != nil {
		mdD3D11LogFallbackOnce("occlusion-queries", err.Error())
		return nil, false
	}
	paddedCount := (len(pairs) + 63) &^ 63
	paddedPairs := make([]mdD3D11OcclusionPair, paddedCount)
	copy(paddedPairs, pairs)
	for i := len(pairs); i < len(paddedPairs); i++ {
		paddedPairs[i].QueryIndex = ^uint32(0)
	}
	if err := b.ensureStructuredInput(&b.occlusionPairsBuffer, &b.occlusionPairsSRV, &b.occlusionPairsCap, len(paddedPairs), pairStride, "pairs"); err != nil {
		mdD3D11LogFallbackOnce("occlusion-pairs", err.Error())
		return nil, false
	}
	if err := b.ensureOcclusionCounts(len(queries)); err != nil {
		mdD3D11LogFallbackOnce("occlusion-counts", err.Error())
		return nil, false
	}
	if err := mdD3D11MapStructured(b, b.occlusionFacesBuffer, faces); err != nil {
		mdD3D11LogFallbackOnce("occlusion-upload-faces", err.Error())
		return nil, false
	}
	if err := mdD3D11MapStructured(b, b.occlusionQueriesBuffer, queries); err != nil {
		mdD3D11LogFallbackOnce("occlusion-upload-queries", err.Error())
		return nil, false
	}
	if err := mdD3D11MapStructured(b, b.occlusionPairsBuffer, paddedPairs); err != nil {
		mdD3D11LogFallbackOnce("occlusion-upload-pairs", err.Error())
		return nil, false
	}
	clear := [4]uint32{}
	mdCOMCall(b.context, 51, b.occlusionCountsUAV, uintptr(unsafe.Pointer(&clear[0])))
	srvs := [3]uintptr{b.occlusionFacesSRV, b.occlusionQueriesSRV, b.occlusionPairsSRV}
	uav := b.occlusionCountsUAV
	mdCOMCall(b.context, 67, 0, uintptr(len(srvs)), uintptr(unsafe.Pointer(&srvs[0])))
	mdCOMCall(b.context, 69, b.occlusionCS, 0, 0)
	mdCOMCall(b.context, 68, 0, 1, uintptr(unsafe.Pointer(&uav)), 0)
	prepareDone := time.Now()
	dispatchX := paddedCount / 64
	mdCOMCall(b.context, 41, uintptr(dispatchX), 1, 1)
	var nilUAV uintptr
	mdCOMCall(b.context, 68, 0, 1, uintptr(unsafe.Pointer(&nilUAV)), 0)
	var nilSRVs [3]uintptr
	mdCOMCall(b.context, 67, 0, uintptr(len(nilSRVs)), uintptr(unsafe.Pointer(&nilSRVs[0])))
	mdCOMCall(b.context, 47, b.occlusionCountsStaging, b.occlusionCountsBuffer)
	var mapped mdD3D11MappedSubresource
	if hr := mdCOMCall(b.context, 14, b.occlusionCountsStaging, 0, mdD3D11MapRead, 0, uintptr(unsafe.Pointer(&mapped))); mdHRESULTFailed(hr) || mapped.Data == 0 {
		mdD3D11LogFallbackOnce("occlusion-readback", fmt.Sprintf("staging Map HRESULT=%#x", uint32(hr)))
		return nil, false
	}
	mapDone := time.Now()
	raw := unsafe.Slice((*uint32)(unsafe.Pointer(mapped.Data)), len(queries))
	out := make([]int, len(queries))
	for i, n := range raw {
		out[i] = int(n)
	}
	mdCOMCall(b.context, 15, b.occlusionCountsStaging, 0)
	finished := time.Now()
	logf("MULTIDIM_RENDER shape=%s backend=D3D11 queries=%d pairs=%d dispatch=%d prepare=%s gpu_wait_readback=%s convert=%s total=%s fallback=false", label, len(queries), len(pairs), dispatchX, prepareDone.Sub(started), mapDone.Sub(prepareDone), finished.Sub(mapDone), finished.Sub(started))
	return out, true
}

func (b *mdD3D11Backend) ensureFieldValues(needed int) error {
	if needed <= 0 {
		return fmt.Errorf("field-potential output size invalid")
	}
	if b.fieldValuesBuffer != 0 && b.fieldValuesUAV != 0 && b.fieldValuesStaging != 0 && b.fieldValuesCap >= needed {
		return nil
	}
	mdCOMRelease(&b.fieldValuesStaging)
	mdCOMRelease(&b.fieldValuesUAV)
	mdCOMRelease(&b.fieldValuesBuffer)
	capElements := mdD3D11NextCapacity(needed)
	stride := int(unsafe.Sizeof(float32(0)))
	desc := mdD3D11BufferDesc{
		ByteWidth: uint32(capElements * stride), Usage: mdD3D11UsageDefault,
		BindFlags: mdD3D11BindUnorderedAccess,
		MiscFlags: mdD3D11ResourceMiscBufferStructured, StructureByteStride: uint32(stride),
	}
	if hr := mdCOMCall(b.device, 3, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&b.fieldValuesBuffer))); mdHRESULTFailed(hr) || b.fieldValuesBuffer == 0 {
		return fmt.Errorf("CreateBuffer(field-potential-values) HRESULT=%#x", uint32(hr))
	}
	view := mdD3D11UnorderedAccessViewDesc{Format: mdDXGIFormatUnknown, ViewDimension: mdD3D11UAVDimensionBuffer, NumElements: uint32(capElements)}
	if hr := mdCOMCall(b.device, 8, b.fieldValuesBuffer, uintptr(unsafe.Pointer(&view)), uintptr(unsafe.Pointer(&b.fieldValuesUAV))); mdHRESULTFailed(hr) || b.fieldValuesUAV == 0 {
		mdCOMRelease(&b.fieldValuesBuffer)
		return fmt.Errorf("CreateUnorderedAccessView(field-potential-values) HRESULT=%#x", uint32(hr))
	}
	stagingDesc := desc
	stagingDesc.Usage = mdD3D11UsageStaging
	stagingDesc.BindFlags = 0
	stagingDesc.CPUAccessFlags = mdD3D11CPUAccessRead
	stagingDesc.MiscFlags = 0
	stagingDesc.StructureByteStride = 0
	if hr := mdCOMCall(b.device, 3, uintptr(unsafe.Pointer(&stagingDesc)), 0, uintptr(unsafe.Pointer(&b.fieldValuesStaging))); mdHRESULTFailed(hr) || b.fieldValuesStaging == 0 {
		mdCOMRelease(&b.fieldValuesUAV)
		mdCOMRelease(&b.fieldValuesBuffer)
		return fmt.Errorf("CreateBuffer(field-potential-staging) HRESULT=%#x", uint32(hr))
	}
	b.fieldValuesCap = capElements
	logf("MULTIDIM_GPU resize pipeline=field-potential resource=values elements=%d stride=%d", capElements, stride)
	return nil
}

func mdD3D11RenderFieldPotentialGrid(samples []fieldChargeSample, steps int, minCoord, maxCoord float64) ([]float64, bool) {
	if len(samples) == 0 || steps < 2 || !(maxCoord > minCoord) {
		return nil, false
	}
	total := steps * steps * steps
	if total <= 0 || total > 1<<24 {
		return nil, false
	}
	started := time.Now()
	b := mdD3D11Get()
	if b == nil || !b.deviceReady || !b.fieldReady {
		mdD3D11LogFallbackOnce("field", mdD3D11PipelineReason("field"))
		return nil, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	gpuSamples := make([]mdD3D11FieldSample, len(samples))
	for i, s := range samples {
		gateEnabled := float32(0)
		if s.Gate {
			gateEnabled = 1
		}
		gpuSamples[i] = mdD3D11FieldSample{
			X: float32(s.Pos.X), Y: float32(s.Pos.Y), Z: float32(s.Pos.Z), Soft: float32(s.Soft),
			Q: float32(s.Q), GateOffset: float32(s.GateOffset), GateX: float32(s.GateNormal.X), GateY: float32(s.GateNormal.Y),
			GateZ: float32(s.GateNormal.Z), GateEnabled: gateEnabled,
		}
	}
	sampleStride := int(unsafe.Sizeof(mdD3D11FieldSample{}))
	if err := b.ensureStructuredInput(&b.fieldSamplesBuffer, &b.fieldSamplesSRV, &b.fieldSamplesCap, len(gpuSamples), sampleStride, "field-samples"); err != nil {
		mdD3D11LogFallbackOnce("field-samples", err.Error())
		return nil, false
	}
	if err := b.ensureFieldValues(total); err != nil {
		mdD3D11LogFallbackOnce("field-values", err.Error())
		return nil, false
	}
	if err := mdD3D11MapStructured(b, b.fieldSamplesBuffer, gpuSamples); err != nil {
		mdD3D11LogFallbackOnce("field-upload", err.Error())
		return nil, false
	}
	constants := mdD3D11FieldConstants{
		Steps: uint32(steps), SampleCount: uint32(len(gpuSamples)), TotalCount: uint32(total),
		MinCoord: float32(minCoord), StepSize: float32((maxCoord - minCoord) / float64(steps-1)), SoftFactor: .12,
	}
	var mapped mdD3D11MappedSubresource
	if hr := mdCOMCall(b.context, 14, b.fieldCB, 0, mdD3D11MapWriteDiscard, 0, uintptr(unsafe.Pointer(&mapped))); mdHRESULTFailed(hr) || mapped.Data == 0 {
		mdD3D11LogFallbackOnce("field-constants", fmt.Sprintf("constant Map HRESULT=%#x", uint32(hr)))
		return nil, false
	}
	*(*mdD3D11FieldConstants)(unsafe.Pointer(mapped.Data)) = constants
	mdCOMCall(b.context, 15, b.fieldCB, 0)

	srv := b.fieldSamplesSRV
	uav := b.fieldValuesUAV
	cb := b.fieldCB
	mdCOMCall(b.context, 67, 0, 1, uintptr(unsafe.Pointer(&srv)))
	mdCOMCall(b.context, 71, 0, 1, uintptr(unsafe.Pointer(&cb)))
	mdCOMCall(b.context, 69, b.fieldCS, 0, 0)
	mdCOMCall(b.context, 68, 0, 1, uintptr(unsafe.Pointer(&uav)), 0)
	prepareDone := time.Now()
	dispatchX := (total + 63) / 64
	mdCOMCall(b.context, 41, uintptr(dispatchX), 1, 1)
	var nilUAV, nilSRV, nilCB uintptr
	mdCOMCall(b.context, 68, 0, 1, uintptr(unsafe.Pointer(&nilUAV)), 0)
	mdCOMCall(b.context, 67, 0, 1, uintptr(unsafe.Pointer(&nilSRV)))
	mdCOMCall(b.context, 71, 0, 1, uintptr(unsafe.Pointer(&nilCB)))
	mdCOMCall(b.context, 47, b.fieldValuesStaging, b.fieldValuesBuffer)
	if hr := mdCOMCall(b.context, 14, b.fieldValuesStaging, 0, mdD3D11MapRead, 0, uintptr(unsafe.Pointer(&mapped))); mdHRESULTFailed(hr) || mapped.Data == 0 {
		mdD3D11LogFallbackOnce("field-readback", fmt.Sprintf("staging Map HRESULT=%#x", uint32(hr)))
		return nil, false
	}
	mapDone := time.Now()
	raw := unsafe.Slice((*float32)(unsafe.Pointer(mapped.Data)), total)
	out := make([]float64, total)
	for i, v := range raw {
		out[i] = float64(v)
	}
	mdCOMCall(b.context, 15, b.fieldValuesStaging, 0)
	finished := time.Now()
	logf("FIELD_GPU potential backend=D3D11 grid=%dx%dx%d points=%d samples=%d dispatch=%d upload=%s gpu_wait_readback=%s convert=%s total=%s fallback=false", steps, steps, steps, total, len(samples), dispatchX, prepareDone.Sub(started), mapDone.Sub(prepareDone), finished.Sub(mapDone), finished.Sub(started))
	return out, true
}

func (b *mdD3D11Backend) ensureFieldVectorValues(needed int) error {
	if needed <= 0 {
		return fmt.Errorf("field-vector output size invalid")
	}
	if b.fieldVectorValuesBuffer != 0 && b.fieldVectorValuesUAV != 0 && b.fieldVectorValuesStaging != 0 && b.fieldVectorValuesCap >= needed {
		return nil
	}
	mdCOMRelease(&b.fieldVectorValuesStaging)
	mdCOMRelease(&b.fieldVectorValuesUAV)
	mdCOMRelease(&b.fieldVectorValuesBuffer)
	capElements := mdD3D11NextCapacity(needed)
	stride := int(unsafe.Sizeof(mdD3D11FieldVectorValue{}))
	desc := mdD3D11BufferDesc{
		ByteWidth: uint32(capElements * stride), Usage: mdD3D11UsageDefault,
		BindFlags: mdD3D11BindUnorderedAccess,
		MiscFlags: mdD3D11ResourceMiscBufferStructured, StructureByteStride: uint32(stride),
	}
	if hr := mdCOMCall(b.device, 3, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&b.fieldVectorValuesBuffer))); mdHRESULTFailed(hr) || b.fieldVectorValuesBuffer == 0 {
		return fmt.Errorf("CreateBuffer(field-vector-values) HRESULT=%#x", uint32(hr))
	}
	view := mdD3D11UnorderedAccessViewDesc{Format: mdDXGIFormatUnknown, ViewDimension: mdD3D11UAVDimensionBuffer, NumElements: uint32(capElements)}
	if hr := mdCOMCall(b.device, 8, b.fieldVectorValuesBuffer, uintptr(unsafe.Pointer(&view)), uintptr(unsafe.Pointer(&b.fieldVectorValuesUAV))); mdHRESULTFailed(hr) || b.fieldVectorValuesUAV == 0 {
		mdCOMRelease(&b.fieldVectorValuesBuffer)
		return fmt.Errorf("CreateUnorderedAccessView(field-vector-values) HRESULT=%#x", uint32(hr))
	}
	stagingDesc := desc
	stagingDesc.Usage = mdD3D11UsageStaging
	stagingDesc.BindFlags = 0
	stagingDesc.CPUAccessFlags = mdD3D11CPUAccessRead
	stagingDesc.MiscFlags = 0
	stagingDesc.StructureByteStride = 0
	if hr := mdCOMCall(b.device, 3, uintptr(unsafe.Pointer(&stagingDesc)), 0, uintptr(unsafe.Pointer(&b.fieldVectorValuesStaging))); mdHRESULTFailed(hr) || b.fieldVectorValuesStaging == 0 {
		mdCOMRelease(&b.fieldVectorValuesUAV)
		mdCOMRelease(&b.fieldVectorValuesBuffer)
		return fmt.Errorf("CreateBuffer(field-vector-staging) HRESULT=%#x", uint32(hr))
	}
	b.fieldVectorValuesCap = capElements
	logf("MULTIDIM_GPU resize pipeline=field-vector resource=values elements=%d stride=%d", capElements, stride)
	return nil
}

func mdD3D11RenderFieldVectorGrid(samples []fieldChargeSample, model fieldMagneticModel, magnetic bool, steps int, minCoord, maxCoord float64) ([]fieldVec3, bool) {
	if steps < 2 || !(maxCoord > minCoord) {
		return nil, false
	}
	segmentCount := 0
	for _, magnet := range model.Magnets {
		segmentCount += len(magnet.Segments)
	}
	segmentCount += len(model.FiniteCurrents)
	if magnetic {
		if segmentCount == 0 && len(model.Wires) == 0 {
			return nil, false
		}
	} else if len(samples) == 0 {
		return nil, false
	}
	total := steps * steps * steps
	if total <= 0 || total > 1<<24 {
		return nil, false
	}
	started := time.Now()
	b := mdD3D11Get()
	if b == nil || !b.deviceReady || !b.fieldVectorReady {
		mdD3D11LogFallbackOnce("field-vector", mdD3D11PipelineReason("field-vector"))
		return nil, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	gpuSamples := make([]mdD3D11FieldSample, max(1, len(samples)))
	for i, s := range samples {
		gateEnabled := float32(0)
		if s.Gate {
			gateEnabled = 1
		}
		gpuSamples[i] = mdD3D11FieldSample{
			X: float32(s.Pos.X), Y: float32(s.Pos.Y), Z: float32(s.Pos.Z), Soft: float32(s.Soft),
			Q: float32(s.Q), GateOffset: float32(s.GateOffset), GateX: float32(s.GateNormal.X), GateY: float32(s.GateNormal.Y),
			GateZ: float32(s.GateNormal.Z), GateEnabled: gateEnabled,
		}
	}
	gpuSegments := make([]mdD3D11FieldVectorSegment, 0, max(1, segmentCount))
	appendSegment := func(s fieldMagneticCurrentSegment) {
		gpuSegments = append(gpuSegments, mdD3D11FieldVectorSegment{
			MidX: float32(s.Mid.X), MidY: float32(s.Mid.Y), MidZ: float32(s.Mid.Z), Soft: float32(s.Soft),
			DLX: float32(s.DL.X), DLY: float32(s.DL.Y), DLZ: float32(s.DL.Z), Current: float32(s.I),
		})
	}
	for _, magnet := range model.Magnets {
		for _, segment := range magnet.Segments {
			appendSegment(segment)
		}
	}
	for _, segment := range model.FiniteCurrents {
		appendSegment(segment)
	}
	if len(gpuSegments) == 0 {
		gpuSegments = append(gpuSegments, mdD3D11FieldVectorSegment{})
	}
	gpuWires := make([]mdD3D11FieldVectorWire, 0, max(1, len(model.Wires)))
	for _, wire := range model.Wires {
		center := fieldObjectCenter(wire)
		_, axis, _ := fieldObjectBasis(wire)
		gpuWires = append(gpuWires, mdD3D11FieldVectorWire{
			CenterX: float32(center.X), CenterY: float32(center.Y), CenterZ: float32(center.Z),
			AxisX: float32(axis.X), AxisY: float32(axis.Y), AxisZ: float32(axis.Z), Current: float32(wire.Charge),
		})
	}
	if len(gpuWires) == 0 {
		gpuWires = append(gpuWires, mdD3D11FieldVectorWire{})
	}

	if err := b.ensureStructuredInput(&b.fieldSamplesBuffer, &b.fieldSamplesSRV, &b.fieldSamplesCap, len(gpuSamples), int(unsafe.Sizeof(mdD3D11FieldSample{})), "field-vector-charges"); err != nil {
		mdD3D11LogFallbackOnce("field-vector-charges", err.Error())
		return nil, false
	}
	if err := b.ensureStructuredInput(&b.fieldVectorSegmentsBuffer, &b.fieldVectorSegmentsSRV, &b.fieldVectorSegmentsCap, len(gpuSegments), int(unsafe.Sizeof(mdD3D11FieldVectorSegment{})), "field-vector-segments"); err != nil {
		mdD3D11LogFallbackOnce("field-vector-segments", err.Error())
		return nil, false
	}
	if err := b.ensureStructuredInput(&b.fieldVectorWiresBuffer, &b.fieldVectorWiresSRV, &b.fieldVectorWiresCap, len(gpuWires), int(unsafe.Sizeof(mdD3D11FieldVectorWire{})), "field-vector-wires"); err != nil {
		mdD3D11LogFallbackOnce("field-vector-wires", err.Error())
		return nil, false
	}
	if err := b.ensureFieldVectorValues(total); err != nil {
		mdD3D11LogFallbackOnce("field-vector-values", err.Error())
		return nil, false
	}
	if err := mdD3D11MapStructured(b, b.fieldSamplesBuffer, gpuSamples); err != nil {
		mdD3D11LogFallbackOnce("field-vector-upload-charges", err.Error())
		return nil, false
	}
	if err := mdD3D11MapStructured(b, b.fieldVectorSegmentsBuffer, gpuSegments); err != nil {
		mdD3D11LogFallbackOnce("field-vector-upload-segments", err.Error())
		return nil, false
	}
	if err := mdD3D11MapStructured(b, b.fieldVectorWiresBuffer, gpuWires); err != nil {
		mdD3D11LogFallbackOnce("field-vector-upload-wires", err.Error())
		return nil, false
	}

	mode := uint32(0)
	if magnetic {
		mode = 1
	}
	constants := mdD3D11FieldVectorConstants{
		Steps: uint32(steps), ChargeCount: uint32(len(samples)), SegmentCount: uint32(segmentCount), WireCount: uint32(len(model.Wires)),
		TotalCount: uint32(total), Mode: mode,
		MinCoord: float32(minCoord), StepSize: float32((maxCoord - minCoord) / float64(steps-1)), SoftFactor: .12,
	}
	var mapped mdD3D11MappedSubresource
	if hr := mdCOMCall(b.context, 14, b.fieldVectorCB, 0, mdD3D11MapWriteDiscard, 0, uintptr(unsafe.Pointer(&mapped))); mdHRESULTFailed(hr) || mapped.Data == 0 {
		mdD3D11LogFallbackOnce("field-vector-constants", fmt.Sprintf("constant Map HRESULT=%#x", uint32(hr)))
		return nil, false
	}
	*(*mdD3D11FieldVectorConstants)(unsafe.Pointer(mapped.Data)) = constants
	mdCOMCall(b.context, 15, b.fieldVectorCB, 0)

	srvs := [3]uintptr{b.fieldSamplesSRV, b.fieldVectorSegmentsSRV, b.fieldVectorWiresSRV}
	uav := b.fieldVectorValuesUAV
	cb := b.fieldVectorCB
	mdCOMCall(b.context, 67, 0, uintptr(len(srvs)), uintptr(unsafe.Pointer(&srvs[0])))
	mdCOMCall(b.context, 71, 0, 1, uintptr(unsafe.Pointer(&cb)))
	mdCOMCall(b.context, 69, b.fieldVectorCS, 0, 0)
	mdCOMCall(b.context, 68, 0, 1, uintptr(unsafe.Pointer(&uav)), 0)
	prepareDone := time.Now()
	dispatchX := (total + 63) / 64
	mdCOMCall(b.context, 41, uintptr(dispatchX), 1, 1)
	var nilUAV, nilCB uintptr
	var nilSRVs [3]uintptr
	mdCOMCall(b.context, 68, 0, 1, uintptr(unsafe.Pointer(&nilUAV)), 0)
	mdCOMCall(b.context, 67, 0, uintptr(len(nilSRVs)), uintptr(unsafe.Pointer(&nilSRVs[0])))
	mdCOMCall(b.context, 71, 0, 1, uintptr(unsafe.Pointer(&nilCB)))
	mdCOMCall(b.context, 47, b.fieldVectorValuesStaging, b.fieldVectorValuesBuffer)
	if hr := mdCOMCall(b.context, 14, b.fieldVectorValuesStaging, 0, mdD3D11MapRead, 0, uintptr(unsafe.Pointer(&mapped))); mdHRESULTFailed(hr) || mapped.Data == 0 {
		mdD3D11LogFallbackOnce("field-vector-readback", fmt.Sprintf("staging Map HRESULT=%#x", uint32(hr)))
		return nil, false
	}
	mapDone := time.Now()
	raw := unsafe.Slice((*mdD3D11FieldVectorValue)(unsafe.Pointer(mapped.Data)), total)
	out := make([]fieldVec3, total)
	for i, v := range raw {
		out[i] = fieldVec3{X: float64(v.X), Y: float64(v.Y), Z: float64(v.Z)}
	}
	mdCOMCall(b.context, 15, b.fieldVectorValuesStaging, 0)
	finished := time.Now()
	kind := "electric"
	if magnetic {
		kind = "magnetic"
	}
	logf("FIELD_GPU %s-vector backend=D3D11 grid=%dx%dx%d points=%d charges=%d segments=%d wires=%d dispatch=%d upload=%s gpu_wait_readback=%s convert=%s total=%s fallback=false", kind, steps, steps, steps, total, len(samples), segmentCount, len(model.Wires), dispatchX, prepareDone.Sub(started), mapDone.Sub(prepareDone), finished.Sub(mapDone), finished.Sub(started))
	return out, true
}

func mdD3D11RenderFieldMembraneTriangles(w, h int, vertices []mdD3D11FaceVertex) ([]byte, bool) {
	if w <= 0 || h <= 0 || len(vertices) < 3 {
		return nil, false
	}
	started := time.Now()
	b := mdD3D11Get()
	if b == nil || !b.deviceReady || !b.faceReady || !b.fieldSurfaceReady {
		mdD3D11LogFallbackOnce("field-surface", mdD3D11PipelineReason("field-surface"))
		return nil, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	byteCount := len(vertices) * int(unsafe.Sizeof(mdD3D11FaceVertex{}))
	if err := b.ensureFaceResources(w, h, byteCount); err != nil {
		mdD3D11LogFallbackOnce("field-surface-output", err.Error())
		return nil, false
	}
	var mapped mdD3D11MappedSubresource
	if hr := mdCOMCall(b.context, 14, b.faceVB, 0, mdD3D11MapWriteDiscard, 0, uintptr(unsafe.Pointer(&mapped))); mdHRESULTFailed(hr) || mapped.Data == 0 {
		mdD3D11LogFallbackOnce("field-surface-map", fmt.Sprintf("vertex Map HRESULT=%#x", uint32(hr)))
		return nil, false
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(mapped.Data)), byteCount)
	src := unsafe.Slice((*byte)(unsafe.Pointer(&vertices[0])), byteCount)
	copy(dst, src)
	mdCOMCall(b.context, 15, b.faceVB, 0)
	runtime.KeepAlive(vertices)

	clear := [4]float32{0, 0, 0, 0}
	mdCOMCall(b.context, 33, 1, uintptr(unsafe.Pointer(&b.faceRTV)), 0)
	mdCOMCall(b.context, 50, b.faceRTV, uintptr(unsafe.Pointer(&clear[0])))
	viewport := mdD3D11Viewport{Width: float32(w), Height: float32(h), MinDepth: 0, MaxDepth: 1}
	mdCOMCall(b.context, 44, 1, uintptr(unsafe.Pointer(&viewport)))
	mdCOMCall(b.context, 43, b.faceRasterState)
	mdCOMCall(b.context, 17, b.faceLayout)
	vb := b.faceVB
	stride := uint32(unsafe.Sizeof(mdD3D11FaceVertex{}))
	offset := uint32(0)
	mdCOMCall(b.context, 18, 0, 1, uintptr(unsafe.Pointer(&vb)), uintptr(unsafe.Pointer(&stride)), uintptr(unsafe.Pointer(&offset)))
	mdCOMCall(b.context, 24, mdD3D11PrimitiveTriangleList)
	mdCOMCall(b.context, 11, b.faceVS, 0, 0)
	mdCOMCall(b.context, 9, b.facePS, 0, 0)
	blendFactor := [4]float32{1, 1, 1, 1}
	mdCOMCall(b.context, 35, b.fieldBlendState, uintptr(unsafe.Pointer(&blendFactor[0])), uintptr(^uint32(0)))
	prepareDone := time.Now()
	mdCOMCall(b.context, 13, uintptr(len(vertices)), 0)
	mdCOMCall(b.context, 35, 0, uintptr(unsafe.Pointer(&blendFactor[0])), uintptr(^uint32(0)))
	var nilRTV uintptr
	mdCOMCall(b.context, 33, 0, uintptr(unsafe.Pointer(&nilRTV)), 0)
	mdCOMCall(b.context, 47, b.faceStaging, b.faceTexture)
	if hr := mdCOMCall(b.context, 14, b.faceStaging, 0, mdD3D11MapRead, 0, uintptr(unsafe.Pointer(&mapped))); mdHRESULTFailed(hr) || mapped.Data == 0 {
		mdD3D11LogFallbackOnce("field-surface-readback", fmt.Sprintf("staging Map HRESULT=%#x", uint32(hr)))
		return nil, false
	}
	mapDone := time.Now()
	gpu := unsafe.Slice((*byte)(unsafe.Pointer(mapped.Data)), int(mapped.RowPitch)*h)
	pix := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		srow := gpu[y*int(mapped.RowPitch):]
		drow := pix[y*w*4:]
		for x := 0; x < w; x++ {
			si, di := x*4, x*4
			drow[di+0] = srow[si+2]
			drow[di+1] = srow[si+1]
			drow[di+2] = srow[si+0]
			drow[di+3] = srow[si+3]
		}
	}
	mdCOMCall(b.context, 15, b.faceStaging, 0)
	finished := time.Now()
	logf("FIELD_GPU membranes backend=D3D11 size=%dx%d vertices=%d prepare=%s gpu_wait_readback=%s convert=%s total=%s fallback=false", w, h, len(vertices), prepareDone.Sub(started), mapDone.Sub(prepareDone), finished.Sub(mapDone), finished.Sub(started))
	return pix, true
}

func (b *mdD3D11Backend) releaseFacePipeline() {
	mdCOMRelease(&b.faceRasterState)
	mdCOMRelease(&b.faceLayout)
	mdCOMRelease(&b.facePS)
	mdCOMRelease(&b.faceVS)
	b.faceReady = false
}

func (b *mdD3D11Backend) close() {
	b.releaseFieldPipeline()
	b.releaseBlackHoleSizeResources()
	b.releaseCelestialOutputResources()
	b.releaseFaceSizeResources()
	for i := range b.celestialSRVs {
		mdCOMRelease(&b.celestialSRVs[i])
		mdCOMRelease(&b.celestialTextures[i])
	}
	b.celestialAssetsReady = false
	mdCOMRelease(&b.celestialSampler)
	mdCOMRelease(&b.celestialCB)
	mdCOMRelease(&b.celestialCS)
	b.releaseFacePipeline()
	b.releaseOcclusionPipeline()
	mdCOMRelease(&b.faceVB)
	mdCOMRelease(&b.blackHoleCB)
	mdCOMRelease(&b.blackHoleCS)
	mdCOMRelease(&b.context)
	mdCOMRelease(&b.device)
	b.deviceReady = false
	b.blackHoleReady = false
	b.celestialReady = false
}

func (b *mdD3D11Backend) releaseBlackHoleSizeResources() {
	mdCOMRelease(&b.blackHoleStaging)
	mdCOMRelease(&b.blackHoleUAV)
	mdCOMRelease(&b.blackHoleTexture)
	b.blackHoleW, b.blackHoleH = 0, 0
}

func (b *mdD3D11Backend) releaseCelestialOutputResources() {
	mdCOMRelease(&b.celestialOutputStaging)
	mdCOMRelease(&b.celestialOutputUAV)
	mdCOMRelease(&b.celestialOutputTexture)
	b.celestialOutputW, b.celestialOutputH = 0, 0
}

func (b *mdD3D11Backend) releaseFaceSizeResources() {
	mdCOMRelease(&b.faceStaging)
	mdCOMRelease(&b.faceDSV)
	mdCOMRelease(&b.faceDepthClear)
	mdCOMRelease(&b.faceDepth)
	mdCOMRelease(&b.faceRTV)
	mdCOMRelease(&b.faceTexture)
	b.faceW, b.faceH = 0, 0
}

func (b *mdD3D11Backend) createTexture(desc mdD3D11Texture2DDesc) (uintptr, error) {
	var tex uintptr
	hr := mdCOMCall(b.device, 5, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&tex)))
	if mdHRESULTFailed(hr) || tex == 0 {
		return 0, fmt.Errorf("CreateTexture2D(%dx%d format=%d) HRESULT=%#x", desc.Width, desc.Height, desc.Format, uint32(hr))
	}
	return tex, nil
}

func (b *mdD3D11Backend) createTextureWithData(desc mdD3D11Texture2DDesc, data mdD3D11SubresourceData) (uintptr, error) {
	var tex uintptr
	hr := mdCOMCall(b.device, 5, uintptr(unsafe.Pointer(&desc)), uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&tex)))
	if mdHRESULTFailed(hr) || tex == 0 {
		return 0, fmt.Errorf("CreateTexture2D-data(%dx%d format=%d) HRESULT=%#x", desc.Width, desc.Height, desc.Format, uint32(hr))
	}
	return tex, nil
}

func (b *mdD3D11Backend) createTextureSRV(tex mdEmbeddedTexture) (uintptr, uintptr, error) {
	if !tex.ok || tex.w <= 0 || tex.h <= 0 || len(tex.rgba) < tex.w*tex.h*4 {
		return 0, 0, fmt.Errorf("천체 텍스처 데이터가 비어 있습니다")
	}
	desc := mdD3D11Texture2DDesc{
		Width: uint32(tex.w), Height: uint32(tex.h), MipLevels: 1, ArraySize: 1,
		Format: mdDXGIFormatR8G8B8A8UNorm, SampleDesc: mdD3D11SampleDesc{Count: 1},
		Usage: mdD3D11UsageDefault, BindFlags: mdD3D11BindShaderResource,
	}
	data := mdD3D11SubresourceData{
		SysMem:      uintptr(unsafe.Pointer(&tex.rgba[0])),
		SysMemPitch: uint32(tex.w * 4), SysMemSlicePitch: uint32(tex.w * tex.h * 4),
	}
	texture, err := b.createTextureWithData(desc, data)
	runtime.KeepAlive(tex.rgba)
	if err != nil {
		return 0, 0, err
	}
	var srv uintptr
	if hr := mdCOMCall(b.device, 7, texture, 0, uintptr(unsafe.Pointer(&srv))); mdHRESULTFailed(hr) || srv == 0 {
		mdCOMRelease(&texture)
		return 0, 0, fmt.Errorf("CreateShaderResourceView HRESULT=%#x", uint32(hr))
	}
	return texture, srv, nil
}

func mdD3D11RingTexture() mdEmbeddedTexture {
	mdLoadCelestialAssets()
	const bins = 720
	tex := mdEmbeddedTexture{w: bins, h: 1, rgba: make([]byte, bins*4), ok: true}
	for i := 0; i < bins; i++ {
		rho := 1.28 + (2.12-1.28)*float64(i)/float64(bins-1)
		r, g, b := mdCelestialRingColor(rho)
		j := i * 4
		tex.rgba[j+0], tex.rgba[j+1], tex.rgba[j+2], tex.rgba[j+3] = r, g, b, 255
	}
	return tex
}

func (b *mdD3D11Backend) ensureCelestialPipeline() error {
	if b.celestialReady {
		return nil
	}
	if b.celestialAttempted {
		if b.celestialErr == "" {
			b.celestialErr = "celestial pipeline unavailable"
		}
		return fmt.Errorf("%s", b.celestialErr)
	}
	b.celestialAttempted = true
	started := time.Now()
	code, err := mdD3DCompileShader(mdD3D11CelestialShader, "CSMain", "cs_5_0")
	if err != nil {
		b.celestialErr = err.Error()
		logf("MULTIDIM_GPU shader name=celestial target=cs_5_0 ok=false elapsed=%s err=%q", time.Since(started), b.celestialErr)
		return err
	}
	compiled := time.Now()
	if hr := mdCOMCall(b.device, 18, uintptr(unsafe.Pointer(&code[0])), uintptr(len(code)), 0, uintptr(unsafe.Pointer(&b.celestialCS))); mdHRESULTFailed(hr) || b.celestialCS == 0 {
		b.celestialErr = fmt.Sprintf("CreateComputeShader(celestial) failed HRESULT=%#x", uint32(hr))
		logf("MULTIDIM_GPU shader name=celestial target=cs_5_0 ok=false compile=%s create=%s err=%q", compiled.Sub(started), time.Since(compiled), b.celestialErr)
		return fmt.Errorf("%s", b.celestialErr)
	}
	cbDesc := mdD3D11BufferDesc{
		ByteWidth: uint32(unsafe.Sizeof(mdD3D11CelestialConstants{})), Usage: mdD3D11UsageDynamic,
		BindFlags: mdD3D11BindConstantBuffer, CPUAccessFlags: mdD3D11CPUAccessWrite,
	}
	if hr := mdCOMCall(b.device, 3, uintptr(unsafe.Pointer(&cbDesc)), 0, uintptr(unsafe.Pointer(&b.celestialCB))); mdHRESULTFailed(hr) || b.celestialCB == 0 {
		b.celestialErr = fmt.Sprintf("celestial constant buffer failed HRESULT=%#x", uint32(hr))
		mdCOMRelease(&b.celestialCS)
		logf("MULTIDIM_GPU shader name=celestial target=cs_5_0 ok=false err=%q", b.celestialErr)
		return fmt.Errorf("%s", b.celestialErr)
	}
	samplerDesc := mdD3D11SamplerDesc{
		Filter:   mdD3D11FilterMinMagMipLinear,
		AddressU: mdD3D11TextureAddressClamp, AddressV: mdD3D11TextureAddressClamp, AddressW: mdD3D11TextureAddressClamp,
		MaxAnisotropy: 1, ComparisonFunc: mdD3D11ComparisonAlways, MinLOD: 0, MaxLOD: math.MaxFloat32,
	}
	if hr := mdCOMCall(b.device, 23, uintptr(unsafe.Pointer(&samplerDesc)), uintptr(unsafe.Pointer(&b.celestialSampler))); mdHRESULTFailed(hr) || b.celestialSampler == 0 {
		b.celestialErr = fmt.Sprintf("celestial sampler failed HRESULT=%#x", uint32(hr))
		mdCOMRelease(&b.celestialCB)
		mdCOMRelease(&b.celestialCS)
		logf("MULTIDIM_GPU shader name=celestial target=cs_5_0 ok=false err=%q", b.celestialErr)
		return fmt.Errorf("%s", b.celestialErr)
	}
	b.celestialReady = true
	logf("MULTIDIM_GPU shader name=celestial target=cs_5_0 ok=true bytes=%d compile=%s create=%s total=%s", len(code), compiled.Sub(started), time.Since(compiled), time.Since(started))
	return nil
}

func (b *mdD3D11Backend) ensureCelestialAssets() error {
	if b.celestialAssetsReady {
		return nil
	}
	if b.celestialAssetsErr != "" {
		return fmt.Errorf("%s", b.celestialAssetsErr)
	}
	started := time.Now()
	if err := b.ensureCelestialPipeline(); err != nil {
		b.celestialAssetsErr = err.Error()
		return err
	}
	mdLoadCelestialAssets()
	textures := [...]mdEmbeddedTexture{mdEarthTexture, mdMoonTexture, mdSaturnGlobal, mdD3D11RingTexture()}
	names := [...]string{"earth", "moon", "saturn", "ring"}
	for i := range textures {
		uploadStarted := time.Now()
		texture, srv, err := b.createTextureSRV(textures[i])
		if err != nil {
			for j := range b.celestialSRVs {
				mdCOMRelease(&b.celestialSRVs[j])
				mdCOMRelease(&b.celestialTextures[j])
			}
			b.celestialAssetsErr = err.Error()
			mdD3D11LogFallbackOnce("celestial", b.celestialAssetsErr)
			return err
		}
		b.celestialTextures[i], b.celestialSRVs[i] = texture, srv
		if names[i] == "earth" {
			logf("MULTIDIM_GPU upload name=%s size=%dx%d bytes=%d source=%s path=%q override_error=%q rgba_sha256=%s detail_rgb=%d,%d,%d/%d,%d,%d elapsed=%s", names[i], textures[i].w, textures[i].h, len(textures[i].rgba), mdEarthTextureSource, mdEarthTextureExternalPath, mdEarthTextureExternalErr, mdEarthTextureRGBAHash, mdEarthDetailColours[0].r, mdEarthDetailColours[0].g, mdEarthDetailColours[0].b, mdEarthDetailColours[1].r, mdEarthDetailColours[1].g, mdEarthDetailColours[1].b, time.Since(uploadStarted))
		} else {
			logf("MULTIDIM_GPU upload name=%s size=%dx%d bytes=%d elapsed=%s", names[i], textures[i].w, textures[i].h, len(textures[i].rgba), time.Since(uploadStarted))
		}
	}
	b.celestialAssetsReady = true
	logf("MULTIDIM_GPU textures celestial ok total=%s", time.Since(started))
	return nil
}

func mdD3D11CelestialShapeName(skin int) string {
	switch skin {
	case mdSphereSkinEarth:
		return "earth"
	case mdSphereSkinMoon:
		return "moon"
	case mdSphereSkinSaturn:
		return "saturn"
	default:
		return "celestial"
	}
}

func mdD3D11RenderCelestial(params mdCelestialRenderParams) ([]byte, bool) {
	if params.width <= 0 || params.height <= 0 || params.radiusPixels <= 0 || params.skin < mdSphereSkinEarth || params.skin > mdSphereSkinSaturn {
		return nil, false
	}
	started := time.Now()
	shape := mdD3D11CelestialShapeName(params.skin)
	b := mdD3D11Get()
	if b == nil || !b.deviceReady {
		mdD3D11LogFallbackOnce("celestial", mdD3D11PipelineReason("celestial"))
		return nil, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.ensureCelestialAssets(); err != nil {
		mdD3D11LogFallbackOnce("celestial", err.Error())
		return nil, false
	}
	if err := b.ensureCelestialOutputResources(params.width, params.height); err != nil {
		mdD3D11LogFallbackOnce("celestial-output", err.Error())
		return nil, false
	}
	polarScale := float32(1)
	if params.skin == mdSphereSkinSaturn {
		polarScale = 0.902
	}
	m := params.rotation
	c := mdD3D11CelestialConstants{
		Width: uint32(params.width), Height: uint32(params.height), Skin: uint32(params.skin),
		Radius: float32(params.radiusPixels), CenterX: float32(params.width) * 0.5, CenterY: float32(params.height) * 0.5,
		PolarScale: polarScale,
		InvRow0:    [4]float32{float32(m[0][0]), float32(m[1][0]), float32(m[2][0]), 0},
		InvRow1:    [4]float32{float32(m[0][1]), float32(m[1][1]), float32(m[2][1]), 0},
		InvRow2:    [4]float32{float32(m[0][2]), float32(m[1][2]), float32(m[2][2]), 0},
	}
	if params.maskPass {
		c.MaskPass = 1
	}
	var mapped mdD3D11MappedSubresource
	if hr := mdCOMCall(b.context, 14, b.celestialCB, 0, mdD3D11MapWriteDiscard, 0, uintptr(unsafe.Pointer(&mapped))); mdHRESULTFailed(hr) || mapped.Data == 0 {
		mdD3D11LogFallbackOnce("celestial-map", fmt.Sprintf("constant Map HRESULT=%#x", uint32(hr)))
		return nil, false
	}
	*(*mdD3D11CelestialConstants)(unsafe.Pointer(mapped.Data)) = c
	mdCOMCall(b.context, 15, b.celestialCB, 0)

	cb := b.celestialCB
	uav := b.celestialOutputUAV
	sampler := b.celestialSampler
	mdCOMCall(b.context, 71, 0, 1, uintptr(unsafe.Pointer(&cb)))
	mdCOMCall(b.context, 67, 0, uintptr(len(b.celestialSRVs)), uintptr(unsafe.Pointer(&b.celestialSRVs[0])))
	mdCOMCall(b.context, 70, 0, 1, uintptr(unsafe.Pointer(&sampler)))
	mdCOMCall(b.context, 69, b.celestialCS, 0, 0)
	mdCOMCall(b.context, 68, 0, 1, uintptr(unsafe.Pointer(&uav)), 0)
	prepareDone := time.Now()
	dispatchX, dispatchY := (params.width+7)/8, (params.height+7)/8
	mdCOMCall(b.context, 41, uintptr(dispatchX), uintptr(dispatchY), 1)
	var nilUAV uintptr
	mdCOMCall(b.context, 68, 0, 1, uintptr(unsafe.Pointer(&nilUAV)), 0)
	var nilSRVs [4]uintptr
	mdCOMCall(b.context, 67, 0, uintptr(len(nilSRVs)), uintptr(unsafe.Pointer(&nilSRVs[0])))
	mdCOMCall(b.context, 47, b.celestialOutputStaging, b.celestialOutputTexture)
	if hr := mdCOMCall(b.context, 14, b.celestialOutputStaging, 0, mdD3D11MapRead, 0, uintptr(unsafe.Pointer(&mapped))); mdHRESULTFailed(hr) || mapped.Data == 0 {
		mdD3D11LogFallbackOnce("celestial-readback", fmt.Sprintf("staging Map HRESULT=%#x", uint32(hr)))
		return nil, false
	}
	mapDone := time.Now()
	src := unsafe.Slice((*byte)(unsafe.Pointer(mapped.Data)), int(mapped.RowPitch)*params.height)
	pix := make([]byte, params.width*params.height*4)
	for y := 0; y < params.height; y++ {
		srow := src[y*int(mapped.RowPitch):]
		drow := pix[y*params.width*4:]
		for x := 0; x < params.width; x++ {
			si, di := x*4, x*4
			drow[di+0] = srow[si+2]
			drow[di+1] = srow[si+1]
			drow[di+2] = srow[si+0]
			drow[di+3] = 0
		}
	}
	mdCOMCall(b.context, 15, b.celestialOutputStaging, 0)
	finished := time.Now()
	logf("MULTIDIM_RENDER shape=%s backend=D3D11 size=%dx%d dispatch=%dx%d prepare=%s gpu_wait_readback=%s convert=%s total=%s fallback=false", shape, params.width, params.height, dispatchX, dispatchY, prepareDone.Sub(started), mapDone.Sub(prepareDone), finished.Sub(mapDone), finished.Sub(started))
	return pix, true
}

func (b *mdD3D11Backend) ensureBlackHoleResources(w, h int) error {
	if b.blackHoleW == w && b.blackHoleH == h && b.blackHoleTexture != 0 && b.blackHoleUAV != 0 && b.blackHoleStaging != 0 {
		return nil
	}
	oldW, oldH := b.blackHoleW, b.blackHoleH
	b.releaseBlackHoleSizeResources()
	base := mdD3D11Texture2DDesc{Width: uint32(w), Height: uint32(h), MipLevels: 1, ArraySize: 1, Format: mdDXGIFormatR8G8B8A8UNorm, SampleDesc: mdD3D11SampleDesc{Count: 1}, Usage: mdD3D11UsageDefault, BindFlags: mdD3D11BindUnorderedAccess}
	tex, err := b.createTexture(base)
	if err != nil {
		return err
	}
	b.blackHoleTexture = tex
	if hr := mdCOMCall(b.device, 8, b.blackHoleTexture, 0, uintptr(unsafe.Pointer(&b.blackHoleUAV))); mdHRESULTFailed(hr) || b.blackHoleUAV == 0 {
		b.releaseBlackHoleSizeResources()
		return fmt.Errorf("CreateUnorderedAccessView(blackhole) HRESULT=%#x", uint32(hr))
	}
	base.Usage, base.BindFlags, base.CPUAccessFlags = mdD3D11UsageStaging, 0, mdD3D11CPUAccessRead
	staging, err := b.createTexture(base)
	if err != nil {
		b.releaseBlackHoleSizeResources()
		return err
	}
	b.blackHoleStaging = staging
	b.blackHoleW, b.blackHoleH = w, h
	logf("MULTIDIM_GPU resize pipeline=blackhole old=%dx%d new=%dx%d", oldW, oldH, w, h)
	return nil
}

func (b *mdD3D11Backend) ensureCelestialOutputResources(w, h int) error {
	if b.celestialOutputW == w && b.celestialOutputH == h && b.celestialOutputTexture != 0 && b.celestialOutputUAV != 0 && b.celestialOutputStaging != 0 {
		return nil
	}
	oldW, oldH := b.celestialOutputW, b.celestialOutputH
	b.releaseCelestialOutputResources()
	base := mdD3D11Texture2DDesc{Width: uint32(w), Height: uint32(h), MipLevels: 1, ArraySize: 1, Format: mdDXGIFormatR8G8B8A8UNorm, SampleDesc: mdD3D11SampleDesc{Count: 1}, Usage: mdD3D11UsageDefault, BindFlags: mdD3D11BindUnorderedAccess}
	tex, err := b.createTexture(base)
	if err != nil {
		return err
	}
	b.celestialOutputTexture = tex
	if hr := mdCOMCall(b.device, 8, b.celestialOutputTexture, 0, uintptr(unsafe.Pointer(&b.celestialOutputUAV))); mdHRESULTFailed(hr) || b.celestialOutputUAV == 0 {
		b.releaseCelestialOutputResources()
		return fmt.Errorf("CreateUnorderedAccessView(celestial) HRESULT=%#x", uint32(hr))
	}
	base.Usage, base.BindFlags, base.CPUAccessFlags = mdD3D11UsageStaging, 0, mdD3D11CPUAccessRead
	staging, err := b.createTexture(base)
	if err != nil {
		b.releaseCelestialOutputResources()
		return err
	}
	b.celestialOutputStaging = staging
	b.celestialOutputW, b.celestialOutputH = w, h
	logf("MULTIDIM_GPU resize pipeline=celestial old=%dx%d new=%dx%d", oldW, oldH, w, h)
	return nil
}

func (b *mdD3D11Backend) ensureFaceResources(w, h, vertexBytes int) error {
	if b.faceW != w || b.faceH != h || b.faceTexture == 0 || b.faceRTV == 0 || b.faceDepth == 0 || b.faceDepthClear == 0 || b.faceDSV == 0 || b.faceStaging == 0 {
		b.releaseFaceSizeResources()
		colorDesc := mdD3D11Texture2DDesc{Width: uint32(w), Height: uint32(h), MipLevels: 1, ArraySize: 1, Format: mdDXGIFormatR8G8B8A8UNorm, SampleDesc: mdD3D11SampleDesc{Count: 1}, Usage: mdD3D11UsageDefault, BindFlags: mdD3D11BindRenderTarget}
		tex, err := b.createTexture(colorDesc)
		if err != nil {
			return err
		}
		b.faceTexture = tex
		if hr := mdCOMCall(b.device, 9, b.faceTexture, 0, uintptr(unsafe.Pointer(&b.faceRTV))); mdHRESULTFailed(hr) || b.faceRTV == 0 {
			b.releaseFaceSizeResources()
			return fmt.Errorf("CreateRenderTargetView HRESULT=%#x", uint32(hr))
		}
		depthDesc := colorDesc
		depthDesc.Format, depthDesc.BindFlags = mdDXGIFormatD32Float, mdD3D11BindDepthStencil
		depth, err := b.createTexture(depthDesc)
		if err != nil {
			b.releaseFaceSizeResources()
			return err
		}
		b.faceDepth = depth
		clearValues := make([]float32, w*h)
		for i := range clearValues {
			clearValues[i] = 1.0
		}
		clearDesc := depthDesc
		clearDesc.BindFlags = 0
		clearData := mdD3D11SubresourceData{SysMem: uintptr(unsafe.Pointer(&clearValues[0])), SysMemPitch: uint32(w * 4), SysMemSlicePitch: uint32(w * h * 4)}
		depthClear, err := b.createTextureWithData(clearDesc, clearData)
		if err != nil {
			b.releaseFaceSizeResources()
			return err
		}
		b.faceDepthClear = depthClear
		if hr := mdCOMCall(b.device, 10, b.faceDepth, 0, uintptr(unsafe.Pointer(&b.faceDSV))); mdHRESULTFailed(hr) || b.faceDSV == 0 {
			b.releaseFaceSizeResources()
			return fmt.Errorf("CreateDepthStencilView HRESULT=%#x", uint32(hr))
		}
		colorDesc.Usage, colorDesc.BindFlags, colorDesc.CPUAccessFlags = mdD3D11UsageStaging, 0, mdD3D11CPUAccessRead
		staging, err := b.createTexture(colorDesc)
		if err != nil {
			b.releaseFaceSizeResources()
			return err
		}
		b.faceStaging = staging
		b.faceW, b.faceH = w, h
	}
	if vertexBytes > b.faceVBCap || b.faceVB == 0 {
		mdCOMRelease(&b.faceVB)
		capBytes := 4096
		for capBytes < vertexBytes {
			capBytes *= 2
		}
		desc := mdD3D11BufferDesc{ByteWidth: uint32(capBytes), Usage: mdD3D11UsageDynamic, BindFlags: mdD3D11BindVertexBuffer, CPUAccessFlags: mdD3D11CPUAccessWrite}
		if hr := mdCOMCall(b.device, 3, uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&b.faceVB))); mdHRESULTFailed(hr) || b.faceVB == 0 {
			return fmt.Errorf("4D vertex buffer 생성 실패 HRESULT=%#x", uint32(hr))
		}
		b.faceVBCap = capBytes
	}
	return nil
}

func mdD3D11RenderBlackHole(params mdBHRenderParams) ([]byte, bool) {
	if params.mode != mdBlackHoleModeRender && params.mode != mdBlackHoleModeRenderWithDiagram {
		return nil, false
	}
	started := time.Now()
	b := mdD3D11Get()
	if b == nil || !b.deviceReady || !b.blackHoleReady {
		mdD3D11LogFallbackOnce("blackhole", mdD3D11PipelineReason("blackhole"))
		return nil, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.ensureBlackHoleResources(params.width, params.height); err != nil {
		mdD3D11LogFallbackOnce("blackhole-output", err.Error())
		return nil, false
	}
	fovY := float32(mdBHClamp(params.fovY, 10*math.Pi/180, 85*math.Pi/180))
	tanY := float32(math.Tan(float64(fovY) * 0.5))
	tanX := tanY * float32(params.width) / float32(params.height)
	n := mdNormCore(params.diskNormal)
	c := mdD3D11BHConstants{
		Width: uint32(params.width), Height: uint32(params.height), Quality: uint32(params.quality), Mode: uint32(params.mode),
		TanX: tanX, TanY: tanY, DiskX: float32(n.x), DiskY: float32(n.y), DiskZ: float32(n.z),
		Spin: float32(mdBHKerrClampSpin(params.spin)), Horizon: float32(mdBHKerrHorizon(params.spin)), DiskInner: float32(mdBHKerrISCO(params.spin)),
	}
	var mapped mdD3D11MappedSubresource
	if hr := mdCOMCall(b.context, 14, b.blackHoleCB, 0, mdD3D11MapWriteDiscard, 0, uintptr(unsafe.Pointer(&mapped))); mdHRESULTFailed(hr) || mapped.Data == 0 {
		mdD3D11LogFallbackOnce("blackhole-map", fmt.Sprintf("constant Map HRESULT=%#x", uint32(hr)))
		return nil, false
	}
	*(*mdD3D11BHConstants)(unsafe.Pointer(mapped.Data)) = c
	mdCOMCall(b.context, 15, b.blackHoleCB, 0)

	cb := b.blackHoleCB
	uav := b.blackHoleUAV
	mdCOMCall(b.context, 71, 0, 1, uintptr(unsafe.Pointer(&cb)))
	mdCOMCall(b.context, 69, b.blackHoleCS, 0, 0)
	mdCOMCall(b.context, 68, 0, 1, uintptr(unsafe.Pointer(&uav)), 0)
	prepareDone := time.Now()
	dispatchX, dispatchY := (params.width+7)/8, (params.height+7)/8
	mdCOMCall(b.context, 41, uintptr(dispatchX), uintptr(dispatchY), 1)
	var nilUAV uintptr
	mdCOMCall(b.context, 68, 0, 1, uintptr(unsafe.Pointer(&nilUAV)), 0)
	mdCOMCall(b.context, 47, b.blackHoleStaging, b.blackHoleTexture)
	if hr := mdCOMCall(b.context, 14, b.blackHoleStaging, 0, mdD3D11MapRead, 0, uintptr(unsafe.Pointer(&mapped))); mdHRESULTFailed(hr) || mapped.Data == 0 {
		mdD3D11LogFallbackOnce("blackhole-readback", fmt.Sprintf("staging Map HRESULT=%#x", uint32(hr)))
		return nil, false
	}
	mapDone := time.Now()
	src := unsafe.Slice((*byte)(unsafe.Pointer(mapped.Data)), int(mapped.RowPitch)*params.height)
	pix := make([]byte, params.width*params.height*4)
	for y := 0; y < params.height; y++ {
		srow := src[y*int(mapped.RowPitch):]
		drow := pix[y*params.width*4:]
		for x := 0; x < params.width; x++ {
			si, di := x*4, x*4
			drow[di+0] = srow[si+2]
			drow[di+1] = srow[si+1]
			drow[di+2] = srow[si+0]
			drow[di+3] = 0
		}
	}
	mdCOMCall(b.context, 15, b.blackHoleStaging, 0)
	if params.mode == mdBlackHoleModeRenderWithDiagram {
		mdBHDrawGeodesicDiagram(pix, params.width, params.height, true, params.quality, params.spin)
	}
	finished := time.Now()
	logf("MULTIDIM_RENDER shape=blackhole backend=D3D11 size=%dx%d quality=%d spin=%.2f horizon=%.3f isco=%.3f dispatch=%dx%d prepare=%s gpu_wait_readback=%s convert=%s total=%s fallback=false", params.width, params.height, params.quality, params.spin, mdBHKerrHorizon(params.spin), mdBHKerrISCO(params.spin), dispatchX, dispatchY, prepareDone.Sub(started), mapDone.Sub(prepareDone), finished.Sub(mapDone), finished.Sub(started))
	return pix, true
}

func mdBuildD3D11FaceVertices(verts []mdVec3, faces []mdFace4, projected []mdDepthRasterVertex, left, top, w, h int, fillColor, background uint32, maxBrightness, alpha float64) []mdD3D11FaceVertex {
	if len(verts) == 0 || len(projected) != len(verts) || w <= 0 || h <= 0 {
		return nil
	}
	minDepth, maxDepth := math.Inf(1), math.Inf(-1)
	for _, v := range verts {
		minDepth = math.Min(minDepth, v.y)
		maxDepth = math.Max(maxDepth, v.y)
	}
	depthRange := maxDepth - minDepth
	if depthRange < 1e-7 {
		depthRange = 1
	}
	incomingLight := mdSceneIncomingLightDirection()
	out := make([]mdD3D11FaceVertex, 0, len(faces)*6)
	makeVertex := func(p mdDepthRasterVertex, r, g, b float32) mdD3D11FaceVertex {
		nx := float32(2.0*(p.x-float64(left))/float64(w) - 1.0)
		ny := float32(1.0 - 2.0*(p.y-float64(top))/float64(h))
		nz := float32((maxDepth - p.depth) / depthRange)
		if nz < 0 {
			nz = 0
		} else if nz > 1 {
			nz = 1
		}
		return mdD3D11FaceVertex{X: nx, Y: ny, Z: nz, R: r, G: g, B: b, A: 1}
	}
	for _, face := range faces {
		if len(face.indices) < 3 {
			continue
		}
		a3 := verts[face.indices[0]]
		b3 := verts[face.indices[1]]
		c3 := verts[face.indices[2]]
		n := mdNorm(mdCross(mdSub(b3, a3), mdSub(c3, a3)))
		if n.y < 0 {
			n = mdMul(n, -1)
		}
		brightness := 0.40 + 0.60*math.Max(0, mdDot(n, incomingLight))
		if brightness > maxBrightness {
			brightness = maxBrightness
		}
		col := mdBlendedColorOnBackground(fillColor, background, brightness, alpha)
		r := float32(byte(col&0xFF)) / 255.0
		g := float32(byte((col>>8)&0xFF)) / 255.0
		b := float32(byte((col>>16)&0xFF)) / 255.0
		root := projected[face.indices[0]]
		for tri := 1; tri+1 < len(face.indices); tri++ {
			p1 := projected[face.indices[tri]]
			p2 := projected[face.indices[tri+1]]
			if math.Abs(mdDepthRasterEdge(root, p1, p2)) < 1e-9 {
				continue
			}
			out = append(out, makeVertex(root, r, g, b), makeVertex(p1, r, g, b), makeVertex(p2, r, g, b))
		}
	}
	return out
}

func mdD3D11RenderFaceTriangles(label string, w, h int, vertices []mdD3D11FaceVertex, background uint32) ([]byte, bool) {
	if w <= 0 || h <= 0 || len(vertices) < 3 {
		return nil, false
	}
	started := time.Now()
	b := mdD3D11Get()
	if b == nil || !b.deviceReady || !b.faceReady {
		mdD3D11LogFallbackOnce("faces", mdD3D11PipelineReason("faces"))
		return nil, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	byteCount := len(vertices) * int(unsafe.Sizeof(mdD3D11FaceVertex{}))
	if err := b.ensureFaceResources(w, h, byteCount); err != nil {
		mdD3D11LogFallbackOnce("faces-output", err.Error())
		return nil, false
	}
	var mapped mdD3D11MappedSubresource
	if hr := mdCOMCall(b.context, 14, b.faceVB, 0, mdD3D11MapWriteDiscard, 0, uintptr(unsafe.Pointer(&mapped))); mdHRESULTFailed(hr) || mapped.Data == 0 {
		mdD3D11LogFallbackOnce("faces-map", fmt.Sprintf("vertex Map HRESULT=%#x", uint32(hr)))
		return nil, false
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(mapped.Data)), byteCount)
	src := unsafe.Slice((*byte)(unsafe.Pointer(&vertices[0])), byteCount)
	copy(dst, src)
	mdCOMCall(b.context, 15, b.faceVB, 0)

	clear := [4]float32{
		float32(background&0xFF) / 255.0,
		float32((background>>8)&0xFF) / 255.0,
		float32((background>>16)&0xFF) / 255.0,
		0,
	}
	mdCOMCall(b.context, 47, b.faceDepth, b.faceDepthClear)
	mdCOMCall(b.context, 33, 1, uintptr(unsafe.Pointer(&b.faceRTV)), b.faceDSV)
	mdCOMCall(b.context, 50, b.faceRTV, uintptr(unsafe.Pointer(&clear[0])))
	viewport := mdD3D11Viewport{Width: float32(w), Height: float32(h), MinDepth: 0, MaxDepth: 1}
	mdCOMCall(b.context, 44, 1, uintptr(unsafe.Pointer(&viewport)))
	mdCOMCall(b.context, 43, b.faceRasterState)
	mdCOMCall(b.context, 17, b.faceLayout)
	vb := b.faceVB
	stride := uint32(unsafe.Sizeof(mdD3D11FaceVertex{}))
	offset := uint32(0)
	mdCOMCall(b.context, 18, 0, 1, uintptr(unsafe.Pointer(&vb)), uintptr(unsafe.Pointer(&stride)), uintptr(unsafe.Pointer(&offset)))
	mdCOMCall(b.context, 24, mdD3D11PrimitiveTriangleList)
	mdCOMCall(b.context, 11, b.faceVS, 0, 0)
	mdCOMCall(b.context, 9, b.facePS, 0, 0)
	prepareDone := time.Now()
	mdCOMCall(b.context, 13, uintptr(len(vertices)), 0)
	var nilRTV uintptr
	mdCOMCall(b.context, 33, 0, uintptr(unsafe.Pointer(&nilRTV)), 0)
	mdCOMCall(b.context, 47, b.faceStaging, b.faceTexture)
	if hr := mdCOMCall(b.context, 14, b.faceStaging, 0, mdD3D11MapRead, 0, uintptr(unsafe.Pointer(&mapped))); mdHRESULTFailed(hr) || mapped.Data == 0 {
		mdD3D11LogFallbackOnce("faces-readback", fmt.Sprintf("staging Map HRESULT=%#x", uint32(hr)))
		return nil, false
	}
	mapDone := time.Now()
	gpu := unsafe.Slice((*byte)(unsafe.Pointer(mapped.Data)), int(mapped.RowPitch)*h)
	pix := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		srow := gpu[y*int(mapped.RowPitch):]
		drow := pix[y*w*4:]
		for x := 0; x < w; x++ {
			si, di := x*4, x*4
			drow[di+0] = srow[si+2]
			drow[di+1] = srow[si+1]
			drow[di+2] = srow[si+0]
			drow[di+3] = 0
		}
	}
	mdCOMCall(b.context, 15, b.faceStaging, 0)
	finished := time.Now()
	if label == "" {
		label = "faces"
	}
	logf("MULTIDIM_RENDER shape=%s backend=D3D11 size=%dx%d vertices=%d prepare=%s gpu_wait_readback=%s convert=%s total=%s fallback=false", label, w, h, len(vertices), prepareDone.Sub(started), mapDone.Sub(prepareDone), finished.Sub(mapDone), finished.Sub(started))
	return pix, true
}

func mdD3D11RenderOpaqueFaceTriangles(w, h int, vertices []mdD3D11FaceVertex) ([]byte, bool) {
	return mdD3D11RenderFaceTriangles("opaque-4d-faces", w, h, vertices, rgb(255, 255, 255))
}

const mdD3D11FieldPotentialShader = `
struct ChargeSample {
    float4 PosSoft;
    float4 ChargeGateXY;
    float4 GateZEnabledPad;
};
StructuredBuffer<ChargeSample> Samples : register(t0);
RWStructuredBuffer<float> Potentials : register(u0);
cbuffer Params : register(b0) {
    uint Steps;
    uint SampleCount;
    uint TotalCount;
    uint Pad0;
    float MinCoord;
    float StepSize;
    float SoftFactor;
    float Pad1;
};

[numthreads(64,1,1)]
void CSMain(uint3 tid : SV_DispatchThreadID) {
    uint id = tid.x;
    if (id >= TotalCount) return;
    uint ix = id % Steps;
    uint yz = id / Steps;
    uint iy = yz % Steps;
    uint iz = yz / Steps;
    float3 p = float3(MinCoord, MinCoord, MinCoord) + float3(ix, iy, iz) * StepSize;
    float potential = 0.0;
    [loop]
    for (uint i = 0; i < SampleCount; ++i) {
        ChargeSample s = Samples[i];
        float3 gateN = float3(s.ChargeGateXY.zw, s.GateZEnabledPad.x);
        if (s.GateZEnabledPad.y > 0.5 && dot(gateN, p) < s.ChargeGateXY.y - 1e-6) continue;
        float3 d = p - s.PosSoft.xyz;
        float r = sqrt(dot(d, d) + s.PosSoft.w * s.PosSoft.w * SoftFactor);
        if (r > 1e-9) potential += s.ChargeGateXY.x / r;
    }
    Potentials[id] = potential;
}
`

const mdD3D11FieldVectorShader = `
struct ChargeSample {
    float4 PosSoft;
    float4 ChargeGateXY;
    float4 GateZEnabledPad;
};
struct CurrentSegment {
    float4 MidSoft;
    float4 DLCurrent;
};
struct WireSample {
    float4 CenterPad;
    float4 AxisCurrent;
};
StructuredBuffer<ChargeSample> Charges : register(t0);
StructuredBuffer<CurrentSegment> Segments : register(t1);
StructuredBuffer<WireSample> Wires : register(t2);
RWStructuredBuffer<float4> Vectors : register(u0);
cbuffer Params : register(b0) {
    uint Steps;
    uint ChargeCount;
    uint SegmentCount;
    uint WireCount;
    uint TotalCount;
    uint Mode;
    uint Pad0;
    uint Pad1;
    float MinCoord;
    float StepSize;
    float SoftFactor;
    float Pad2;
};

[numthreads(64,1,1)]
void CSMain(uint3 tid : SV_DispatchThreadID) {
    uint id = tid.x;
    if (id >= TotalCount) return;
    uint ix = id % Steps;
    uint yz = id / Steps;
    uint iy = yz % Steps;
    uint iz = yz / Steps;
    float3 p = float3(MinCoord, MinCoord, MinCoord) + float3(ix, iy, iz) * StepSize;
    float3 field = float3(0.0, 0.0, 0.0);
    if (Mode == 0) {
        [loop]
        for (uint i = 0; i < ChargeCount; ++i) {
            ChargeSample s = Charges[i];
            float3 gateN = float3(s.ChargeGateXY.zw, s.GateZEnabledPad.x);
            if (s.GateZEnabledPad.y > 0.5 && dot(gateN, p) < s.ChargeGateXY.y - 1e-6) continue;
            float3 d = p - s.PosSoft.xyz;
            float r2 = dot(d, d) + s.PosSoft.w * s.PosSoft.w * SoftFactor;
            if (r2 > 1e-18) field += d * (s.ChargeGateXY.x / (r2 * sqrt(r2)));
        }
    } else {
        [loop]
        for (uint i = 0; i < SegmentCount; ++i) {
            CurrentSegment s = Segments[i];
            float3 r = p - s.MidSoft.xyz;
            float r2 = dot(r, r) + s.MidSoft.w * s.MidSoft.w;
            if (r2 > 1e-18) field += cross(s.DLCurrent.xyz, r) * (s.DLCurrent.w / (r2 * sqrt(r2)));
        }
        [loop]
        for (uint i = 0; i < WireCount; ++i) {
            WireSample w = Wires[i];
            float3 axis = w.AxisCurrent.xyz;
            float3 r = p - w.CenterPad.xyz;
            float3 perp = r - axis * dot(r, axis);
            float r2 = dot(perp, perp) + 0.03;
            field += cross(axis, perp) * (w.AxisCurrent.w / r2);
        }
    }
    Vectors[id] = float4(field, length(field));
}
`

const mdD3D11FaceShader = `
struct VSIn { float3 pos : POSITION; float4 color : COLOR0; };
struct VSOut { float4 pos : SV_POSITION; float4 color : COLOR0; };
VSOut VSMain(VSIn v) {
    VSOut o;
    o.pos = float4(v.pos, 1.0);
    o.color = v.color;
    return o;
}
float4 PSMain(VSOut i) : SV_TARGET { return i.color; }
`

const mdD3D11OcclusionShader = `
#define MAX_FACE_VERTS 8
struct FaceData {
    float4 V[MAX_FACE_VERTS];
    uint Count;
    uint3 Pad;
};
struct QueryData { float4 PointEps; };
struct PairData { uint QueryIndex; uint FaceIndex; uint2 Pad; };
StructuredBuffer<FaceData> Faces : register(t0);
StructuredBuffer<QueryData> Queries : register(t1);
StructuredBuffer<PairData> Pairs : register(t2);
RWStructuredBuffer<uint> Counts : register(u0);

bool FaceCoversPoint(FaceData f, QueryData q) {
    float2 p = q.PointEps.xy;
    uint count = min(f.Count, (uint)MAX_FACE_VERTS);
    if (count < 3) return false;
    float4 a = f.V[0];
    [loop]
    for (uint i = 1; i + 1 < count; ++i) {
        float4 b = f.V[i];
        float4 c = f.V[i + 1];
        float2 v0 = b.xy - a.xy;
        float2 v1 = c.xy - a.xy;
        float2 v2 = p - a.xy;
        float denom = v0.x * v1.y - v0.y * v1.x;
        if (abs(denom) < 1e-10) continue;
        float inv = 1.0 / denom;
        float l1 = (v2.x * v1.y - v2.y * v1.x) * inv;
        float l2 = (v0.x * v2.y - v0.y * v2.x) * inv;
        float l0 = 1.0 - l1 - l2;
        const float edgeEps = -1e-5;
        if (l0 < edgeEps || l1 < edgeEps || l2 < edgeEps) continue;
        float faceDepth = l0 * a.z + l1 * b.z + l2 * c.z;
        if (faceDepth > q.PointEps.z + q.PointEps.w) return true;
    }
    return false;
}

[numthreads(64,1,1)]
void CSMain(uint3 tid : SV_DispatchThreadID) {
    PairData pair = Pairs[tid.x];
    if (pair.QueryIndex == 0xffffffffu) return;
    if (FaceCoversPoint(Faces[pair.FaceIndex], Queries[pair.QueryIndex])) {
        InterlockedAdd(Counts[pair.QueryIndex], 1);
    }
}
`

const mdD3D11CelestialShader = `
cbuffer Params : register(b0) {
    uint Width; uint Height; uint Skin; uint MaskPass;
    float Radius; float CenterX; float CenterY; float PolarScale;
    float4 InvRow0; float4 InvRow1; float4 InvRow2;
};
Texture2D<float4> EarthTex : register(t0);
Texture2D<float4> MoonTex : register(t1);
Texture2D<float4> SaturnTex : register(t2);
Texture2D<float4> RingTex : register(t3);
SamplerState LinearClamp : register(s0);
RWTexture2D<float4> Output : register(u0);

static const float PI = 3.14159265358979323846;
float clamp1(float x, float a, float b) { return min(max(x,a),b); }
float3 safeNorm(float3 v) { float q=dot(v,v); return q>1e-16 ? v*rsqrt(q) : float3(0,0,0); }
float3 applyInv(float3 v) { return float3(dot(InvRow0.xyz,v), dot(InvRow1.xyz,v), dot(InvRow2.xyz,v)); }
float3 applyFwd(float3 v) { return InvRow0.xyz*v.x + InvRow1.xyz*v.y + InvRow2.xyz*v.z; }
float wrapAngle(float a) {
    a = fmod(a + PI, 2.0*PI);
    if (a < 0) a += 2.0*PI;
    return a - PI;
}
float3 sampleMap(uint which, float lon, float lat) {
    float u = frac((wrapAngle(lon)+PI)/(2.0*PI));
    float v = saturate((PI*0.5-lat)/PI);
    float2 uv = float2(u,v);
    if (which == 1) return EarthTex.SampleLevel(LinearClamp,uv,0).rgb;
    if (which == 2) return MoonTex.SampleLevel(LinearClamp,uv,0).rgb;
    return SaturnTex.SampleLevel(LinearClamp,uv,0).rgb;
}
float angularDistance(float lon,float lat,float lon0,float lat0) {
    float s=sin(lat)*sin(lat0)+cos(lat)*cos(lat0)*cos(wrapAngle(lon-lon0));
    return acos(clamp1(s,-1,1));
}
float earthDetailLandScore(float3 c) {
    return c.r + c.g - 5.0*c.b;
}
float3 earthDetailColour(float lon0,float lat0) {
    uint tw,th; EarthTex.GetDimensions(tw,th);
    float u=frac((wrapAngle(lon0)+PI)/(2.0*PI));
    float v=saturate((PI*0.5-lat0)/PI);
    int2 centre=int2(round(u*float(tw-1)),round(v*float(th-1)));
    float bestScore=-1e20;
    float3 best=float3(0,0,0);
    [unroll] for(int dy=-2;dy<=2;dy++){
        int py=clamp(centre.y+dy,0,int(th)-1);
        [unroll] for(int dx=-2;dx<=2;dx++){
            int px=centre.x+dx;
            while(px<0) px+=int(tw);
            while(px>=int(tw)) px-=int(tw);
            float3 sampleColour=EarthTex.Load(int3(px,py,0)).rgb;
            float score=earthDetailLandScore(sampleColour);
            if(score>bestScore){bestScore=score;best=sampleColour;}
        }
    }
    return best;
}
float3 preserveEarthIslands(float lon,float lat,float3 c) {
    float d1=angularDistance(lon,lat,130.8667*PI/180.0,37.5*PI/180.0);
    float d2=angularDistance(lon,lat,131.8697*PI/180.0,37.2411*PI/180.0);
    float t=0; float3 land=float3(0,0,0);
    float r1=1.10*PI/180.0, r2=0.78*PI/180.0;
    if (d1<r1) { float q=1-d1/r1; q=q*q*(3-2*q); t=q; land=earthDetailColour(130.8667*PI/180.0,37.5*PI/180.0); }
    if (d2<r2) { float q=1-d2/r2; q=q*q*(3-2*q); if(q>t){t=q;land=earthDetailColour(131.8697*PI/180.0,37.2411*PI/180.0);} }
    return lerp(c,land,t);
}
bool sphereHit(float sx,float sz,out float t,out float3 p,out float3 normalWorld) {
    float3 o=applyInv(float3(sx,4.0,sz));
    float3 d=applyInv(float3(0,-1,0));
    float invP2=1.0/(PolarScale*PolarScale);
    float a=d.x*d.x+d.y*d.y+d.z*d.z*invP2;
    float b=2.0*(o.x*d.x+o.y*d.y+o.z*d.z*invP2);
    float c=o.x*o.x+o.y*o.y+o.z*o.z*invP2-1.0;
    float disc=b*b-4*a*c;
    if(disc<0 || abs(a)<1e-12){t=0;p=0;normalWorld=0;return false;}
    t=(-b-sqrt(disc))/(2*a);
    if(t<0){p=0;normalWorld=0;return false;}
    p=o+d*t;
    float3 ln=safeNorm(float3(p.x,p.y,p.z*invP2));
    normalWorld=safeNorm(applyFwd(ln));
    return true;
}
float ringOpacity(float rho) {
    float band=0.72+0.16*sin((rho-1.28)*58.0)+0.08*sin((rho-1.28)*143.0);
    if(rho>1.72 && rho<1.80) band*=0.24;
    return saturate(0.46+0.42*band);
}
bool ringHit(float sx,float sz,out float t,out float3 p,out float alpha) {
    float3 o=applyInv(float3(sx,4.0,sz));
    float3 d=applyInv(float3(0,-1,0));
    if(abs(d.z)<1e-7){t=0;p=0;alpha=0;return false;}
    t=-o.z/d.z;
    if(t<0){p=0;alpha=0;return false;}
    p=o+d*t;
    float rho=length(p.xy);
    if(rho<1.28 || rho>2.12){alpha=0;return false;}
    alpha=ringOpacity(rho);
    return true;
}
bool rayHitsEllipsoid(float3 origin,float3 dir) {
    float invP2=1.0/(PolarScale*PolarScale);
    float a=dir.x*dir.x+dir.y*dir.y+dir.z*dir.z*invP2;
    float b=2*(origin.x*dir.x+origin.y*dir.y+origin.z*dir.z*invP2);
    float c=origin.x*origin.x+origin.y*origin.y+origin.z*origin.z*invP2-1;
    float disc=b*b-4*a*c;
    if(disc<=0 || abs(a)<1e-12) return false;
    float q=sqrt(disc);
    float t0=(-b-q)/(2*a),t1=(-b+q)/(2*a);
    return t0>1e-4 || t1>1e-4;
}
float3 shadeSurface(float3 c,float3 n,uint skin) {
    float3 light=safeNorm(float3(-0.42,0.78,0.46));
    float diffuse=max(0,dot(n,light));
    float ambient=skin==1 ? 0.14 : 0.19;
    float shade=ambient+(1-ambient)*diffuse;
    float3 halfv=safeNorm(light+float3(0,1,0));
    float spec=skin==2 ? 0 : pow(max(0,dot(n,halfv)),24.0);
    return saturate(c*shade+(26.0/255.0)*spec);
}
float saturnRingShadow(float3 surfacePoint) {
    float3 lightWorld=safeNorm(float3(-0.42,0.78,0.46));
    float3 lightLocal=applyInv(lightWorld);
    if(abs(lightLocal.z)<1e-8) return 1;
    float t=-surfacePoint.z/lightLocal.z;
    if(t<=1e-4) return 1;
    float3 q=surfacePoint+lightLocal*t;
    float rho=length(q.xy);
    if(rho<1.28 || rho>2.12) return 1;
    return 1-0.34*ringOpacity(rho);
}
[numthreads(8,8,1)]
void CSMain(uint3 id:SV_DispatchThreadID) {
    if(id.x>=Width || id.y>=Height) return;
    float sx=(float(id.x)+0.5-CenterX)/Radius;
    float sz=(CenterY-(float(id.y)+0.5))/Radius;
    float sphereT,ringT,ringA; float3 p,n,rp;
    bool hs=sphereHit(sx,sz,sphereT,p,n);
    bool hr=false;
    if(Skin==3) hr=ringHit(sx,sz,ringT,rp,ringA);
    if(!hs && !hr){Output[id.xy]=float4(1,1,1,0);return;}
    float3 outc=float3(1,1,1);
    if(hs){
        if(MaskPass!=0) outc=0;
        else {
            float3 q=p;
            if(Skin==3) q.z/=PolarScale;
            q=safeNorm(q);
            float base=atan2(q.x,q.y);
            float lon=Skin==1 ? wrapAngle(128.0*PI/180.0+base) : wrapAngle(base);
            float lat=asin(clamp1(q.z,-1,1));
            if(Skin==1){ outc=sampleMap(1,lon,lat); outc=preserveEarthIslands(lon,lat,outc); }
            else if(Skin==2) outc=sampleMap(2,lon,lat);
            else outc=sampleMap(3,lon,lat);
            outc=shadeSurface(outc,n,Skin);
            if(Skin==3) outc*=saturnRingShadow(p);
            float limb=saturate((n.y-0.01)/0.10);
            outc=lerp(float3(18,18,18)/255.0,outc,limb);
        }
    }
    if(hr && (!hs || ringT<sphereT)){
        if(MaskPass!=0){ float v=1-ringA; outc=float3(v,v,v); }
        else {
            float rho=length(rp.xy);
            float u=saturate((rho-1.28)/(2.12-1.28));
            float3 rc=RingTex.SampleLevel(LinearClamp,float2(u,0.5),0).rgb;
            float3 light=safeNorm(float3(-0.42,0.78,0.46));
            float3 ringNormal=safeNorm(applyFwd(float3(0,0,1)));
            float sh=0.42+0.58*abs(dot(ringNormal,light));
            float3 lightLocal=applyInv(light);
            if(rayHitsEllipsoid(rp,lightLocal)) sh*=0.34;
            rc=saturate(rc*sh);
            outc=lerp(outc,rc,ringA);
        }
    }
    Output[id.xy]=float4(saturate(outc),0);
}
`

const mdD3D11BlackHoleShader = `
cbuffer Params : register(b0) {
    uint Width; uint Height; uint Quality; uint Mode;
    float TanX; float TanY; float Pad0; float Pad1;
    float DiskX; float DiskY; float DiskZ; float Pad2;
    float Spin; float KerrHorizon; float KerrDiskInner; float Pad3;
};
RWTexture2D<float4> Output : register(u0);

static const float PI = 3.14159265358979323846;
static const float SchwarzHorizon = 2.0;
static const float PhotonSphere = 3.0;
static const float SchwarzDiskInner = 6.0;
static const float DiskOuter = 15.0;
static const float DiskHalf = 0.22;
static const float CameraRadius = 36.0;

float clamp1(float x, float a, float b) { return min(max(x,a),b); }
float3 safeNorm(float3 v) { float q = dot(v,v); return q > 1e-16 ? v*rsqrt(q) : float3(0,0,0); }
float3 orbitDeriv(float3 s, float impact) {
    float r = max(s.x, SchwarzHorizon*0.98);
    float b2 = impact*impact;
    return float3(s.y, b2*(r-3.0)/(r*r*r*r), impact/(r*r));
}
float3 orbitRK4(float3 s, float impact, float h) {
    float3 k1 = orbitDeriv(s,impact);
    float3 k2 = orbitDeriv(s+k1*(h*0.5),impact);
    float3 k3 = orbitDeriv(s+k2*(h*0.5),impact);
    float3 k4 = orbitDeriv(s+k3*h,impact);
    return s + h*(k1+2*k2+2*k3+k4)/6.0;
}
float adaptiveStep(float r, float radialSpeed) {
    float q = Quality == 0 ? 1.45 : 1.0;
    float d = abs(r-PhotonSphere);
    if (d < 0.45 && abs(radialSpeed) < 0.34) return 0.010*q;
    if (r < 3.5) return 0.016*q;
    if (r < 5.0) return 0.028*q;
    if (r < 8.0) return 0.050*q;
    if (r < 14.0) return 0.110*q;
    return 0.220*q;
}
void pointTangent(float3 s, float impact, float3 er0, float3 e2, out float3 p, out float3 tangent) {
    float c = cos(s.z), sn = sin(s.z);
    float3 er = er0*c + e2*sn;
    float3 ep = er0*(-sn) + e2*c;
    p = er*s.x;
    tangent = safeNorm(er*s.y + ep*(impact/max(s.x,1e-8)));
}
bool diskHitRange(float3 a, float3 b, float3 n, float innerRadius, out float3 hit) {
    float ha=dot(a,n), hb=dot(b,n), t=0;
    bool ia=abs(ha)<=DiskHalf, ib=abs(hb)<=DiskHalf;
    if (ia) t=0;
    else if (ib) t=1;
    else if (ha>DiskHalf && hb<DiskHalf) t=(ha-DiskHalf)/(ha-hb);
    else if (ha<-DiskHalf && hb>-DiskHalf) t=(ha+DiskHalf)/(ha-hb);
    else { hit=0; return false; }
    t=clamp1(t,0,1);
    hit=lerp(a,b,t);
    float hh=dot(hit,n);
    float3 planar=hit-n*hh;
    float rho=length(planar);
    return rho>=innerRadius && rho<=DiskOuter;
}
uint hash2(int x,int y) {
    uint h=(uint)x*0x9E3779B1u ^ (uint)y*0x85EBCA77u ^ 0xC2B2AE3Du;
    h^=h>>16; h*=0x7FEB352Du; h^=h>>15; h*=0x846CA68Bu; h^=h>>16; return h;
}
float3 backgroundRGB(float3 dir) {
    float3 d=safeNorm(dir);
    float lon=atan2(d.x,d.y);
    float lat=asin(clamp1(d.z,-1,1));
    float centre=0.18*sin(2*lon)-0.05*sin(5*lon);
    float band=exp(-pow((lat-centre)/0.22,2));
    float3 c=float3(2+7*band,4+9*band,10+18*band);
    int ix=(int)floor((lon+PI)/(2*PI)*2048);
    int iy=(int)floor((lat+PI*0.5)/PI*1024);
    uint h=hash2(ix,iy);
    if ((h&0xFFFFu)>65280u) {
        float v=135.0+(float)((h>>16)&0x7Fu);
        float warm=(float)((h>>24)&0xFFu)/255.0;
        c+=float3(v,v*(0.80+0.20*warm),v*(0.72+0.28*(1-warm)));
    }
    return clamp(c,0,255);
}
float3 temperatureRGB(float kelvin) {
    float t=clamp1(kelvin,1000,40000)/100.0;
    float3 c;
    if (t<=66) {
        c.r=255;
        c.g=99.4708025861*log(t)-161.1195681661;
        c.b=t<=19?0:138.5177312231*log(t-10)-305.0447927307;
    } else {
        c.r=329.698727446*pow(t-60,-0.1332047592);
        c.g=288.1221695283*pow(t-60,-0.0755148492);
        c.b=255;
    }
    return clamp(c,0,255);
}
void diskBasis(float3 n, out float3 u, out float3 v) {
    float3 ref=abs(dot(n,float3(0,0,1)))>0.90?float3(1,0,0):float3(0,0,1);
    u=safeNorm(cross(ref,n)); v=safeNorm(cross(n,u));
}
float3 diskRGB(float3 hit,float3 tangent,float minR,float winding,float3 n) {
    float hh=dot(hit,n); float3 planar=hit-n*hh; float radius=length(planar);
    if (radius<=0) return 0;
    float3 radial=planar/radius; float3 az=safeNorm(cross(n,radial));
    float beta;
    float grav;
    float innerRadius = Spin < 1e-5 ? SchwarzDiskInner : KerrDiskInner;
    if (Spin < 1e-5) {
        beta=sqrt(1.0/max(radius-2.0,1e-6)); beta=clamp1(beta,0,0.62);
        grav=sqrt(max(1-2.0/radius,0.02));
    } else {
        float omega=1.0/(pow(radius,1.5)+Spin);
        float delta=radius*radius-2*radius+Spin*Spin;
        float area=pow(radius*radius+Spin*Spin,2)-Spin*Spin*delta;
        grav=sqrt(max(delta*radius*radius/max(area,1e-9),0.02));
        beta=clamp1(radius*omega/max(grav,0.12),0,0.72);
    }
    float3 toObserver=-safeNorm(tangent);
    float betaLOS=beta*dot(az,toObserver);
    float gamma=rsqrt(max(1-beta*beta,1e-6));
    float doppler=1.0/(gamma*max(1-betaLOS,0.18));
    float t=clamp1((radius-innerRadius)/(DiskOuter-innerRadius),0,1);
    float baseTemp=10500*pow(1-t,0.72)+2400*pow(t,0.82);
    float3 c=temperatureRGB(baseTemp*grav*doppler);
    float3 u,v; diskBasis(n,u,v);
    float phi=atan2(dot(radial,v),dot(radial,u));
    float diskModulation=0.90+0.055*sin(9*phi+0.75*radius)+0.035*sin(23*phi-0.34*radius);
    float radialGlow=0.40+0.95*pow(1-t,1.35);
    float redshift=pow(grav*doppler,3);
    float higher=pow(0.80,max(0,winding-0.20));
    float intensity=1.18*diskModulation*radialGlow*redshift*higher;
    if (minR<3.35) intensity*=1+1.10*exp(-pow((minR-3.0)/0.20,2));
    return clamp(c*intensity,0,255);
}
float3 escapedColor(float3 tangent,float minR,float winding) {
    float3 c=backgroundRGB(tangent);
    if(minR<3.34) {
        float ring=exp(-pow((minR-3.0)/0.16,2));
        float gain=55.0*ring*clamp1(0.5+winding,0.5,2.0);
        c+=float3(gain,gain*0.72,gain*0.35);
    }
    return clamp(c,0,255);
}
float3 traceSchwarzschildColor(float3 initialDir,float3 n) {
    float3 camera=float3(0,CameraRadius,0), er0=float3(0,1,0), d=safeNorm(initialDir);
    float radial=dot(d,er0); float3 tv=d-er0*radial; float tl=length(tv);
    float3 e2;
    if (tl<1e-10) { e2=float3(1,0,0); if(abs(dot(e2,er0))>0.9)e2=float3(0,0,1); e2=safeNorm(e2-er0*dot(e2,er0)); }
    else e2=tv/tl;
    float impact=CameraRadius*tl/sqrt(max(1-2.0/CameraRadius,1e-9));
    float vr=-sqrt(max(1-(1-2.0/CameraRadius)*impact*impact/(CameraRadius*CameraRadius),0));
    if(radial>0) vr=-vr;
    float3 s=float3(CameraRadius,vr,0); float3 prev=camera; float minR=CameraRadius;
    int maxSteps=Quality==0?360:680;
    [loop] for(int step=0;step<680;step++) {
        if(step>=maxSteps) break;
        float h=adaptiveStep(s.x,s.y); float3 next=orbitRK4(s,impact,h);
        if(next.x!=next.x || next.z!=next.z || next.x<=0 || abs(next.x)>1e20) return float3(0.8,1.0,1.8);
        float3 p,tangent; pointTangent(next,impact,er0,e2,p,tangent);
        minR=min(minR,next.x);
        float3 hit;
        if(diskHitRange(prev,p,n,SchwarzDiskInner,hit)) return diskRGB(hit,tangent,minR,abs(next.z)/(2*PI),n);
        if(next.x<=SchwarzHorizon*1.002) return float3(0.8,1.0,1.8);
        if(next.x>=CameraRadius*1.035 && next.y>0 && step>16) return escapedColor(tangent,minR,abs(next.z)/(2*PI));
        s=next; prev=p;
    }
    return float3(0.8,1.0,1.8);
}
void kerrBasis(float3 n,out float3 er,out float3 ex,out float3 ey,out float3 eTheta,out float thetaO) {
    er=float3(0,1,0);
    float3 ez=safeNorm(n);
    float cosO=clamp1(dot(er,ez),-1,1);
    thetaO=acos(cosO);
    ex=er-ez*cosO;
    if(length(ex)<1e-8) { ex=float3(1,0,0); if(abs(dot(ex,ez))>0.9)ex=float3(0,1,0); ex=safeNorm(ex-ez*dot(ex,ez)); }
    else ex=safeNorm(ex);
    ey=safeNorm(cross(ez,ex));
    eTheta=er*cosO-ez;
    if(length(eTheta)<1e-8)eTheta=ex; else eTheta=safeNorm(eTheta);
}
float3 kerrPoint(float3 state,float3 ex,float3 ey,float3 ez) {
    float st=sin(state.y),ct=cos(state.y),cp=cos(state.z),sp=sin(state.z);
    return state.x*(ex*(st*cp)+ey*(st*sp)+ez*ct);
}
void kerrPotentials(float3 state,float a,float lambda,float q,out float radial,out float polar,out float delta,out float sigma,out float pTerm) {
    float r=state.x,sinT=sin(state.y),cosT=cos(state.y),sin2=max(sinT*sinT,1e-9);
    delta=r*r-2*r+a*a;
    sigma=r*r+a*a*cosT*cosT;
    pTerm=r*r+a*a-a*lambda;
    radial=pTerm*pTerm-delta*((lambda-a)*(lambda-a)+q);
    polar=q+a*a*cosT*cosT-lambda*lambda*cosT*cosT/sin2;
}
float3 kerrDerivative(float3 state,float a,float lambda,float q,float radialSign,float thetaSign) {
    float radial,polar,delta,sigma,pTerm;
    kerrPotentials(state,a,lambda,q,radial,polar,delta,sigma,pTerm);
    sigma=max(sigma,1e-9);
    if(abs(delta)<1e-8)delta=delta<0?-1e-8:1e-8;
    float sin2=max(sin(state.y)*sin(state.y),1e-9);
    return float3(radialSign*sqrt(max(radial,0))/sigma,thetaSign*sqrt(max(polar,0))/sigma,(lambda/sin2-a+a*pTerm/delta)/sigma);
}
void normalizeKerrAngles(inout float3 state,inout float thetaSign) {
    if(state.y<0){state.y=-state.y;state.z+=PI;thetaSign=-thetaSign;}
    if(state.y>PI){state.y=2*PI-state.y;state.z+=PI;thetaSign=-thetaSign;}
}
float3 traceKerrColor(float3 initialDir,float3 n) {
    float3 er,ex,ey,eTheta; float thetaO;
    kerrBasis(n,er,ex,ey,eTheta,thetaO);
    float3 ez=safeNorm(n),d=safeNorm(initialDir);
    float inward=max(-dot(d,er),1e-6);
    float alpha=-CameraRadius*dot(d,ey)/inward;
    float beta=CameraRadius*dot(d,eTheta)/inward;
    float sinO=sin(thetaO),cosO=cos(thetaO);
    float lambda=-alpha*sinO;
    float q=beta*beta+cosO*cosO*(alpha*alpha-Spin*Spin);
    float3 state=float3(CameraRadius,thetaO,0);
    float radialSign=-1.0,thetaSign=beta<0?-1.0:1.0;
    float3 prev=float3(0,CameraRadius,0); float minR=CameraRadius;
    int maxSteps=Quality==0?520:920;
    [loop] for(int step=0;step<920;step++) {
        if(step>=maxSteps)break;
        float radial,polar,delta,sigma,pTerm;
        kerrPotentials(state,Spin,lambda,q,radial,polar,delta,sigma,pTerm);
        if(radial<1e-8 && radialSign<0 && state.x>KerrHorizon*1.01)radialSign=1;
        if(polar<1e-9){thetaSign=-thetaSign;state.y+=thetaSign*1e-7;}
        float h=adaptiveStep(state.x,radialSign);
        if(state.x<4)h=min(h,0.010);
        if(state.x<2.5)h=min(h,0.004);
        float3 d1=kerrDerivative(state,Spin,lambda,q,radialSign,thetaSign);
        float3 mid=state+d1*(h*0.5); normalizeKerrAngles(mid,thetaSign);
        float3 d2=kerrDerivative(mid,Spin,lambda,q,radialSign,thetaSign);
        float3 next=state+d2*h; normalizeKerrAngles(next,thetaSign);
        if(next.x!=next.x || next.y!=next.y || next.z!=next.z || next.x<=0 || abs(next.x)>1e20)return float3(0.8,1.0,1.8);
        float3 p=kerrPoint(next,ex,ey,ez);
        float3 tangent=safeNorm(p-prev);
        minR=min(minR,next.x);
        float3 hit;
        if(diskHitRange(prev,p,n,KerrDiskInner,hit))return diskRGB(hit,tangent,minR,abs(next.z)/(2*PI),n);
        if(next.x<=KerrHorizon*1.002)return float3(0.8,1.0,1.8);
        if(next.x>=CameraRadius*1.035 && radialSign>0 && step>18)return escapedColor(tangent,minR,abs(next.z)/(2*PI));
        state=next;prev=p;
    }
    return float3(0.8,1.0,1.8);
}
[numthreads(8,8,1)]
void CSMain(uint3 id : SV_DispatchThreadID) {
    if(id.x>=Width || id.y>=Height) return;
    float nx=2.0*((float)id.x+0.5)/(float)Width-1.0;
    float ny=1.0-2.0*((float)id.y+0.5)/(float)Height;
    float3 dir=safeNorm(float3(nx*TanX,-1.0,ny*TanY));
    float3 n=safeNorm(float3(DiskX,DiskY,DiskZ));
    float3 rgb=(Spin<1e-5?traceSchwarzschildColor(dir,n):traceKerrColor(dir,n))/255.0;
    Output[id.xy]=float4(rgb,1.0);
}
`
